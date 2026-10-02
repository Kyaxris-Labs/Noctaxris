package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestIoTJobLifecyclePositiveNegativeBoundary(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000001"
	const region = "us-east-1"

	if _, err := st.CreateIoTThing(account, region, "job-device", nil); err != nil {
		t.Fatal(err)
	}

	_, err := st.CreateIoTJob(account, region, "", `{"op":"x"}`, []string{"job-device"})
	if !errors.Is(err, store.ErrIoTBadRequest) {
		t.Fatalf("empty jobId err=%v", err)
	}
	_, err = st.CreateIoTJob(account, region, "job-a", `{"op":"x"}`, nil)
	if !errors.Is(err, store.ErrIoTBadRequest) {
		t.Fatalf("empty targets err=%v", err)
	}
	_, err = st.CreateIoTJob(account, region, "job-missing-thing", `{}`, []string{"no-such-thing"})
	if !errors.Is(err, store.ErrIoTNotFound) {
		t.Fatalf("missing thing err=%v", err)
	}

	job, err := st.CreateIoTJob(account, region, "job-a", "", []string{
		"arn:aws:iot:" + region + ":" + account + ":thing/job-device",
		"   ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "IN_PROGRESS" || job.Document != "{}" || job.JobARN == "" {
		t.Fatalf("job=%+v", job)
	}
	_, err = st.CreateIoTJob(account, region, "job-a", `{}`, []string{"job-device"})
	if !errors.Is(err, store.ErrIoTConflict) {
		t.Fatalf("duplicate err=%v", err)
	}

	got, err := st.DescribeIoTJob(account, region, "job-a")
	if err != nil || got.JobID != "job-a" || len(got.Targets) < 1 {
		t.Fatalf("describe=%+v err=%v", got, err)
	}
	_, err = st.DescribeIoTJob(account, region, "missing")
	if !errors.Is(err, store.ErrIoTNotFound) {
		t.Fatalf("describe missing err=%v", err)
	}

	inProg, queued, err := st.ListPendingIoTJobExecutions(account, region, "job-device")
	if err != nil || len(inProg) != 0 || len(queued) != 1 {
		t.Fatalf("pending in=%d queued=%d err=%v", len(inProg), len(queued), err)
	}

	ex, err := st.GetIoTJobExecution(account, region, "job-device", "$next")
	if err != nil || ex.JobID != "job-a" || ex.Status != store.IoTJobExecQueued {
		t.Fatalf("$next=%+v err=%v", ex, err)
	}
	ex, err = st.GetIoTJobExecution(account, region, "job-device", "job-a")
	if err != nil || ex.ThingName != "job-device" {
		t.Fatalf("get=%+v err=%v", ex, err)
	}
	_, err = st.GetIoTJobExecution(account, region, "job-device", "nope")
	if !errors.Is(err, store.ErrIoTNotFound) {
		t.Fatalf("missing exec err=%v", err)
	}

	started, err := st.StartNextIoTJobExecution(account, region, "job-device", map[string]string{"phase": "1"})
	if err != nil || started.Status != store.IoTJobExecInProgress || started.StatusDetails["phase"] != "1" {
		t.Fatalf("start=%+v err=%v", started, err)
	}
	again, err := st.StartNextIoTJobExecution(account, region, "job-device", nil)
	if err != nil || again.Status != store.IoTJobExecInProgress || again.VersionNumber <= started.VersionNumber {
		t.Fatalf("restart in-progress=%+v err=%v", again, err)
	}
	inProg, queued, err = st.ListPendingIoTJobExecutions(account, region, "job-device")
	if err != nil || len(inProg) != 1 || len(queued) != 0 {
		t.Fatalf("after start in=%d queued=%d err=%v", len(inProg), len(queued), err)
	}

	_, err = st.StartNextIoTJobExecution(account, region, "other-thing", nil)
	if !errors.Is(err, store.ErrIoTNotFound) {
		t.Fatalf("no pending err=%v", err)
	}
	_, err = st.GetIoTJobExecution(account, region, "other-thing", "$next")
	if !errors.Is(err, store.ErrIoTNotFound) {
		t.Fatalf("$next empty err=%v", err)
	}
}

func TestIoTRoleAliasCRUDAndCertParse(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000001"
	const region = "us-east-1"
	roleARN := "arn:aws:iam::" + account + ":role/iot-creds"

	_, err := st.CreateIoTRoleAlias(account, region, "", roleARN, 0)
	if !errors.Is(err, store.ErrIoTBadRequest) {
		t.Fatalf("empty alias err=%v", err)
	}
	_, err = st.CreateIoTRoleAlias(account, region, "alias-a", "", 0)
	if !errors.Is(err, store.ErrIoTBadRequest) {
		t.Fatalf("empty role err=%v", err)
	}

	ra, err := st.CreateIoTRoleAlias(account, region, "alias-a", roleARN, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ra.CredentialDurationSeconds != 3600 || !strings.Contains(ra.RoleAliasARN, "rolealias/alias-a") {
		t.Fatalf("ra=%+v", ra)
	}
	_, err = st.CreateIoTRoleAlias(account, region, "alias-a", roleARN, 120)
	if !errors.Is(err, store.ErrIoTConflict) {
		t.Fatalf("dup err=%v", err)
	}

	got, err := st.DescribeIoTRoleAlias(account, region, "alias-a")
	if err != nil || got.RoleARN != roleARN {
		t.Fatalf("describe=%+v err=%v", got, err)
	}
	_, err = st.DescribeIoTRoleAlias(account, region, "missing")
	if !errors.Is(err, store.ErrIoTNotFound) {
		t.Fatalf("describe missing err=%v", err)
	}

	list, err := st.ListIoTRoleAliases(account, region)
	if err != nil || len(list) != 1 || list[0].RoleAlias != "alias-a" {
		t.Fatalf("list=%v err=%v", list, err)
	}

	if err := st.DeleteIoTRoleAlias(account, region, "alias-a"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteIoTRoleAlias(account, region, "alias-a"); !errors.Is(err, store.ErrIoTNotFound) {
		t.Fatalf("delete missing err=%v", err)
	}
	list, err = st.ListIoTRoleAliases(account, region)
	if err != nil || len(list) != 0 {
		t.Fatalf("list after delete=%v err=%v", list, err)
	}

	if _, err := store.ParseIoTCertificatePEM("not-pem"); !errors.Is(err, store.ErrIoTBadRequest) {
		t.Fatalf("bad pem err=%v", err)
	}
	if _, err := store.ParseIoTCertificatePEM("-----BEGIN PRIVATE KEY-----\nQQ==\n-----END PRIVATE KEY-----"); !errors.Is(err, store.ErrIoTBadRequest) {
		t.Fatalf("wrong pem type err=%v", err)
	}
	_, err = st.ResolveIoTDeviceFromClientCert(nil)
	if !errors.Is(err, store.ErrIoTNotFound) {
		t.Fatalf("nil cert err=%v", err)
	}

	if store.IoTJobARN(region, account, "j1") == "" || store.IoTRoleAliasARN(region, account, "a1") == "" {
		t.Fatal("arn helpers empty")
	}
	if store.IoTTopicARN(region, account, "t/1") == "" {
		t.Fatal("topic arn empty")
	}
}

func TestListIoTNamedShadowsEmptyAndCreate(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000001"
	const region = "us-east-1"
	if _, err := st.CreateIoTThing(account, region, "shadow-thing", nil); err != nil {
		t.Fatal(err)
	}
	names, err := st.ListIoTNamedShadows(account, region, "shadow-thing")
	if err != nil || names == nil || len(names) != 0 {
		t.Fatalf("empty names=%v err=%v", names, err)
	}
}
