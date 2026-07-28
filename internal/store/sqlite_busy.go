package store

import (
	"strings"
	"time"
)

const sqliteBusyAttempts = 12

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "database is locked") || strings.Contains(s, "SQLITE_BUSY")
}

// withSQLiteBusyRetry runs fn, retrying when SQLite returns SQLITE_BUSY as a
// multi-connection deadlock victim. busy_timeout does not wait on those;
// brief backoff lets the other writer finish.
func withSQLiteBusyRetry(fn func() error) error {
	var err error
	for attempt := 0; attempt < sqliteBusyAttempts; attempt++ {
		err = fn()
		if err == nil || !isSQLiteBusy(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 2 * time.Millisecond)
	}
	return err
}
