package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCreateOpenSearchDomainRejectsDottedDomainName(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	_, err := st.CreateOpenSearchDomain(account, "us-east-1", "ssrf.attacker.com", "")
	if !errors.Is(err, store.ErrOpenSearchBadRequest) {
		t.Fatalf("err=%v want ErrOpenSearchBadRequest", err)
	}
	if !strings.Contains(err.Error(), "DomainName") {
		t.Fatalf("err=%v want DomainName hint", err)
	}
}

func TestCreateOpenSearchDomainCharsetValidation(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	for _, name := range []string{
		"ab",                // too short (AWS min 3)
		"UPPER",             // uppercase
		"bad_name",          // underscore
		"has.dot",           // dotted
		"ssrf.attacker.com", // DNS SSRF shape
		"-leading",          // must start with letter
		strings.Repeat("a", 29), // too long (AWS max 28)
	} {
		_, err := st.CreateOpenSearchDomain(account, "us-east-1", name, "")
		if !errors.Is(err, store.ErrOpenSearchBadRequest) {
			t.Fatalf("name=%q err=%v want ErrOpenSearchBadRequest", name, err)
		}
	}
	if _, err := st.CreateOpenSearchDomain(account, "us-east-1", "lab1", ""); err != nil {
		t.Fatalf("valid DomainName lab1: %v", err)
	}
	if _, err := st.CreateOpenSearchDomain(account, "us-east-1", "lab-domain", ""); err != nil {
		t.Fatalf("valid DomainName lab-domain: %v", err)
	}
}

func TestValidateNestedOpenSearchHostRejectsDottedSuffix(t *testing.T) {
	if err := store.ValidateNestedOpenSearchHost("noctaxris-opensearch-lab1"); err != nil {
		t.Fatal(err)
	}
	if err := store.ValidateNestedOpenSearchHost("noctaxris-data-opensearch-lab1"); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{
		"noctaxris-opensearch-ssrf.attacker.com",
		"noctaxris-data-opensearch-ssrf.attacker.com",
		"noctaxris-opensearch-a.b",
		"noctaxris-opensearch-lab.example",
	} {
		if err := store.ValidateNestedOpenSearchHost(host); err == nil {
			t.Fatalf("expected reject for host %q", host)
		}
	}
}

func TestPinnedNestedOpenSearchDialRejectsDottedHost(t *testing.T) {
	_, err := store.PinnedNestedOpenSearchDialContext(t.Context(), "tcp", "noctaxris-opensearch-ssrf.attacker.com:9200")
	if err == nil {
		t.Fatal("expected dial reject for dotted allowlist host")
	}
	if !strings.Contains(err.Error(), "dotted") && !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("want dotted-host reject, got %v", err)
	}
}
