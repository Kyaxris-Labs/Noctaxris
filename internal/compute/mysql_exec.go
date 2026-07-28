package compute

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// MySQLSQLOpts runs SQL via the mysql client inside a nested MySQL/MariaDB container.
type MySQLSQLOpts struct {
	ContainerID string
	Username    string
	Password    string
	Database    string
	SQL         string
}

// MySQLSQLResult is a lab-shaped tabular result from nested mysql CLI.
type MySQLSQLResult struct {
	Columns                []string
	Rows                   [][]string
	NumberOfRecordsUpdated int64
	Select                 bool
}

// ExecMySQLSQL executes SQL with mysql in the nested container.
// SELECT/WITH use batch tab-separated output; other statements parse affected rows when present.
func (c *Client) ExecMySQLSQL(ctx context.Context, opts MySQLSQLOpts) (MySQLSQLResult, error) {
	if strings.TrimSpace(opts.SQL) == "" {
		return MySQLSQLResult{}, fmt.Errorf("compute: mysql SQL is required")
	}
	user := strings.TrimSpace(opts.Username)
	if user == "" {
		user = "root"
	}
	db := strings.TrimSpace(opts.Database)
	if db == "" {
		db = "mysql"
	}
	sqlText := opts.SQL
	isSelect := isMySQLSelect(sqlText)
	cmd := mysqlExecCmd(user, db, sqlText)
	var env []string
	if pw := strings.TrimSpace(opts.Password); pw != "" {
		env = append(env, "MYSQL_PWD="+pw)
	}
	execRes, err := c.Exec(ctx, ExecOpts{
		ContainerID: opts.ContainerID,
		Cmd:         cmd,
		Env:         env,
	})
	if err != nil {
		return MySQLSQLResult{}, err
	}
	if execRes.ExitCode != 0 {
		msg := strings.TrimSpace(execRes.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(execRes.Stdout)
		}
		if msg == "" {
			msg = fmt.Sprintf("mysql exit %d", execRes.ExitCode)
		}
		return MySQLSQLResult{}, fmt.Errorf("compute: nested mysql: %s", msg)
	}
	if isSelect {
		cols, rows, parseErr := parseMySQLBatchTSV(execRes.Stdout)
		if parseErr != nil {
			return MySQLSQLResult{}, parseErr
		}
		return MySQLSQLResult{
			Columns: cols,
			Rows:    rows,
			Select:  true,
		}, nil
	}
	return MySQLSQLResult{
		NumberOfRecordsUpdated: parseMySQLAffectedRows(execRes.Stdout),
		Select:                 false,
	}, nil
}

func isMySQLSelect(sqlText string) bool {
	upper := strings.ToUpper(strings.TrimSpace(sqlText))
	return strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")
}

// mysqlExecCmd builds nested mysql argv (no psql-only flags).
func mysqlExecCmd(user, db, sqlText string) []string {
	return []string{"mysql", "-u", user, "-D", db, "-B", "-e", sqlText}
}

func parseMySQLBatchTSV(stdout string) ([]string, [][]string, error) {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return nil, nil, nil
	}
	lines := strings.Split(stdout, "\n")
	if len(lines) == 0 {
		return nil, nil, nil
	}
	cols := strings.Split(lines[0], "\t")
	rows := make([][]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		row := make([]string, len(cols))
		for i := range cols {
			if i < len(parts) {
				row[i] = parts[i]
			}
		}
		rows = append(rows, row)
	}
	return cols, rows, nil
}

func parseMySQLAffectedRows(stdout string) int64 {
	// Examples: "Query OK, 1 row affected", "Query OK, 3 rows affected"
	line := strings.TrimSpace(stdout)
	if line == "" {
		return 0
	}
	lower := strings.ToLower(line)
	idx := strings.Index(lower, "row")
	if idx < 0 {
		return 0
	}
	fields := strings.Fields(line[:idx])
	if len(fields) == 0 {
		return 0
	}
	n, err := strconv.ParseInt(fields[len(fields)-1], 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
