package store

import (
	"path/filepath"
	"testing"
)

func openPatternStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	key, err := LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestMatchEventPatternPrefixAndExists(t *testing.T) {
	pat := `{"source":["aws.s3"],"detail":{"bucket":{"name":[{"prefix":"lab-"}]},"reason":[{"exists":false}]}}`
	ok, err := matchEventPatternJSON(pat, "aws.s3", "Object Created", `{"bucket":{"name":"lab-1"},"obj":1}`)
	if err != nil || !ok {
		t.Fatalf("want match err=%v ok=%v", err, ok)
	}
	ok, err = matchEventPatternJSON(pat, "aws.s3", "Object Created", `{"bucket":{"name":"prod-1"}}`)
	if err != nil || ok {
		t.Fatalf("want miss err=%v ok=%v", err, ok)
	}
}

func TestMatchEventPatternAnythingButAndNumeric(t *testing.T) {
	pat := `{"detail":{"state":[{"anything-but":"init"}],"n":[{"numeric":[">",1,"<=",3]}]}}`
	ok, _ := matchEventPatternJSON(pat, "x", "y", `{"state":"ready","n":2}`)
	if !ok {
		t.Fatal("want match")
	}
	ok, _ = matchEventPatternJSON(pat, "x", "y", `{"state":"init","n":2}`)
	if ok {
		t.Fatal("want miss on anything-but")
	}
}

func TestMatchEventPatternSuffixAndEqualsIgnoreCase(t *testing.T) {
	pat := `{"detail":{"file":[{"suffix":".png"}],"user":[{"equals-ignore-case":"alice"}]}}`
	ok, err := matchEventPatternJSON(pat, "x", "y", `{"file":"shot.PNG","user":"Alice"}`)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("suffix is case-sensitive; .PNG should not match .png")
	}
	ok, err = matchEventPatternJSON(pat, "x", "y", `{"file":"shot.png","user":"ALICE"}`)
	if err != nil || !ok {
		t.Fatalf("want match err=%v ok=%v", err, ok)
	}
}

func TestMatchEventPatternAnythingButPrefixSuffix(t *testing.T) {
	pat := `{"detail":{"region":[{"anything-but":{"prefix":"us-"}}],"file":[{"anything-but":{"suffix":".txt"}}]}}`
	ok, err := matchEventPatternJSON(pat, "x", "y", `{"region":"eu-west-1","file":"a.png"}`)
	if err != nil || !ok {
		t.Fatalf("want match err=%v ok=%v", err, ok)
	}
	ok, err = matchEventPatternJSON(pat, "x", "y", `{"region":"us-east-1","file":"a.png"}`)
	if err != nil || ok {
		t.Fatalf("want miss on prefix err=%v ok=%v", err, ok)
	}
}

func TestMatchEventPatternExistsTrueFalse(t *testing.T) {
	patTrue := `{"detail":{"state":[{"exists":true}]}}`
	ok, err := matchEventPatternJSON(patTrue, "x", "y", `{"state":"ready"}`)
	if err != nil || !ok {
		t.Fatalf("exists true present: err=%v ok=%v", err, ok)
	}
	ok, err = matchEventPatternJSON(patTrue, "x", "y", `{"other":1}`)
	if err != nil || ok {
		t.Fatalf("exists true missing: err=%v ok=%v", err, ok)
	}

	patFalse := `{"detail":{"state":[{"exists":false}]}}`
	ok, err = matchEventPatternJSON(patFalse, "x", "y", `{"other":1}`)
	if err != nil || !ok {
		t.Fatalf("exists false missing: err=%v ok=%v", err, ok)
	}
	ok, err = matchEventPatternJSON(patFalse, "x", "y", `{"state":"ready"}`)
	if err != nil || ok {
		t.Fatalf("exists false present: err=%v ok=%v", err, ok)
	}
}

func TestMatchEventPatternExactOrListAndNested(t *testing.T) {
	pat := `{"source":["a","b"],"detail":{"a":{"b":["x","y"]}}}`
	ok, err := matchEventPatternJSON(pat, "b", "t", `{"a":{"b":"y"}}`)
	if err != nil || !ok {
		t.Fatalf("want match err=%v ok=%v", err, ok)
	}
	ok, err = matchEventPatternJSON(pat, "c", "t", `{"a":{"b":"y"}}`)
	if err != nil || ok {
		t.Fatalf("want miss source err=%v ok=%v", err, ok)
	}
}

func TestValidateEventPatternRejectsWildcard(t *testing.T) {
	err := validateEventPatternDeep(`{"detail":{"f":[{"wildcard":"*.png"}]}}`)
	if err == nil {
		t.Fatal("want reject wildcard")
	}
}

func TestValidateEventPatternRejectsOrAndCIDR(t *testing.T) {
	if err := validateEventPatternDeep(`{"$or":[{"source":["a"]},{"source":["b"]}]}`); err == nil {
		t.Fatal("want reject $or")
	}
	if err := validateEventPatternDeep(`{"detail":{"ip":[{"cidr":"10.0.0.0/24"}]}}`); err == nil {
		t.Fatal("want reject cidr")
	}
}

func TestValidateEventPatternRejectsAnythingButWildcard(t *testing.T) {
	err := validateEventPatternDeep(`{"detail":{"f":[{"anything-but":{"wildcard":"*/lib/*"}}]}}`)
	if err == nil {
		t.Fatal("want reject anything-but wildcard")
	}
}

func TestValidateEventPatternRejectsPrefixIgnoreCaseCombo(t *testing.T) {
	err := validateEventPatternDeep(`{"detail":{"s":[{"prefix":{"equals-ignore-case":"eventb"}}]}}`)
	if err == nil {
		t.Fatal("want reject prefix equals-ignore-case combo")
	}
}

func TestValidateEventPatternAcceptsShippedOperators(t *testing.T) {
	pat := `{
		"source":["aws.s3"],
		"detail-type":[{"equals-ignore-case":"object created"}],
		"detail":{
			"bucket":{"name":[{"prefix":"lab-"}]},
			"key":[{"suffix":".png"}],
			"state":[{"anything-but":["init","stop"]}],
			"n":[{"numeric":[">",0,"<=",10]}],
			"missing":[{"exists":false}]
		}
	}`
	if err := validateEventPatternDeep(pat); err != nil {
		t.Fatalf("want accept shipped operators: %v", err)
	}
}

func TestPutRuleRejectsUnsupportedPatternOperator(t *testing.T) {
	st := openPatternStore(t)
	_, err := st.PutRule("000000000001", "us-east-1", "default", "bad-rule",
		`{"detail":{"f":[{"wildcard":"*.png"}]}}`, "", RuleStateEnabled)
	if err == nil {
		t.Fatal("want PutRule reject wildcard")
	}
}
