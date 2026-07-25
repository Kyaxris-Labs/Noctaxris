package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Writer appends CloudTrail-shaped JSON lines to events.jsonl under a directory.
type Writer struct {
	path string
	file *os.File
	mu   sync.Mutex

	// afterWrite is invoked after a successful line append (outside the write lock).
	afterWrite func()
}

// NewWriter opens or creates dir/events.jsonl, creating dir when needed.
func NewWriter(dir string) (*Writer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("audit: create dir: %w", err)
	}

	path := filepath.Join(dir, "events.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("audit: open events file: %w", err)
	}
	// Enforce mode on existing files (OpenFile mode applies only at create).
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("audit: chmod events file: %w", err)
	}

	return &Writer{path: path, file: f}, nil
}

// SetAfterWrite registers a callback invoked after each successful Write.
// Pass nil to clear. The callback must not call Write on this Writer (deadlock).
func (w *Writer) SetAfterWrite(fn func()) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.afterWrite = fn
}

// Write marshals ev as one JSON object and appends it as a single line.
func (w *Writer) Write(ctx context.Context, ev Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("audit: marshal event: %w", err)
	}

	w.mu.Lock()
	_, err = w.file.Write(append(data, '\n'))
	after := w.afterWrite
	w.mu.Unlock()
	if err != nil {
		return fmt.Errorf("audit: write event: %w", err)
	}
	if after != nil {
		after()
	}
	return nil
}

// Close releases the underlying events.jsonl file handle.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}

	err := w.file.Close()
	w.file = nil
	return err
}
