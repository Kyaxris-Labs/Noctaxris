package store_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openSchedulerStore(t *testing.T) *store.Store {
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

func TestNextScheduleRunRate(t *testing.T) {
	from := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	next, err := store.NextScheduleRun("rate(5 minutes)", from)
	if err != nil {
		t.Fatal(err)
	}
	want := from.Add(5 * time.Minute)
	if !next.Equal(want) {
		t.Fatalf("next=%v want %v", next, want)
	}
}

func TestNextScheduleRunCronHourly(t *testing.T) {
	from := time.Date(2026, 7, 20, 12, 30, 0, 0, time.UTC)
	next, err := store.NextScheduleRun("cron(0 * * * ? *)", from)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 7, 20, 13, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next=%v want %v", next, want)
	}
}

func TestSchedulerCRUD(t *testing.T) {
	st := openSchedulerStore(t)
	account := "000000000001"
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	queueARN := "arn:aws:sqs:us-east-1:" + account + ":sched-q"

	sch, err := st.CreateSchedule(account, "us-east-1", store.CreateScheduleInput{
		Name:       "every-five",
		Expression: "rate(5 minutes)",
		TargetARN:  queueARN,
		Input:      `{"hello":true}`,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	wantARN := "arn:aws:scheduler:us-east-1:" + account + ":schedule/default/every-five"
	if sch.ScheduleARN != wantARN {
		t.Fatalf("arn=%q want %q", sch.ScheduleARN, wantARN)
	}
	if sch.NextRun != now.Add(5*time.Minute).Format(time.RFC3339) {
		t.Fatalf("next_run=%q", sch.NextRun)
	}

	got, err := st.GetSchedule(account, "", "every-five")
	if err != nil {
		t.Fatal(err)
	}
	if got.Expression != "rate(5 minutes)" || got.TargetARN != queueARN {
		t.Fatalf("got=%+v", got)
	}

	list, err := st.ListSchedules(account, "")
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%+v err=%v", list, err)
	}

	expr := "rate(1 hours)"
	updated, err := st.UpdateSchedule(account, "", "every-five", store.UpdateScheduleInput{Expression: &expr}, now)
	if err != nil {
		t.Fatal(err)
	}
	if updated.NextRun != now.Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("updated next_run=%q", updated.NextRun)
	}

	if _, err := st.CreateSchedule(account, "us-east-1", store.CreateScheduleInput{
		Name: "every-five", Expression: "rate(1 minutes)", TargetARN: queueARN,
	}, now); !errors.Is(err, store.ErrScheduleAlreadyExists) {
		t.Fatalf("want ErrScheduleAlreadyExists got %v", err)
	}

	if err := st.DeleteSchedule(account, "", "every-five"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSchedule(account, "", "every-five"); !errors.Is(err, store.ErrNoSuchSchedule) {
		t.Fatalf("want ErrNoSuchSchedule got %v", err)
	}
}

func TestProcessDueSchedulesDeliversToSQS(t *testing.T) {
	st := openSchedulerStore(t)
	account := "000000000001"
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	queue, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "sched-target", nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"scheduler.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + queue.QueueARN + `"}]}`
	if err := st.SetQueueAttributes(account, "sched-target", map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}

	// Create with next_run already due: use at() in the past fails, so create then backdate next_run via update path.
	// Create with rate then force ProcessDueSchedules with a future clock past next_run.
	sch, err := st.CreateSchedule(account, "us-east-1", store.CreateScheduleInput{
		Name:       "due-sqs",
		Expression: "rate(1 minutes)",
		TargetARN:  queue.QueueARN,
		Input:      `{"from":"scheduler"}`,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if sch.NextRun == "" {
		t.Fatal("expected next_run")
	}

	dueAt := now.Add(2 * time.Minute)
	n, err := st.ProcessDueSchedules(dueAt)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("delivered=%d want 1", n)
	}

	msgs, err := st.ReceiveMessages(account, "sched-target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || string(msgs[0].Body) != `{"from":"scheduler"}` {
		t.Fatalf("msgs=%+v", msgs)
	}

	advanced, err := st.GetSchedule(account, "", "due-sqs")
	if err != nil {
		t.Fatal(err)
	}
	wantNext := dueAt.Add(time.Minute).Format(time.RFC3339)
	if advanced.NextRun != wantNext {
		t.Fatalf("next_run=%q want %q", advanced.NextRun, wantNext)
	}
}

func TestProcessDueSchedulesSkipsWithoutPolicy(t *testing.T) {
	st := openSchedulerStore(t)
	account := "000000000001"
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	queue, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "sched-nopolicy", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSchedule(account, "us-east-1", store.CreateScheduleInput{
		Name: "blocked", Expression: "rate(1 minutes)", TargetARN: queue.QueueARN, Input: "x",
	}, now); err != nil {
		t.Fatal(err)
	}
	n, err := st.ProcessDueSchedules(now.Add(2 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("delivered=%d want 0", n)
	}
	msgs, err := st.ReceiveMessages(account, "sched-nopolicy", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("unexpected msgs=%+v", msgs)
	}
}
