package store

import (
	"errors"
	"testing"
)

func TestIsSQLiteBusy(t *testing.T) {
	if isSQLiteBusy(nil) {
		t.Fatal("nil")
	}
	if !isSQLiteBusy(errors.New("receive messages: database is locked (5) (SQLITE_BUSY)")) {
		t.Fatal("want busy")
	}
	if isSQLiteBusy(errors.New("no such table")) {
		t.Fatal("non-busy")
	}
}

func TestWithSQLiteBusyRetrySucceedsAfterBusy(t *testing.T) {
	n := 0
	err := withSQLiteBusyRetry(func() error {
		n++
		if n < 3 {
			return errors.New("database is locked (5) (SQLITE_BUSY)")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("attempts=%d", n)
	}
}
