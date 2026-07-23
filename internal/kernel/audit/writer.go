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
	defer w.mu.Unlock()

	if _, err := w.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("audit: write event: %w", err)
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
