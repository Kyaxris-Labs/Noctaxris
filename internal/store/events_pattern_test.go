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
	if err := validateEventPatternDeep(`{`); err == nil {
		t.Fatal("want reject invalid JSON")
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

func TestMatchEventPatternEmptyInvalidAndNumericBounds(t *testing.T) {
	ok, err := matchEventPatternJSON(`{}`, "aws.s3", "t", `{}`)
	if err != nil || ok {
		t.Fatalf("empty pattern want miss err=%v ok=%v", err, ok)
	}
	if _, err := matchEventPatternJSON(`{`, "aws.s3", "t", `{}`); err == nil {
		t.Fatal("want invalid pattern JSON error")
	}
	if _, err := matchEventPatternJSON(`{"source":["aws.s3"]}`, "aws.s3", "t", `{`); err == nil {
		t.Fatal("want invalid detail JSON error")
	}
	ok, err = matchEventPatternJSON(`{"source":["aws.s3"]}`, "aws.s3", "t", "")
	if err != nil || !ok {
		t.Fatalf("empty detail ok with source match err=%v ok=%v", err, ok)
	}

	pat := `{"detail":{"n":[{"numeric":["=",2]},{"numeric":[">=",1,"<",3]}]}}`
	ok, err = matchEventPatternJSON(pat, "x", "y", `{"n":2}`)
	if err != nil || !ok {
		t.Fatalf("numeric bounds match err=%v ok=%v", err, ok)
	}
	ok, err = matchEventPatternJSON(pat, "x", "y", `{"n":3}`)
	if err != nil || ok {
		t.Fatalf("numeric bounds miss err=%v ok=%v", err, ok)
	}
	ok, err = matchEventPatternJSON(`{"detail":{"n":[{"numeric":[">",1]}]}}`, "x", "y", `{"n":"nope"}`)
	if err != nil || ok {
		t.Fatalf("non-numeric miss err=%v ok=%v", err, ok)
	}
}

func TestMatchEventPatternAnythingButListAndPrefixList(t *testing.T) {
	pat := `{"detail":{"state":[{"anything-but":["init","stop"]}],"region":[{"anything-but":{"prefix":["us-","eu-"]}}],"file":[{"anything-but":{"suffix":[".txt",".log"]}}]}}`
	ok, err := matchEventPatternJSON(pat, "x", "y", `{"state":"ready","region":"ap-south-1","file":"a.png"}`)
	if err != nil || !ok {
		t.Fatalf("want match err=%v ok=%v", err, ok)
	}
	ok, err = matchEventPatternJSON(pat, "x", "y", `{"state":"stop","region":"ap-south-1","file":"a.png"}`)
	if err != nil || ok {
		t.Fatalf("want miss state err=%v ok=%v", err, ok)
	}
	ok, err = matchEventPatternJSON(pat, "x", "y", `{"state":"ready","region":"us-west-2","file":"a.png"}`)
	if err != nil || ok {
		t.Fatalf("want miss region prefix list err=%v ok=%v", err, ok)
	}
	ok, err = matchEventPatternJSON(pat, "x", "y", `{"state":"ready","region":"ap-south-1","file":"a.log"}`)
	if err != nil || ok {
		t.Fatalf("want miss file suffix list err=%v ok=%v", err, ok)
	}
}

func TestPatternValueMatchesAndValidateOperatorEdges(t *testing.T) {
	if !patternValueMatches([]any{"ready", "go"}, "ready") {
		t.Fatal("want list match")
	}
	if patternValueMatches([]any{"ready"}, "stop") {
		t.Fatal("want list miss")
	}
	if !patternValueMatches(map[string]any{"prefix": "lab-"}, "lab-1") {
		t.Fatal("want prefix via patternValueMatches")
	}
	if patternValueMatches(map[string]any{"unknown": true}, "x") {
		t.Fatal("unknown operator fail closed")
	}
	if patternValueMatches(map[string]any{"a": 1, "b": 2}, "x") {
		t.Fatal("multi-key operator object fail closed")
	}
	if patternValueMatches(map[string]any{"exists": "yes"}, "x") {
		t.Fatal("exists non-bool fail closed")
	}
	if patternValueMatches(map[string]any{"prefix": true}, "x") {
		t.Fatal("prefix non-string fail closed")
	}
	if patternValueMatches(map[string]any{"suffix": true}, "x") {
		t.Fatal("suffix non-string fail closed")
	}
	if patternValueMatches(map[string]any{"equals-ignore-case": true}, "x") {
		t.Fatal("equals-ignore-case non-string fail closed")
	}
	if patternValueMatches(map[string]any{"numeric": []any{">"}}, 2) {
		t.Fatal("numeric odd array fail closed")
	}
	if patternValueMatches(map[string]any{"numeric": []any{"~", 1}}, 2) {
		t.Fatal("numeric bad op fail closed")
	}
	if patternValueMatches(map[string]any{"numeric": []any{">", "x"}}, 2) {
		t.Fatal("numeric bad bound fail closed")
	}
	if patternValueMatches(map[string]any{"anything-but": map[string]any{"a": 1, "b": 2}}, "x") {
		t.Fatal("anything-but multi-key fail closed")
	}
	if patternValueMatches(map[string]any{"anything-but": map[string]any{"cidr": "10.0.0.0/8"}}, "x") {
		t.Fatal("anything-but unknown nested fail closed")
	}
	if !patternValueMatches(map[string]any{"numeric": []any{"=", 2.5}}, 2.5) {
		t.Fatal("want float numeric match")
	}
	if !patternValueMatches(map[string]any{"numeric": []any{">=", 1, "<=", 1}}, 1) {
		t.Fatal("want >= <= numeric match")
	}
	if patternValueMatches(map[string]any{"prefix": "lab-"}, nil) {
		t.Fatal("prefix missing present=true still needs string actual")
	}

	ok, _ := matchEventPatternJSON(`{"$or":[{"source":["a"]}]}`, "a", "t", `{}`)
	if ok {
		t.Fatal("$or at match time fail closed")
	}
	ok, _ = matchEventPatternJSON(`{"detail":{"x":1}}`, "a", "t", `{"x":1}`)
	if ok {
		t.Fatal("non-array leaf pattern fail closed")
	}
	ok, _ = matchEventPatternJSON(`{"detail":{"n":[{"numeric":[">",1]}]}}`, "a", "t", `{}`)
	if ok {
		t.Fatal("numeric missing field fail closed")
	}
	ok, _ = matchEventPatternJSON(`{"detail":{"s":[{"prefix":"a"}]}}`, "a", "t", `{}`)
	if ok {
		t.Fatal("prefix missing field fail closed")
	}
	ok, _ = matchEventPatternJSON(`{"detail":{"s":[{"suffix":"a"}]}}`, "a", "t", `{}`)
	if ok {
		t.Fatal("suffix missing field fail closed")
	}
	ok, _ = matchEventPatternJSON(`{"detail":{"s":[{"equals-ignore-case":"a"}]}}`, "a", "t", `{}`)
	if ok {
		t.Fatal("equals-ignore-case missing field fail closed")
	}
	ok, _ = matchEventPatternJSON(`{"detail":{"s":[{"anything-but":"a"}]}}`, "a", "t", `{}`)
	if ok {
		t.Fatal("anything-but missing field fail closed")
	}
	// non-string actual: stringHasAnyPrefix returns false, so anything-but prefix matches
	ok, _ = matchEventPatternJSON(`{"detail":{"s":[{"anything-but":{"prefix":"a"}}]}}`, "a", "t", `{"s":1}`)
	if !ok {
		t.Fatal("anything-but prefix against non-string should match")
	}
	ok, _ = matchEventPatternJSON(`{"detail":{"s":[{"anything-but":{"suffix":[".txt"]}}]}}`, "a", "t", `{"s":1}`)
	if !ok {
		t.Fatal("anything-but suffix list against non-string should match")
	}

	if err := validateEventPatternDeep(`[]`); err == nil {
		t.Fatal("want reject non-object pattern")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":1}}`); err == nil {
		t.Fatal("want reject non-array field")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"prefix":1}]}}`); err == nil {
		t.Fatal("want reject non-string prefix")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"exists":"yes"}]}}`); err == nil {
		t.Fatal("want reject non-bool exists")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"numeric":[">"]}]}}`); err == nil {
		t.Fatal("want reject odd numeric array")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"numeric":["~",1]}]}}`); err == nil {
		t.Fatal("want reject bad numeric op")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"numeric":[1,2]}]}}`); err == nil {
		t.Fatal("want reject non-string numeric comparison")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"anything-but":{"prefix":[1]}}]}}`); err == nil {
		t.Fatal("want reject anything-but prefix non-string list")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"anything-but":{"prefix":"a","suffix":"b"}}]}}`); err == nil {
		t.Fatal("want reject multi-key anything-but object")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"anything-but":{"suffix":true}}]}}`); err == nil {
		t.Fatal("want reject anything-but suffix bad type")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"anything-but":{"equals-ignore-case":"x"}}]}}`); err == nil {
		t.Fatal("want reject anything-but equals-ignore-case")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"anything-but":[{"a":1}]}]}}`); err == nil {
		t.Fatal("want reject anything-but list of objects")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"equals-ignore-case":1}]}}`); err == nil {
		t.Fatal("want reject non-string equals-ignore-case")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[[1]]}}`); err == nil {
		t.Fatal("want reject nested array matcher")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"prefix":{"equals-ignore-case":"x"}}]}}`); err == nil {
		t.Fatal("want reject prefix nested object")
	}
	if err := validateEventPatternDeep(`{"detail":{"f":[{"bogus":1}]}}`); err == nil {
		t.Fatal("want reject unknown operator")
	}

	cases := []struct {
		v  any
		ok bool
	}{
		{float32(1.5), true},
		{int(3), true},
		{int64(4), true},
		{"2.5", true},
		{struct{}{}, false},
	}
	for _, tc := range cases {
		_, ok := asFloat64(tc.v)
		if ok != tc.ok {
			t.Fatalf("asFloat64(%T)=%v want %v", tc.v, ok, tc.ok)
		}
	}
	if !jsonValueEqual(2.0, float64(2)) {
		t.Fatal("normalize int-like float equality")
	}
	if !jsonValueEqual(map[string]any{"a": 1.0}, map[string]any{"a": float64(1)}) {
		t.Fatal("normalize nested map equality")
	}
	if !jsonValueEqual([]any{1.0}, []any{float64(1)}) {
		t.Fatal("normalize nested array equality")
	}
}
