package compute

import (
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

// PostgresSQLOpts runs SQL via psql inside a nested Postgres container.
// No host port publish. The Data API prefers in-process pgx against the nested
// data-plane DSN when reachable; this DinD-exec path is the fallback.
type PostgresSQLOpts struct {
	ContainerID string
	Username    string
	Password    string
	Database    string
	SQL         string
}

// PostgresSQLResult is a lab-shaped tabular result from nested psql.
type PostgresSQLResult struct {
	Columns                []string
	Rows                   [][]string
	NumberOfRecordsUpdated int64
	Select                 bool
}

// ExecPostgresSQL executes SQL with psql in the nested container.
// SELECT/WITH use CSV output; other statements use ON_ERROR_STOP and parse a
// command tag when present. Failures return a non-nil error (fail closed).
func (c *Client) ExecPostgresSQL(ctx context.Context, opts PostgresSQLOpts) (PostgresSQLResult, error) {
	if strings.TrimSpace(opts.SQL) == "" {
		return PostgresSQLResult{}, fmt.Errorf("compute: postgres SQL is required")
	}
	user := strings.TrimSpace(opts.Username)
	if user == "" {
		user = "postgres"
	}
	db := strings.TrimSpace(opts.Database)
	if db == "" {
		db = "postgres"
	}
	sqlText := opts.SQL
	isSelect := isPostgresSelect(sqlText)
	cmd := []string{"psql", "-U", user, "-d", db, "-v", "ON_ERROR_STOP=1"}
	if isSelect {
		cmd = append(cmd, "--csv", "-c", sqlText)
	} else {
		cmd = append(cmd, "-t", "-A", "-c", sqlText)
	}
	var env []string
	if pw := strings.TrimSpace(opts.Password); pw != "" {
		env = append(env, "PGPASSWORD="+pw)
	}
	execRes, err := c.Exec(ctx, ExecOpts{
		ContainerID: opts.ContainerID,
		Cmd:         cmd,
		Env:         env,
	})
	if err != nil {
		return PostgresSQLResult{}, err
	}
	if execRes.ExitCode != 0 {
		msg := strings.TrimSpace(execRes.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(execRes.Stdout)
		}
		if msg == "" {
			msg = fmt.Sprintf("psql exit %d", execRes.ExitCode)
		}
		return PostgresSQLResult{}, fmt.Errorf("compute: nested postgres: %s", msg)
	}
	if isSelect {
		cols, rows, parseErr := parsePostgresCSV(execRes.Stdout)
		if parseErr != nil {
			return PostgresSQLResult{}, parseErr
		}
		return PostgresSQLResult{
			Columns: cols,
			Rows:    rows,
			Select:  true,
		}, nil
	}
	return PostgresSQLResult{
		NumberOfRecordsUpdated: parsePostgresCommandTagUpdated(execRes.Stdout),
		Select:                 false,
	}, nil
}

func isPostgresSelect(sqlText string) bool {
	upper := strings.ToUpper(strings.TrimSpace(sqlText))
	return strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")
}

func parsePostgresCSV(stdout string) ([]string, [][]string, error) {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return nil, nil, nil
	}
	r := csv.NewReader(strings.NewReader(stdout))
	r.ReuseRecord = false
	records, err := r.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("compute: parse postgres csv: %w", err)
	}
	if len(records) == 0 {
		return nil, nil, nil
	}
	cols := records[0]
	rows := make([][]string, 0, len(records)-1)
	for _, rec := range records[1:] {
		row := make([]string, len(cols))
		for i := range cols {
			if i < len(rec) {
				row[i] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return cols, rows, nil
}

func parsePostgresCommandTagUpdated(stdout string) int64 {
	line := strings.TrimSpace(stdout)
	if line == "" {
		return 0
	}
	// Examples: "INSERT 0 1", "UPDATE 3", "DELETE 2"
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return 0
	}
	n, err := strconv.ParseInt(fields[len(fields)-1], 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
