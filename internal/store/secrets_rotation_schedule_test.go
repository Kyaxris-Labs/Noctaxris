package store_test

import (
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// fixedInt63Source returns a constant Int63 for deterministic rand.Rand tests.
type fixedInt63Source struct{ v int64 }

func (s fixedInt63Source) Int63() int64 { return s.v }
func (s fixedInt63Source) Seed(int64)   {}

func openSecretsScheduleStore(t *testing.T) *store.Store {
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
	if err := st.EnsureSecretsSchema(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSetRotationRulesDeferralWithoutImmediateRotate(t *testing.T) {
	st := openSecretsScheduleStore(t)
	account := "000000000001"
	created, err := st.CreateSecret(account, "us-east-1", "sched-defer", "v1", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rules := store.SecretRotationRules{AutomaticallyAfterDays: 10}
	if err := st.SetSecretRotationRules(account, created.Name, rules, now, false); err != nil {
		t.Fatal(err)
	}
	sec, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !sec.RotationEnabled {
		t.Fatal("want RotationEnabled")
	}
	if sec.RotationRules.AutomaticallyAfterDays != 10 {
		t.Fatalf("rules=%+v", sec.RotationRules)
	}
	wantNext := now.Add(10 * 24 * time.Hour).Format(time.RFC3339)
	if sec.NextRotationDate != wantNext {
		t.Fatalf("NextRotationDate=%q want %q", sec.NextRotationDate, wantNext)
	}
	cur, err := st.GetSecretValue(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if cur.SecretString != "v1" {
		t.Fatalf("RotateImmediately=false must not rotate: got %q", cur.SecretString)
	}
}

func TestProcessDueSecretRotationsFakeClock(t *testing.T) {
	st := openSecretsScheduleStore(t)
	account := "000000000001"
	created, err := st.CreateSecret(account, "us-east-1", "sched-tick", "initial", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if err := st.SetSecretRotationRules(account, created.Name, store.SecretRotationRules{AutomaticallyAfterDays: 1}, now, false); err != nil {
		t.Fatal(err)
	}

	n, err := st.ProcessDueSecretRotations(now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("not due yet: n=%d", n)
	}

	due := now.Add(25 * time.Hour)
	n, err = st.ProcessDueSecretRotations(due, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("due rotate count=%d", n)
	}
	sec, err := st.GetSecretValue(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if sec.SecretString == "initial" {
		t.Fatal("expected scheduled rotate to change secret string")
	}
	meta, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	wantNext := due.Add(24 * time.Hour).Format(time.RFC3339)
	if meta.NextRotationDate != wantNext {
		t.Fatalf("NextRotationDate=%q want %q", meta.NextRotationDate, wantNext)
	}
	if meta.LastRotatedDate != due.Format(time.RFC3339) {
		t.Fatalf("LastRotatedDate=%q", meta.LastRotatedDate)
	}
}

func TestSetRotationRulesRejectsBothDaysAndSchedule(t *testing.T) {
	st := openSecretsScheduleStore(t)
	account := "000000000001"
	if _, err := st.CreateSecret(account, "us-east-1", "sched-both", "v", nil, "", "", ""); err != nil {
		t.Fatal(err)
	}
	err := st.SetSecretRotationRules(account, "sched-both", store.SecretRotationRules{
		AutomaticallyAfterDays: 7,
		ScheduleExpression:     "rate(7 days)",
	}, time.Now().UTC(), false)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestSetRotationRulesRateHoursFakeClock(t *testing.T) {
	st := openSecretsScheduleStore(t)
	// Duration is stored; pin zero jitter so this case asserts window-start scheduling.
	st.SetRotationJitterRand(rand.New(fixedInt63Source{v: 0}))
	t.Cleanup(func() { st.SetRotationJitterRand(nil) })
	account := "000000000001"
	created, err := st.CreateSecret(account, "us-east-1", "sched-hours", "v1", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rules := store.SecretRotationRules{
		ScheduleExpression: "rate(4 hours)",
		Duration:           "1h",
	}
	if err := st.SetSecretRotationRules(account, created.Name, rules, now, false); err != nil {
		t.Fatal(err)
	}
	sec, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if sec.RotationRules.ScheduleExpression != "rate(4 hours)" || sec.RotationRules.Duration != "1h" {
		t.Fatalf("rules=%+v", sec.RotationRules)
	}
	wantNext := now.Add(4 * time.Hour).Format(time.RFC3339)
	if sec.NextRotationDate != wantNext {
		t.Fatalf("NextRotationDate=%q want %q", sec.NextRotationDate, wantNext)
	}
	cur, err := st.GetSecretValue(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if cur.SecretString != "v1" {
		t.Fatalf("RotateImmediately=false must not rotate: got %q", cur.SecretString)
	}

	n, err := st.ProcessDueSecretRotations(now.Add(3*time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("not due yet: n=%d", n)
	}
	dueAt := now.Add(4 * time.Hour)
	n, err = st.ProcessDueSecretRotations(dueAt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("due count=%d", n)
	}
	meta, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	wantNext2 := dueAt.Add(4 * time.Hour).Format(time.RFC3339)
	if meta.NextRotationDate != wantNext2 {
		t.Fatalf("NextRotationDate=%q want %q", meta.NextRotationDate, wantNext2)
	}
}

func TestSetRotationRulesCronFakeClock(t *testing.T) {
	st := openSecretsScheduleStore(t)
	st.SetRotationJitterRand(rand.New(fixedInt63Source{v: 0}))
	t.Cleanup(func() { st.SetRotationJitterRand(nil) })
	account := "000000000001"
	created, err := st.CreateSecret(account, "us-east-1", "sched-cron", "v1", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 23, 9, 30, 0, 0, time.UTC)
	rules := store.SecretRotationRules{
		ScheduleExpression: "cron(0 10 * * ? *)",
		Duration:           "2h",
	}
	if err := st.SetSecretRotationRules(account, created.Name, rules, now, false); err != nil {
		t.Fatal(err)
	}
	sec, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	wantNext := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if sec.NextRotationDate != wantNext {
		t.Fatalf("NextRotationDate=%q want %q", sec.NextRotationDate, wantNext)
	}
	cur, err := st.GetSecretValue(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if cur.SecretString != "v1" {
		t.Fatal("RotateImmediately=false must not rotate")
	}

	n, err := st.ProcessDueSecretRotations(time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("due count=%d", n)
	}
	meta, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	wantNext2 := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if meta.NextRotationDate != wantNext2 {
		t.Fatalf("NextRotationDate=%q want %q", meta.NextRotationDate, wantNext2)
	}
}

func TestSetRotationRulesRejectsInvalidDurationAndCron(t *testing.T) {
	st := openSecretsScheduleStore(t)
	account := "000000000001"
	if _, err := st.CreateSecret(account, "us-east-1", "sched-bad", "v", nil, "", "", ""); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	cases := []store.SecretRotationRules{
		{ScheduleExpression: "rate(4 hours)", Duration: "5h"},   // window > interval
		{ScheduleExpression: "rate(10 days)", Duration: "25h"},  // > 24h day window
		{ScheduleExpression: "rate(3 hours)"},                   // AWS min 4 hours
		{ScheduleExpression: "cron(15 10 * * ? *)"},             // minutes must be 0
		{ScheduleExpression: "cron(0 10 * * ? 2027)"},           // year must be *
		{Duration: "1h"},                                        // duration alone
	}
	for i, rules := range cases {
		err := st.SetSecretRotationRules(account, "sched-bad", rules, now, false)
		if err == nil {
			t.Fatalf("case %d: expected validation error for %+v", i, rules)
		}
	}
}

func TestSecretsCronWithHourList(t *testing.T) {
	st := openSecretsScheduleStore(t)
	account := "000000000001"
	created, err := st.CreateSecret(account, "us-east-1", "sched-cron-list", "v1", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rules := store.SecretRotationRules{
		ScheduleExpression: "cron(0 1,13 * * ? *)",
	}
	if err := st.SetSecretRotationRules(account, created.Name, rules, now, false); err != nil {
		t.Fatal(err)
	}
	sec, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	wantNext := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if sec.NextRotationDate != wantNext {
		t.Fatalf("NextRotationDate=%q want %q", sec.NextRotationDate, wantNext)
	}
}

func TestSecretsCronWithHourStep(t *testing.T) {
	st := openSecretsScheduleStore(t)
	account := "000000000001"
	created, err := st.CreateSecret(account, "us-east-1", "sched-cron-step", "v1", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rules := store.SecretRotationRules{
		ScheduleExpression: "cron(0 2/10 * * ? *)",
	}
	if err := st.SetSecretRotationRules(account, created.Name, rules, now, false); err != nil {
		t.Fatal(err)
	}
	sec, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	wantNext := time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if sec.NextRotationDate != wantNext {
		t.Fatalf("NextRotationDate=%q want %q", sec.NextRotationDate, wantNext)
	}
}

func TestRotationDurationJitterDeterministic(t *testing.T) {
	st := openSecretsScheduleStore(t)
	// 30m of nanoseconds: Int63n(1h) returns this when Int63 is this value (< 1h).
	st.SetRotationJitterRand(rand.New(fixedInt63Source{v: int64(30 * time.Minute)}))
	t.Cleanup(func() { st.SetRotationJitterRand(nil) })

	account := "000000000001"
	created, err := st.CreateSecret(account, "us-east-1", "sched-jitter", "v1", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rules := store.SecretRotationRules{
		ScheduleExpression: "rate(4 hours)",
		Duration:           "1h",
	}
	if err := st.SetSecretRotationRules(account, created.Name, rules, now, false); err != nil {
		t.Fatal(err)
	}
	windowStart := now.Add(4 * time.Hour)
	wantNext := windowStart.Add(30 * time.Minute)
	sec, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if sec.NextRotationDate != wantNext.Format(time.RFC3339) {
		t.Fatalf("NextRotationDate=%q want %q", sec.NextRotationDate, wantNext.Format(time.RFC3339))
	}

	n, err := st.ProcessDueSecretRotations(windowStart, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("not due at window start: n=%d", n)
	}
	n, err = st.ProcessDueSecretRotations(wantNext, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("due at jittered time: n=%d", n)
	}
	meta, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	// Next window start + same fixed 30m jitter.
	wantNext2 := wantNext.Add(4 * time.Hour).Add(30 * time.Minute).Format(time.RFC3339)
	if meta.NextRotationDate != wantNext2 {
		t.Fatalf("NextRotationDate after rotate=%q want %q", meta.NextRotationDate, wantNext2)
	}
}

func TestRotateImmediatelyTrueAdvancesScheduleAfterRotate(t *testing.T) {
	st := openSecretsScheduleStore(t)
	account := "000000000001"
	if _, err := st.CreateSecret(account, "us-east-1", "sched-now", "before", nil, "", "", ""); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	if err := st.SetSecretRotationRules(account, "sched-now", store.SecretRotationRules{AutomaticallyAfterDays: 5}, now, true); err != nil {
		t.Fatal(err)
	}
	// SetSecretRotationRules with rotateImmediately only configures; caller rotates then marks.
	// Simulate handler: rotate then mark.
	if _, err := st.RotateSecret(account, "sched-now"); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkSecretRotated(account, "sched-now", now); err != nil {
		t.Fatal(err)
	}
	meta, err := st.DescribeSecret(account, "sched-now")
	if err != nil {
		t.Fatal(err)
	}
	wantNext := now.Add(5 * 24 * time.Hour).Format(time.RFC3339)
	if meta.NextRotationDate != wantNext {
		t.Fatalf("NextRotationDate=%q want %q", meta.NextRotationDate, wantNext)
	}
	cur, err := st.GetSecretValue(account, "sched-now")
	if err != nil {
		t.Fatal(err)
	}
	if cur.SecretString == "before" {
		t.Fatal("immediate path should have rotated")
	}
}

func TestSetRotationRulesCronLastDayAndWeekdayNames(t *testing.T) {
	st := openSecretsScheduleStore(t)
	account := "000000000001"
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)

	created, err := st.CreateSecret(account, "us-east-1", "cron-l", "v", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	rules := store.SecretRotationRules{ScheduleExpression: "cron(0 17 L * ? *)"}
	if err := st.SetSecretRotationRules(account, created.Name, rules, now, false); err != nil {
		t.Fatal(err)
	}
	sec, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 7, 31, 17, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if sec.NextRotationDate != want {
		t.Fatalf("NextRotationDate=%q want %q", sec.NextRotationDate, want)
	}

	created2, err := st.CreateSecret(account, "us-east-1", "cron-dow", "v", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	fromFri := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC) // Friday after 08:00
	rules2 := store.SecretRotationRules{ScheduleExpression: "cron(0 8 ? * MON-FRI *)"}
	if err := st.SetSecretRotationRules(account, created2.Name, rules2, fromFri, false); err != nil {
		t.Fatal(err)
	}
	sec2, err := st.DescribeSecret(account, created2.Name)
	if err != nil {
		t.Fatal(err)
	}
	wantMon := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if sec2.NextRotationDate != wantMon {
		t.Fatalf("MON-FRI NextRotationDate=%q want %q", sec2.NextRotationDate, wantMon)
	}
}
