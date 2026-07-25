package store

import (
	"testing"
	"time"
)

func TestCronNearestWeekdayW(t *testing.T) {
	// 2026-07-15 is Wednesday → 15W matches 15
	from := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	got, err := NextScheduleRun("cron(0 10 15W * ? *)", from)
	if err != nil {
		t.Fatal(err)
	}
	if got.Day() != 15 || got.Month() != time.July {
		t.Fatalf("got %v", got)
	}
}

func TestCronNearestWeekdayWSaturdayGoesFriday(t *testing.T) {
	// 2026-08-15 is Saturday → 15W → Friday 14
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	got, err := NextScheduleRun("cron(0 9 15W * ? *)", from)
	if err != nil {
		t.Fatal(err)
	}
	if got.Day() != 14 || got.Weekday() != time.Friday {
		t.Fatalf("got %v weekday=%v", got, got.Weekday())
	}
}

func TestCronNearestWeekdayWMonthBoundary(t *testing.T) {
	// 2026-08-01 is Saturday → 1W must stay in August → Monday the 3rd
	from := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	got, err := NextScheduleRun("cron(0 9 1W * ? *)", from)
	if err != nil {
		t.Fatal(err)
	}
	if got.Day() != 3 || got.Month() != time.August || got.Weekday() != time.Monday {
		t.Fatalf("got %v weekday=%v", got, got.Weekday())
	}
}

func TestCronLastWeekdayLW(t *testing.T) {
	// July 2026 last weekday = Fri 31
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	got, err := NextScheduleRun("cron(0 8 LW * ? *)", from)
	if err != nil {
		t.Fatal(err)
	}
	if got.Day() != 31 || got.Weekday() != time.Friday {
		t.Fatalf("got %v", got)
	}
}

func TestValidateCronDOMRejectsOddW(t *testing.T) {
	if err := validateCronDOMField("15W"); err != nil {
		t.Fatalf("15W: %v", err)
	}
	if err := validateCronDOMField("LW"); err != nil {
		t.Fatalf("LW: %v", err)
	}
	if err := validateCronDOMField("1W,15W"); err == nil {
		t.Fatal("want reject W list")
	}
	if err := validateCronDOMField("1-5W"); err == nil {
		t.Fatal("want reject W range")
	}
}
