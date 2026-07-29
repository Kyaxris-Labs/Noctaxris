package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAthenaWorkGroupCRUD(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	primary, err := st.GetAthenaWorkGroup(account, "primary")
	if err != nil {
		t.Fatal(err)
	}
	if primary.Name != "primary" || primary.State != "ENABLED" {
		t.Fatalf("primary=%#v", primary)
	}

	if _, err := st.CreateAthenaWorkGroup(account, store.AthenaWorkGroupCreate{Name: ""}); !errors.Is(err, store.ErrAthenaBadRequest) {
		t.Fatalf("empty name: %v", err)
	}

	wg, err := st.CreateAthenaWorkGroup(account, store.AthenaWorkGroupCreate{
		Name:                   "analytics",
		Description:            "lab wg",
		OutputLocation:         "s3://athena-wg/out/",
		EnforceWorkGroupConfig: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if wg.Name != "analytics" || !wg.EnforceWorkGroupConfig || wg.OutputLocation != "s3://athena-wg/out/" {
		t.Fatalf("created=%#v", wg)
	}

	if _, err := st.CreateAthenaWorkGroup(account, store.AthenaWorkGroupCreate{Name: "analytics"}); !errors.Is(err, store.ErrAthenaBadRequest) {
		t.Fatalf("duplicate: %v", err)
	}

	got, err := st.GetAthenaWorkGroup(account, "analytics")
	if err != nil || got.Description != "lab wg" {
		t.Fatalf("get err=%v got=%#v", err, got)
	}

	listed, err := st.ListAthenaWorkGroups(account)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) < 2 {
		t.Fatalf("list=%#v", listed)
	}
	names := map[string]bool{}
	for _, g := range listed {
		names[g.Name] = true
	}
	if !names["primary"] || !names["analytics"] {
		t.Fatalf("list names=%v", names)
	}

	desc := "updated"
	state := "DISABLED"
	enforce := false
	out := "s3://athena-wg/new/"
	updated, err := st.UpdateAthenaWorkGroup(account, "analytics", store.AthenaWorkGroupUpdate{
		Description:            &desc,
		State:                  &state,
		OutputLocation:         &out,
		EnforceWorkGroupConfig: &enforce,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != "updated" || updated.State != "DISABLED" || updated.OutputLocation != out || updated.EnforceWorkGroupConfig {
		t.Fatalf("updated=%#v", updated)
	}

	if err := st.DeleteAthenaWorkGroup(account, "primary"); !errors.Is(err, store.ErrAthenaBadRequest) {
		t.Fatalf("delete primary: %v", err)
	}
	if err := st.DeleteAthenaWorkGroup(account, "analytics"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetAthenaWorkGroup(account, "analytics"); !errors.Is(err, store.ErrAthenaNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestAthenaStartUsesWorkGroupOutputLocation(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	if _, err := st.CreateAthenaWorkGroup(account, store.AthenaWorkGroupCreate{
		Name:                   "enforced",
		OutputLocation:         "s3://wg-out/",
		EnforceWorkGroupConfig: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAthenaWorkGroup(account, store.AthenaWorkGroupCreate{
		Name:           "soft",
		OutputLocation: "s3://soft-out/",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAthenaWorkGroup(account, store.AthenaWorkGroupCreate{Name: "off"}); err != nil {
		t.Fatal(err)
	}
	off := "DISABLED"
	if _, err := st.UpdateAthenaWorkGroup(account, "off", store.AthenaWorkGroupUpdate{State: &off}); err != nil {
		t.Fatal(err)
	}

	if _, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT 1",
		WorkGroup:   "off",
	}); !errors.Is(err, store.ErrAthenaBadRequest) || !strings.Contains(err.Error(), "DISABLED") {
		t.Fatalf("disabled wg: %v", err)
	}

	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString:    "SELECT id FROM missingdb.t",
		WorkGroup:      "enforced",
		OutputLocation: "s3://client-out/",
		Database:       "missingdb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if exec.OutputLocation != "s3://wg-out/" {
		t.Fatalf("enforce output=%q", exec.OutputLocation)
	}

	exec2, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT id FROM missingdb.t",
		WorkGroup:   "soft",
		Database:    "missingdb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if exec2.OutputLocation != "s3://soft-out/" {
		t.Fatalf("soft fill output=%q", exec2.OutputLocation)
	}

	exec3, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString:    "SELECT id FROM missingdb.t",
		WorkGroup:      "soft",
		OutputLocation: "s3://client-keep/",
		Database:       "missingdb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if exec3.OutputLocation != "s3://client-keep/" {
		t.Fatalf("soft keep output=%q", exec3.OutputLocation)
	}
}
