package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCodeBuildWebhookCRUDAndFilterValidation(t *testing.T) {
	st := openCodeBuildStore(t)
	account := "000000000001"
	region := store.DefaultCodeBuildRegion

	if err := store.EnsureCodeBuildWebhookSchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}

	_, err := st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
		Name:        "wh-proj",
		ServiceRole: "arn:aws:iam::" + account + ":role/CodeBuildRole",
		SourceType:  "NO_SOURCE",
		Buildspec:   "echo ok",
		Image:       "alpine:3.20",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = st.UpsertCodeBuildWebhook("", store.CodeBuildWebhook{ProjectName: "wh-proj"})
	if !errors.Is(err, store.ErrCodeBuildInvalidInput) {
		t.Fatalf("empty account err=%v", err)
	}
	_, err = st.UpsertCodeBuildWebhook(account, store.CodeBuildWebhook{ProjectName: ""})
	if !errors.Is(err, store.ErrCodeBuildInvalidInput) {
		t.Fatalf("empty project err=%v", err)
	}
	_, err = st.UpsertCodeBuildWebhook(account, store.CodeBuildWebhook{ProjectName: "missing-proj"})
	if err == nil {
		t.Fatal("missing project must fail")
	}
	_, err = st.UpsertCodeBuildWebhook(account, store.CodeBuildWebhook{
		ProjectName:      "wh-proj",
		FilterGroupsJSON: `{bad`,
	})
	if !errors.Is(err, store.ErrCodeBuildInvalidInput) {
		t.Fatalf("bad filters err=%v", err)
	}
	_, err = st.UpsertCodeBuildWebhook(account, store.CodeBuildWebhook{
		ProjectName:      "wh-proj",
		FilterGroupsJSON: `[[{"type":"BRANCH","pattern":"main"}]]`,
	})
	if !errors.Is(err, store.ErrCodeBuildInvalidInput) {
		t.Fatalf("bad type err=%v", err)
	}
	_, err = st.UpsertCodeBuildWebhook(account, store.CodeBuildWebhook{
		ProjectName:      "wh-proj",
		FilterGroupsJSON: `[[{"type":"EVENT","pattern":""}]]`,
	})
	if !errors.Is(err, store.ErrCodeBuildInvalidInput) {
		t.Fatalf("empty pattern err=%v", err)
	}
	_, err = st.UpsertCodeBuildWebhook(account, store.CodeBuildWebhook{
		ProjectName:      "wh-proj",
		FilterGroupsJSON: `[[{"type":"HEAD_REF","pattern":"("}]]`,
	})
	if !errors.Is(err, store.ErrCodeBuildInvalidInput) {
		t.Fatalf("bad regex err=%v", err)
	}

	filters := `[[{"type":"EVENT","pattern":"PUSH"},{"type":"HEAD_REF","pattern":"^refs/heads/main$"}],[{"type":"FILE_PATH","pattern":"^src/.*"}]]`
	wh, err := st.UpsertCodeBuildWebhook(account, store.CodeBuildWebhook{
		ProjectName:      "wh-proj",
		FilterGroupsJSON: filters,
		Secret:           "lab-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if wh.PayloadJSON != "{}" || wh.Secret != "lab-secret" {
		t.Fatalf("wh=%+v", wh)
	}
	got, err := st.GetCodeBuildWebhook(account, "wh-proj")
	if err != nil || got.FilterGroupsJSON != filters {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	_, err = st.GetCodeBuildWebhook(account, "nope")
	if !errors.Is(err, store.ErrCodeBuildWebhookNotFound) {
		t.Fatalf("get missing err=%v", err)
	}

	list, err := st.ListCodeBuildWebhooks(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.DeleteCodeBuildWebhook(account, "wh-proj"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCodeBuildWebhook(account, "wh-proj"); !errors.Is(err, store.ErrCodeBuildWebhookNotFound) {
		t.Fatalf("delete missing err=%v", err)
	}
}

func TestMatchCodeBuildWebhookFilters(t *testing.T) {
	ok, err := store.MatchCodeBuildWebhookFilters("", store.CodeBuildWebhookEvent{Event: "PUSH"})
	if err != nil || !ok {
		t.Fatalf("empty groups ok=%v err=%v", ok, err)
	}
	ok, err = store.MatchCodeBuildWebhookFilters("[]", store.CodeBuildWebhookEvent{Event: "PUSH"})
	if err != nil || !ok {
		t.Fatalf("empty array ok=%v err=%v", ok, err)
	}
	_, err = store.MatchCodeBuildWebhookFilters("{bad", store.CodeBuildWebhookEvent{})
	if err == nil {
		t.Fatal("bad json must fail")
	}

	ev := store.CodeBuildWebhookEvent{
		Event:     "PUSH",
		HeadRef:   "refs/heads/main",
		FilePaths: []string{"docs/readme.md"},
	}
	ok, err = store.MatchCodeBuildWebhookFilters(
		`[[{"type":"EVENT","pattern":"PUSH,PULL_REQUEST_CREATED"},{"type":"HEAD_REF","pattern":"^refs/heads/main$"}]]`,
		ev,
	)
	if err != nil || !ok {
		t.Fatalf("and-group ok=%v err=%v", ok, err)
	}
	ok, err = store.MatchCodeBuildWebhookFilters(
		`[[{"type":"FILE_PATH","pattern":"^src/"}],[{"type":"EVENT","pattern":"PUSH"}]]`,
		ev,
	)
	if err != nil || !ok {
		t.Fatalf("or-group ok=%v err=%v", ok, err)
	}
	ok, err = store.MatchCodeBuildWebhookFilters(
		`[[{"type":"EVENT","pattern":"PUSH","excludeMatchedPattern":true}]]`,
		ev,
	)
	if err != nil || ok {
		t.Fatalf("exclude ok=%v err=%v", ok, err)
	}
	ok, err = store.MatchCodeBuildWebhookFilters(
		`[[{"type":"FILE_PATH","pattern":"^src/"}]]`,
		ev,
	)
	if err != nil || ok {
		t.Fatalf("file miss ok=%v err=%v", ok, err)
	}
	_, err = store.MatchCodeBuildWebhookFilters(
		`[[{"type":"WEIRD","pattern":"x"}]]`,
		ev,
	)
	if err == nil {
		t.Fatal("unsupported type must fail at match")
	}
}
