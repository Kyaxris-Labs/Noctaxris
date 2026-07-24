package store

import "testing"

func TestCronFieldMatchesListRangeStep(t *testing.T) {
	cases := []struct {
		field string
		value int
		want  bool
	}{
		{"1,15", 15, true},
		{"1,15", 2, false},
		{"1-3", 2, true},
		{"1-3", 4, false},
		{"*/6", 0, true},
		{"*/6", 6, true},
		{"*/6", 1, false},
		{"/8", 0, true},
		{"/8", 8, true},
		{"/8", 1, false},
		{"2/10", 2, true},
		{"2/10", 12, true},
		{"2/10", 3, false},
		{"1-10/2", 1, true},
		{"1-10/2", 9, true},
		{"1-10/2", 10, false},
		{"1-10/2", 2, false},
	}
	for _, tc := range cases {
		if got := cronFieldMatches(tc.field, tc.value, 0, 23, false); got != tc.want {
			t.Fatalf("field=%q value=%d got=%v want=%v", tc.field, tc.value, got, tc.want)
		}
	}
}

func TestValidateCronFieldListRangeStep(t *testing.T) {
	if err := validateCronField("1,13", 0, 23, false); err != nil {
		t.Fatalf("list: %v", err)
	}
	if err := validateCronField("1-5", 0, 23, false); err != nil {
		t.Fatalf("range: %v", err)
	}
	if err := validateCronField("*/6", 0, 23, false); err != nil {
		t.Fatalf("star step: %v", err)
	}
	if err := validateCronField("2/10", 0, 23, false); err != nil {
		t.Fatalf("start/step: %v", err)
	}
	if err := validateCronField("1-10/2", 0, 23, false); err != nil {
		t.Fatalf("range/step: %v", err)
	}
	if err := validateCronField("L", 1, 31, true); err != nil {
		t.Fatalf("DOM L: %v", err)
	}
	if err := validateCronField("1#2", 1, 7, true); err != nil {
		t.Fatalf("DOW #: %v", err)
	}
	if err := validateCronField("24", 0, 23, false); err == nil {
		t.Fatal("want reject out of range")
	}
}
