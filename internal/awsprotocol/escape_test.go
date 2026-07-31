package awsprotocol

import "testing"

func TestEscapeXML_basic(t *testing.T) {
	in := `a&b<c>d"e'f`
	got := EscapeXML(in)
	if got == in {
		t.Fatal("expected escaping")
	}
	if want := "&amp;"; !containsAll(got, want, "&lt;", "&gt;") {
		t.Fatalf("got %q", got)
	}
}

func TestEscapeXML_plain(t *testing.T) {
	if got := EscapeXML("hello"); got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) == 0 {
			continue
		}
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
