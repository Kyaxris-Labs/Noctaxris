package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSSMParameterVersionLabelsAndHistory(t *testing.T) {
	st := openSSMStore(t)
	account := "000000000001"

	if _, err := st.PutParameter(account, "us-east-1", "/app/ami", store.ParamTypeString, "ami-v1", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutParameter(account, "us-east-1", "/app/ami", store.ParamTypeString, "ami-v2", "", true); err != nil {
		t.Fatal(err)
	}

	hist, err := st.GetParameterHistory(account, "/app/ami", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len=%d want 2: %+v", len(hist), hist)
	}
	if hist[0].Value != "ami-v1" || hist[0].Version != 1 {
		t.Fatalf("v1=%+v", hist[0])
	}
	if hist[1].Value != "ami-v2" || hist[1].Version != 2 {
		t.Fatalf("v2=%+v", hist[1])
	}

	labeled, err := st.LabelParameterVersion(account, "/app/ami", 1, []string{"Test", "1bad", "awsCurrent"})
	if err != nil {
		t.Fatal(err)
	}
	if labeled.ParameterVersion != 1 {
		t.Fatalf("ParameterVersion=%d", labeled.ParameterVersion)
	}
	if len(labeled.InvalidLabels) != 2 {
		t.Fatalf("InvalidLabels=%v want 1bad and awsCurrent", labeled.InvalidLabels)
	}

	byLabel, err := st.GetParameterResolved(account, store.ParseParameterSelector("/app/ami:Test", 0, ""), true)
	if err != nil {
		t.Fatal(err)
	}
	if byLabel.Value != "ami-v1" || byLabel.Version != 1 || byLabel.Selector != ":Test" {
		t.Fatalf("by label=%+v", byLabel)
	}

	byVersion, err := st.GetParameterResolved(account, store.ParseParameterSelector("/app/ami:2", 0, ""), true)
	if err != nil {
		t.Fatal(err)
	}
	if byVersion.Value != "ami-v2" || byVersion.Selector != ":2" {
		t.Fatalf("by version=%+v", byVersion)
	}

	byField, err := st.GetParameterResolved(account, store.ParseParameterSelector("/app/ami", 1, ""), true)
	if err != nil {
		t.Fatal(err)
	}
	if byField.Value != "ami-v1" || byField.Selector != ":1" {
		t.Fatalf("by Version field=%+v", byField)
	}

	// Move label Test from v1 to v2 (AWS-like overwrite).
	moved, err := st.LabelParameterVersion(account, "/app/ami", 2, []string{"Test", "Production"})
	if err != nil {
		t.Fatal(err)
	}
	if moved.ParameterVersion != 2 || len(moved.InvalidLabels) != 0 {
		t.Fatalf("moved=%+v", moved)
	}

	hist, err = st.GetParameterHistory(account, "/app/ami", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist[0].Labels) != 0 {
		t.Fatalf("v1 labels after move=%v want empty", hist[0].Labels)
	}
	if len(hist[1].Labels) != 2 {
		t.Fatalf("v2 labels=%v want Test,Production", hist[1].Labels)
	}

	prod, err := st.GetParameterResolved(account, store.ParseParameterSelector("/app/ami", 0, "Production"), true)
	if err != nil {
		t.Fatal(err)
	}
	if prod.Value != "ami-v2" || prod.Selector != ":Production" {
		t.Fatalf("by Label field=%+v", prod)
	}

	batch, err := st.GetParametersResolved(account, []string{"/app/ami:Test", "/app/ami:99", "/missing"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 1 || batch[0].Value != "ami-v2" {
		t.Fatalf("batch=%+v", batch)
	}

	if _, err := st.GetParameterResolved(account, store.ParseParameterSelector("/app/ami:9", 0, ""), true); !errors.Is(err, store.ErrParameterVersionNotFound) {
		t.Fatalf("want ErrParameterVersionNotFound, got %v", err)
	}
}

func TestSSMLabelParameterVersionLatestAndLimit(t *testing.T) {
	st := openSSMStore(t)
	account := "000000000001"

	if _, err := st.PutParameter(account, "us-east-1", "/lim", store.ParamTypeString, "v", "", false); err != nil {
		t.Fatal(err)
	}
	labels := make([]string, 0, store.MaxSSMParameterLabels)
	for i := 0; i < store.MaxSSMParameterLabels; i++ {
		labels = append(labels, string(rune('A'+i)))
	}
	if _, err := st.LabelParameterVersion(account, "/lim", 0, labels); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LabelParameterVersion(account, "/lim", 0, []string{"extra"}); !errors.Is(err, store.ErrParameterVersionLabelLimitExceeded) {
		t.Fatalf("want ErrParameterVersionLabelLimitExceeded, got %v", err)
	}
}

func TestParseParameterSelector(t *testing.T) {
	sel := store.ParseParameterSelector("/cfg/endpoint:stable", 0, "")
	if sel.Name != "/cfg/endpoint" || sel.Label != "stable" || sel.Selector != ":stable" {
		t.Fatalf("label path=%+v", sel)
	}
	sel = store.ParseParameterSelector("cfg/endpoint:3", 0, "")
	if sel.Name != "/cfg/endpoint" || sel.Version != 3 || sel.Selector != ":3" {
		t.Fatalf("version path=%+v", sel)
	}
	sel = store.ParseParameterSelector("/cfg/endpoint", 0, "prod")
	if sel.Label != "prod" || sel.Selector != ":prod" {
		t.Fatalf("label field=%+v", sel)
	}
	// Name-path selector wins over Version/Label fields.
	sel = store.ParseParameterSelector("/cfg/endpoint:1", 9, "ignored")
	if sel.Version != 1 || sel.Label != "" {
		t.Fatalf("path wins=%+v", sel)
	}
}
