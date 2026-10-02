package audit_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
)

func TestNewWriterCancelledContextAndAfterWrite(t *testing.T) {
	dir := t.TempDir()
	w, err := audit.NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	var n atomic.Int32
	w.SetAfterWrite(func() { n.Add(1) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := w.Write(ctx, audit.Event{EventName: "X"}); err == nil {
		t.Fatal("cancelled context should fail")
	}

	if err := w.Write(context.Background(), audit.Event{
		EventVersion: "1.11",
		EventTime:    time.Now().UTC().Format(time.RFC3339),
		EventSource:  "sts.amazonaws.com",
		EventName:    "GetCallerIdentity",
		EventID:      "e1",
		RequestID:    "r1",
	}); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 1 {
		t.Fatalf("afterWrite count=%d", n.Load())
	}

	w.SetAfterWrite(nil)
	var nilWriter *audit.Writer
	nilWriter.SetAfterWrite(func() {})

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	path := filepath.Join(dir, "events.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestNewWriterReopenExistingFile(t *testing.T) {
	dir := t.TempDir()
	w1, err := audit.NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w1.Close(); err != nil {
		t.Fatal(err)
	}
	w2, err := audit.NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w2.Close() })
	if err := w2.Write(context.Background(), audit.Event{EventName: "Reopen"}); err != nil {
		t.Fatal(err)
	}
}
