package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestFilterLogEventsSubstringAndTimeBounds(t *testing.T) {
	st := openLogsStore(t)
	account := "000000000001"
	group := "/lab/filter"

	if _, err := st.CreateLogGroup(account, "us-east-1", group); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogStream(account, "us-east-1", group, "s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogStream(account, "us-east-1", group, "s2"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := st.PutLogEvents(account, group, "s1", "", []store.LogEvent{
		{Timestamp: 1_000, Message: "alpha error one", EventID: "e1"},
		{Timestamp: 2_000, Message: "alpha ok", EventID: "e2"},
		{Timestamp: 5_000, Message: "late error", EventID: "e3"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutLogEvents(account, group, "s2", "", []store.LogEvent{
		{Timestamp: 1_500, Message: "beta error two", EventID: "e4"},
		{Timestamp: 3_000, Message: "beta ok", EventID: "e5"},
	}); err != nil {
		t.Fatal(err)
	}

	events, next, err := st.FilterLogEvents(account, store.FilterLogEventsInput{
		LogGroupName:  group,
		FilterPattern: "error",
		StartTime:     1_000,
		EndTime:       3_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if next != "" {
		t.Fatalf("nextToken=%q want empty", next)
	}
	if len(events) != 2 {
		t.Fatalf("len=%d want 2 events=%+v", len(events), events)
	}
	if events[0].Message != "alpha error one" || events[0].LogStreamName != "s1" {
		t.Fatalf("events[0]=%+v", events[0])
	}
	if events[1].Message != "beta error two" || events[1].LogStreamName != "s2" {
		t.Fatalf("events[1]=%+v", events[1])
	}

	byStream, _, err := st.FilterLogEvents(account, store.FilterLogEventsInput{
		LogGroupName:   group,
		LogStreamNames: []string{"s2"},
		FilterPattern:  "error",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(byStream) != 1 || byStream[0].EventID != "e4" {
		t.Fatalf("byStream=%+v", byStream)
	}

	emptyPattern, _, err := st.FilterLogEvents(account, store.FilterLogEventsInput{
		LogGroupName: group,
		StartTime:    2_000,
		EndTime:      3_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(emptyPattern) != 2 {
		t.Fatalf("emptyPattern len=%d want 2 %+v", len(emptyPattern), emptyPattern)
	}
}

func TestFilterLogEventsPaginationAndMissingGroup(t *testing.T) {
	st := openLogsStore(t)
	account := "000000000001"
	group := "/lab/page"

	if _, err := st.CreateLogGroup(account, "us-east-1", group); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogStream(account, "us-east-1", group, "s1"); err != nil {
		t.Fatal(err)
	}
	evs := []store.LogEvent{
		{Timestamp: 1000, Message: "page", EventID: "p1"},
		{Timestamp: 2000, Message: "page", EventID: "p2"},
		{Timestamp: 3000, Message: "page", EventID: "p3"},
		{Timestamp: 4000, Message: "page", EventID: "p4"},
		{Timestamp: 5000, Message: "page", EventID: "p5"},
	}
	if _, _, err := st.PutLogEvents(account, group, "s1", "", evs); err != nil {
		t.Fatal(err)
	}

	page1, tok, err := st.FilterLogEvents(account, store.FilterLogEventsInput{
		LogGroupName: group,
		Limit:        2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 || tok == "" {
		t.Fatalf("page1 len=%d tok=%q", len(page1), tok)
	}
	page2, tok2, err := st.FilterLogEvents(account, store.FilterLogEventsInput{
		LogGroupName: group,
		Limit:        2,
		NextToken:    tok,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 2 || page2[0].EventID != "p3" {
		t.Fatalf("page2=%+v tok2=%q", page2, tok2)
	}
	page3, tok3, err := st.FilterLogEvents(account, store.FilterLogEventsInput{
		LogGroupName: group,
		Limit:        2,
		NextToken:    tok2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page3) != 1 || page3[0].EventID != "p5" || tok3 != "" {
		t.Fatalf("page3=%+v tok3=%q", page3, tok3)
	}

	if _, _, err := st.FilterLogEvents(account, store.FilterLogEventsInput{
		LogGroupName: "/missing",
	}); !errors.Is(err, store.ErrLogGroupNotFound) {
		t.Fatalf("err=%v want group not found", err)
	}
}
