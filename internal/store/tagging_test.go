package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openTaggingStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestTaggingTagUntagGet(t *testing.T) {
	st := openTaggingStore(t)
	account := "000000000001"
	arn := "arn:aws:s3:::lab-bucket"
	arn2 := "arn:aws:sqs:us-east-1:000000000001:lab-queue"

	failed, err := st.TagResources(account, []string{arn, arn2}, map[string]string{
		"env":  "lab",
		"team": "core",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 0 {
		t.Fatalf("failed=%v", failed)
	}

	got, err := st.GetResources(account, []store.TagFilter{{Key: "env", Values: []string{"lab"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d want 2 got=%+v", len(got), got)
	}

	sqsOnly, err := st.GetResources(account, nil, []string{"sqs"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sqsOnly) != 1 || sqsOnly[0].ResourceARN != arn2 {
		t.Fatalf("sqsOnly=%+v", sqsOnly)
	}

	failed, err = st.UntagResources(account, []string{arn}, []string{"team"})
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 0 {
		t.Fatalf("failed=%v", failed)
	}

	after, err := st.GetResources(account, []store.TagFilter{{Key: "team"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].ResourceARN != arn2 {
		t.Fatalf("after=%+v", after)
	}

	listed, err := st.ListResourceTags(account, arn2)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("ListResourceTags=%+v want env+team", listed)
	}
	byKey := map[string]string{}
	for _, tag := range listed {
		byKey[tag.Key] = tag.Value
	}
	if byKey["env"] != "lab" || byKey["team"] != "core" {
		t.Fatalf("byKey=%v", byKey)
	}
}

func TestTaggingRejectsForeignAccount(t *testing.T) {
	st := openTaggingStore(t)
	failed, err := st.TagResources("000000000001", []string{
		"arn:aws:sqs:us-east-1:999999999999:other",
	}, map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Fatalf("failed=%v", failed)
	}
}
