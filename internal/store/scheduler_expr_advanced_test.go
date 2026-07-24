package store

import (
	"testing"
	"time"
)

func TestCronDayOfWeekNamesAndRanges(t *testing.T) {
	// Monday 2026-07-20 08:00 UTC
	mon := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	if !cronDayMatches("?", "MON", mon) {
		t.Fatal("want MON match Monday")
	}
	if cronDayMatches("?", "TUE", mon) {
		t.Fatal("want TUE miss Monday")
	}
	if !cronDayMatches("?", "MON-FRI", mon) {
		t.Fatal("want MON-FRI match Monday")
	}
	sat := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	if cronDayMatches("?", "MON-FRI", sat) {
		t.Fatal("want MON-FRI miss Saturday")
	}
	if !cronDayMatches("?", "SAT,SUN", sat) {
		t.Fatal("want SAT,SUN match Saturday")
	}
}

func TestCronMonthNames(t *testing.T) {
	if err := validateCronField("JUL", 1, 12, false); err != nil {
		t.Fatalf("JUL: %v", err)
	}
	jul := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	if !cronFieldMatches("JUL", int(jul.Month()), 1, 12, false) {
		t.Fatal("want JUL match July")
	}
	if cronFieldMatches("JAN", int(jul.Month()), 1, 12, false) {
		t.Fatal("want JAN miss July")
	}
}

func TestCronLastDayOfMonth(t *testing.T) {
	if err := validateCronField("L", 1, 31, true); err != nil {
		t.Fatalf("DOM L: %v", err)
	}
	last := time.Date(2026, 7, 31, 17, 0, 0, 0, time.UTC)
	if !cronDayMatches("L", "?", last) {
		t.Fatal("want L match July 31")
	}
	mid := time.Date(2026, 7, 15, 17, 0, 0, 0, time.UTC)
	if cronDayMatches("L", "?", mid) {
		t.Fatal("want L miss July 15")
	}
	feb := time.Date(2026, 2, 28, 17, 0, 0, 0, time.UTC)
	if !cronDayMatches("L", "?", feb) {
		t.Fatal("want L match Feb 28 2026")
	}
}

func TestCronNthWeekdayHash(t *testing.T) {
	if err := validateCronField("SUN#1", 1, 7, true); err != nil {
		t.Fatalf("SUN#1: %v", err)
	}
	if err := validateCronField("1#2", 1, 7, true); err != nil {
		t.Fatalf("1#2: %v", err)
	}
	// First Sunday of July 2026 is July 5.
	firstSun := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	if !cronDayMatches("?", "SUN#1", firstSun) {
		t.Fatal("want SUN#1 match first Sunday")
	}
	secondSun := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	if cronDayMatches("?", "SUN#1", secondSun) {
		t.Fatal("want SUN#1 miss second Sunday")
	}
	if !cronDayMatches("?", "1#2", secondSun) {
		t.Fatal("want 1#2 match second Sunday")
	}
}

func TestCronLastWeekdayOfMonth(t *testing.T) {
	if err := validateCronField("6L", 1, 7, true); err != nil {
		t.Fatalf("6L: %v", err)
	}
	if err := validateCronField("FRIL", 1, 7, true); err != nil {
		t.Fatalf("FRIL: %v", err)
	}
	// Last Friday of July 2026 is July 31.
	lastFri := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	if !cronDayMatches("?", "6L", lastFri) {
		t.Fatal("want 6L match last Friday")
	}
	if !cronDayMatches("?", "FRIL", lastFri) {
		t.Fatal("want FRIL match last Friday")
	}
	earlierFri := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	if cronDayMatches("?", "FRIL", earlierFri) {
		t.Fatal("want FRIL miss earlier Friday")
	}
}

func TestNextScheduleRunCronAdvanced(t *testing.T) {
	from := time.Date(2026, 7, 20, 7, 0, 0, 0, time.UTC) // Monday before 08:00
	next, err := NextScheduleRun("cron(0 8 ? * MON-FRI *)", from)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}

	fromSat := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC) // Saturday after window
	nextMon, err := NextScheduleRun("cron(0 8 ? * MON-FRI *)", fromSat)
	if err != nil {
		t.Fatal(err)
	}
	wantMon := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	if !nextMon.Equal(wantMon) {
		t.Fatalf("got %v want %v", nextMon, wantMon)
	}

	fromMid := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	nextL, err := NextScheduleRun("cron(0 17 L * ? *)", fromMid)
	if err != nil {
		t.Fatal(err)
	}
	wantL := time.Date(2026, 7, 31, 17, 0, 0, 0, time.UTC)
	if !nextL.Equal(wantL) {
		t.Fatalf("L got %v want %v", nextL, wantL)
	}

	fromJune := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	nextHash, err := NextScheduleRun("cron(0 0 ? * SUN#1 *)", fromJune)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	if !nextHash.Equal(wantHash) {
		t.Fatalf("SUN#1 got %v want %v", nextHash, wantHash)
	}
}

func TestValidateCronRejectsHashList(t *testing.T) {
	if err := validateCronField("1#1,2#2", 1, 7, true); err == nil {
		t.Fatal("want reject multiple # expressions")
	}
}
