package audit_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
)

func TestWriteEvent(t *testing.T) {
	dir := t.TempDir()

	w, err := audit.NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Errorf("close writer: %v", err)
		}
	})

	ev := audit.Event{
		EventVersion:       "1.11",
		EventTime:          time.Now().UTC().Format(time.RFC3339),
		EventSource:        "sts.amazonaws.com",
		EventName:          "GetCallerIdentity",
		AWSRegion:          "us-east-1",
		RequestID:          "req-1",
		EventID:            "evt-1",
		EventType:          "AwsApiCall",
		RecipientAccountID: "000000000001",
		ErrorCode:          "InvalidClientTokenId",
		ReadOnly:           true,
	}
	if err := w.Write(context.Background(), ev); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	line := strings.TrimSpace(string(data))
	if !strings.Contains(line, `"errorCode":"InvalidClientTokenId"`) {
		t.Fatalf("expected InvalidClientTokenId in %q", line)
	}
	if !strings.Contains(line, `"eventVersion":"1.11"`) {
		t.Fatalf("expected eventVersion 1.11 in %q", line)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o644 {
			t.Fatalf("events.jsonl mode=%o want 0644", perm)
		}
	}
}
