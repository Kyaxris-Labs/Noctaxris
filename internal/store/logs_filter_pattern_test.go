package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMatchLogFilterPatternANDExclude(t *testing.T) {
	ok, err := store.MatchLogFilterPattern(`ERROR -timeout`, `request ERROR code=5`)
	if err != nil || !ok {
		t.Fatalf("want match, ok=%v err=%v", ok, err)
	}
	ok, err = store.MatchLogFilterPattern(`ERROR -timeout`, `ERROR timeout`)
	if err != nil || ok {
		t.Fatalf("exclude failed: ok=%v err=%v", ok, err)
	}
}

func TestMatchLogFilterPatternQuotedPhraseAndWildcard(t *testing.T) {
	ok, err := store.MatchLogFilterPattern(`"INTERNAL SERVER ERROR"`, `[ERROR 500] INTERNAL SERVER ERROR`)
	if err != nil || !ok {
		t.Fatalf("quoted phrase: ok=%v err=%v", ok, err)
	}
	ok, err = store.MatchLogFilterPattern(`"INTERNAL SERVER ERROR"`, `INTERNAL SERVER`)
	if err != nil || ok {
		t.Fatalf("partial phrase must not match: ok=%v err=%v", ok, err)
	}

	ok, err = store.MatchLogFilterPattern(`ERR*`, `ERROR boom`)
	if err != nil || !ok {
		t.Fatalf("star wildcard: ok=%v err=%v", ok, err)
	}
	ok, err = store.MatchLogFilterPattern(`E?ROR`, `ERROR boom`)
	if err != nil || !ok {
		t.Fatalf("question wildcard: ok=%v err=%v", ok, err)
	}

	ok, err = store.MatchLogFilterPattern(`ERROR -"bad news"`, `ERROR all good`)
	if err != nil || !ok {
		t.Fatalf("exclude phrase allow: ok=%v err=%v", ok, err)
	}
	ok, err = store.MatchLogFilterPattern(`ERROR -"bad news"`, `ERROR bad news`)
	if err != nil || ok {
		t.Fatalf("exclude phrase deny: ok=%v err=%v", ok, err)
	}
}

func TestMatchLogFilterPatternEmptyAndRejectUnsupported(t *testing.T) {
	ok, err := store.MatchLogFilterPattern("", "anything")
	if err != nil || !ok {
		t.Fatalf("empty pattern: ok=%v err=%v", ok, err)
	}

	cases := []string{
		`{$.status=1}`,
		`fields @message | stats count()`,
		`[w1=ERROR, w2]`,
		`%AUTHORIZED%`,
		`ERROR && WARN`,
		`"unclosed`,
	}
	for _, pat := range cases {
		_, err := store.MatchLogFilterPattern(pat, "x")
		if !errors.Is(err, store.ErrLogFilterPatternInvalid) {
			t.Fatalf("pattern %q: err=%v want ErrLogFilterPatternInvalid", pat, err)
		}
	}
}

func TestFilterLogEventsFilterPatternSubset(t *testing.T) {
	st := openLogsStore(t)
	account := "000000000001"
	group := "/lab/pattern"

	if _, err := st.CreateLogGroup(account, "us-east-1", group); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogStream(account, "us-east-1", group, "s1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutLogEvents(account, group, "s1", "", []store.LogEvent{
		{Timestamp: 1_000, Message: "request ERROR code=5", EventID: "e1"},
		{Timestamp: 2_000, Message: "ERROR timeout", EventID: "e2"},
		{Timestamp: 3_000, Message: "INFO ok", EventID: "e3"},
	}); err != nil {
		t.Fatal(err)
	}

	events, _, err := st.FilterLogEvents(account, store.FilterLogEventsInput{
		LogGroupName:  group,
		FilterPattern: `ERROR -timeout`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != "e1" {
		t.Fatalf("events=%+v", events)
	}

	_, _, err = st.FilterLogEvents(account, store.FilterLogEventsInput{
		LogGroupName:  group,
		FilterPattern: `{$.a=1}`,
	})
	if !errors.Is(err, store.ErrLogFilterPatternInvalid) {
		t.Fatalf("err=%v want ErrLogFilterPatternInvalid", err)
	}
}

func TestPutSubscriptionFilterRejectsUnsupportedPattern(t *testing.T) {
	st := openLogsStore(t)
	account := "000000000001"
	group := "/lab/sub-pat"
	if _, err := st.CreateLogGroup(account, "us-east-1", group); err != nil {
		t.Fatal(err)
	}
	_, err := st.PutSubscriptionFilter(account, group, "f1", `{$.a=1}`, "arn:aws:sqs:us-east-1:000000000001:q", "")
	if !errors.Is(err, store.ErrLogFilterPatternInvalid) {
		t.Fatalf("err=%v want ErrLogFilterPatternInvalid", err)
	}
}
