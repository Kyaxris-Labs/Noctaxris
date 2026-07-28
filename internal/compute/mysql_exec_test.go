package compute

import (
	"strings"
	"testing"
)

func TestExecMySQLSQLNilClient(t *testing.T) {
	c := &Client{}
	_, err := c.ExecMySQLSQL(t.Context(), MySQLSQLOpts{
		ContainerID: "cid",
		SQL:         "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected error for nil exec backend")
	}
}

func TestParseMySQLBatchTSV(t *testing.T) {
	cols, rows, err := parseMySQLBatchTSV("a\tb\n1\t2\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 2 || cols[0] != "a" || cols[1] != "b" {
		t.Fatalf("cols=%v", cols)
	}
	if len(rows) != 1 || rows[0][0] != "1" || rows[0][1] != "2" {
		t.Fatalf("rows=%v", rows)
	}
}

func TestParseMySQLAffectedRows(t *testing.T) {
	if n := parseMySQLAffectedRows("Query OK, 3 rows affected\n"); n != 3 {
		t.Fatalf("rows=%d", n)
	}
	if n := parseMySQLAffectedRows("Query OK, 1 row affected"); n != 1 {
		t.Fatalf("row=%d", n)
	}
}

func TestMySQLExecCmdNoPsqlFlags(t *testing.T) {
	cmd := mysqlExecCmd("root", "app", "SELECT 1")
	joined := strings.Join(cmd, " ")
	if strings.Contains(joined, "ON_ERROR_STOP") {
		t.Fatalf("psql flag leaked into mysql argv: %v", cmd)
	}
	for i, a := range cmd {
		if a == "-v" {
			t.Fatalf("unexpected -v at %d in %v", i, cmd)
		}
	}
	if len(cmd) < 7 || cmd[0] != "mysql" || cmd[len(cmd)-2] != "-e" {
		t.Fatalf("cmd=%v", cmd)
	}
}
