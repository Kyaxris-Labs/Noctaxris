package audit_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
)

func TestSetAfterWriteCallback(t *testing.T) {
	dir := t.TempDir()
	w, err := audit.NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	var hits atomic.Int32
	w.SetAfterWrite(func() { hits.Add(1) })
	ev := audit.Event{
		EventVersion: "1.11",
		EventTime:    time.Now().UTC().Format(time.RFC3339),
		EventSource:  "sts.amazonaws.com",
		EventName:    "GetCallerIdentity",
		AWSRegion:    "us-east-1",
		RequestID:    "req-after",
		EventID:      "evt-after",
		EventType:    "AwsApiCall",
	}
	if err := w.Write(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}
	w.SetAfterWrite(nil)
	if err := w.Write(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("cleared callback still fired hits=%d", hits.Load())
	}
	var nilWriter *audit.Writer
	nilWriter.SetAfterWrite(func() {})
}
