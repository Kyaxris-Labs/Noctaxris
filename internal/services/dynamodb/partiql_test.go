package dynamodb_test

import (
	"testing"

	ddb "github.com/Kyaxris-Labs/Noctaxris/internal/services/dynamodb"
)

func TestParsePartiQLInsertSelectDelete(t *testing.T) {
	params := []any{
		map[string]any{"S": "Acme"},
		map[string]any{"S": "Song"},
	}
	op, err := ddb.ParsePartiQLStatement(`INSERT INTO Music VALUE {'Artist':?,'SongTitle':?}`, params)
	if err != nil {
		t.Fatal(err)
	}
	if op.Kind != "INSERT" || op.TableName != "Music" || op.Item["Artist"]["S"] != "Acme" {
		t.Fatalf("op=%+v", op)
	}

	op, err = ddb.ParsePartiQLStatement(`SELECT * FROM Music WHERE Artist=? AND SongTitle=?`, []any{
		map[string]any{"S": "Acme"},
		map[string]any{"S": "Song"},
	})
	if err != nil || op.Kind != "SELECT" || op.Key["Artist"]["S"] != "Acme" {
		t.Fatalf("select op=%+v err=%v", op, err)
	}

	op, err = ddb.ParsePartiQLStatement(`DELETE FROM Music WHERE Artist=?`, []any{map[string]any{"S": "Acme"}})
	if err != nil || op.Kind != "DELETE" {
		t.Fatalf("delete op=%+v err=%v", op, err)
	}

	op, err = ddb.ParsePartiQLStatement(`UPDATE Music SET Awards=? WHERE Artist=?`, []any{
		map[string]any{"N": "1"},
		map[string]any{"S": "Acme"},
	})
	if err != nil || op.Kind != "UPDATE" || op.SetAttrs["Awards"].(map[string]any)["N"] != "1" {
		t.Fatalf("update op=%+v err=%v", op, err)
	}
}

func TestParsePartiQLFailClosed(t *testing.T) {
	cases := []string{
		`SELECT * FROM A JOIN B ON A.id=B.id WHERE A.id=?`,
		`SELECT * FROM A WHERE id IN (?)`,
		`SELECT * FROM A WHERE id=(SELECT id FROM B)`,
		`SELECT * FROM A GROUP BY id`,
	}
	for _, stmt := range cases {
		if _, err := ddb.ParsePartiQLStatement(stmt, nil); err == nil {
			t.Fatalf("want error for %q", stmt)
		}
	}
}
