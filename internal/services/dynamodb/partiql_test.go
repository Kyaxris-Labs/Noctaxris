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

	lit, err := ddb.ParsePartiQLStatement(`INSERT INTO T VALUE {'Name':'Acme','Count':42,'Ok':true}`, nil)
	if err != nil || lit.Item["Name"]["S"] != "Acme" || lit.Item["Count"]["N"] != "42" {
		t.Fatalf("literal insert=%+v err=%v", lit, err)
	}

	strParam, err := ddb.ParsePartiQLStatement(`INSERT INTO T VALUE {'A':?}`, []any{"hello"})
	if err != nil || strParam.Item["A"]["S"] != "hello" {
		t.Fatalf("str param=%+v err=%v", strParam, err)
	}
	numParam, err := ddb.ParsePartiQLStatement(`INSERT INTO T VALUE {'N':?}`, []any{float64(7)})
	if err != nil || numParam.Item["N"]["N"] != "7" {
		t.Fatalf("num param=%+v err=%v", numParam, err)
	}
	fracParam, err := ddb.ParsePartiQLStatement(`INSERT INTO T VALUE {'F':?}`, []any{3.14})
	if err != nil || fracParam.Item["F"]["N"] != "3.14" {
		t.Fatalf("frac param=%+v err=%v", fracParam, err)
	}
	boolParam, err := ddb.ParsePartiQLStatement(`INSERT INTO T VALUE {'B':?}`, []any{false})
	if err != nil || boolParam.Item["B"]["BOOL"] != false {
		t.Fatalf("bool param=%+v err=%v", boolParam, err)
	}
	nullParam, err := ddb.ParsePartiQLStatement(`INSERT INTO T VALUE {'Z':?}`, []any{nil})
	if err != nil || nullParam.Item["Z"]["NULL"] != true {
		t.Fatalf("null param=%+v err=%v", nullParam, err)
	}
	if _, err := ddb.ParsePartiQLStatement(`INSERT INTO T VALUE {'X':?}`, []any{struct{}{}}); err == nil {
		t.Fatal("expected unsupported parameter type")
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
