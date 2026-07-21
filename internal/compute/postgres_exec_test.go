package compute

import (
	"strings"
	"testing"
)

func TestIsPostgresSelect(t *testing.T) {
	if !isPostgresSelect("SELECT 1") || !isPostgresSelect("  with x as (select 1) select * from x") {
		t.Fatal("expected select/with")
	}
	if isPostgresSelect("INSERT INTO t VALUES (1)") {
		t.Fatal("insert is not select")
	}
}

func TestParsePostgresCSV(t *testing.T) {
	cols, rows, err := parsePostgresCSV("stub,n\nok,1\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 2 || cols[0] != "stub" || cols[1] != "n" {
		t.Fatalf("cols=%v", cols)
	}
	if len(rows) != 1 || rows[0][0] != "ok" || rows[0][1] != "1" {
		t.Fatalf("rows=%v", rows)
	}
}

func TestParsePostgresCommandTagUpdated(t *testing.T) {
	if parsePostgresCommandTagUpdated("INSERT 0 1") != 1 {
		t.Fatal("insert")
	}
	if parsePostgresCommandTagUpdated("UPDATE 3") != 3 {
		t.Fatal("update")
	}
	if parsePostgresCommandTagUpdated("") != 1 {
		t.Fatal("empty default")
	}
}

func TestExecPostgresSQLNilClient(t *testing.T) {
	var c *Client
	_, err := c.ExecPostgresSQL(t.Context(), PostgresSQLOpts{
		ContainerID: "abc",
		SQL:         "SELECT 1",
	})
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("err=%v", err)
	}
}

func TestExecNilClient(t *testing.T) {
	var c *Client
	_, err := c.Exec(t.Context(), ExecOpts{ContainerID: "x", Cmd: []string{"true"}})
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("err=%v", err)
	}
}
