package sts_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

func TestRootCallerIDs(t *testing.T) {
	userID, arn := sts.RootCallerIDs("123456789012")
	if userID != "123456789012" {
		t.Fatalf("userID = %q, want %q", userID, "123456789012")
	}
	wantARN := "arn:aws:iam::123456789012:root"
	if arn != wantARN {
		t.Fatalf("arn = %q, want %q", arn, wantARN)
	}
}

func TestGetCallerIdentityXMLRoundTrip(t *testing.T) {
	accountID := "123456789012"
	userID, arn := sts.RootCallerIDs(accountID)
	requestID := "req-abc-123"

	raw, err := sts.GetCallerIdentityXML(accountID, userID, arn, requestID)
	if err != nil {
		t.Fatalf("GetCallerIdentityXML: %v", err)
	}

	body := string(raw)
	if !strings.Contains(body, `xmlns="https://sts.amazonaws.com/doc/2011-06-15/"`) {
		t.Fatalf("missing STS xmlns: %s", body)
	}
	if !strings.Contains(body, "<GetCallerIdentityResponse") {
		t.Fatalf("missing root element: %s", body)
	}
	if !strings.Contains(body, "<GetCallerIdentityResult>") {
		t.Fatalf("missing result element: %s", body)
	}
	if !strings.Contains(body, "<ResponseMetadata>") {
		t.Fatalf("missing metadata element: %s", body)
	}

	var parsed struct {
		XMLName xml.Name `xml:"GetCallerIdentityResponse"`
		XMLNS   string   `xml:"xmlns,attr"`
		Result  struct {
			Arn     string `xml:"Arn"`
			UserId  string `xml:"UserId"`
			Account string `xml:"Account"`
		} `xml:"GetCallerIdentityResult"`
		Metadata struct {
			RequestId string `xml:"RequestId"`
		} `xml:"ResponseMetadata"`
	}
	if err := xml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("xml.Unmarshal: %v\nbody: %s", err, body)
	}

	if parsed.XMLNS != "https://sts.amazonaws.com/doc/2011-06-15/" {
		t.Fatalf("xmlns = %q, want STS 2011-06-15 namespace", parsed.XMLNS)
	}
	if parsed.Result.Account != accountID {
		t.Fatalf("Account = %q, want %q", parsed.Result.Account, accountID)
	}
	if parsed.Result.UserId != userID {
		t.Fatalf("UserId = %q, want %q", parsed.Result.UserId, userID)
	}
	if parsed.Result.Arn != arn {
		t.Fatalf("Arn = %q, want %q", parsed.Result.Arn, arn)
	}
	if parsed.Metadata.RequestId != requestID {
		t.Fatalf("RequestId = %q, want %q", parsed.Metadata.RequestId, requestID)
	}
}

func TestGetCallerIdentityXMLRootUserIdIsAccount(t *testing.T) {
	accountID := "999888777666"
	userID, arn := sts.RootCallerIDs(accountID)

	raw, err := sts.GetCallerIdentityXML(accountID, userID, arn, "rid-1")
	if err != nil {
		t.Fatalf("GetCallerIdentityXML: %v", err)
	}

	var parsed struct {
		Result struct {
			UserId  string `xml:"UserId"`
			Account string `xml:"Account"`
		} `xml:"GetCallerIdentityResult"`
	}
	if err := xml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("xml.Unmarshal: %v", err)
	}
	if parsed.Result.UserId != accountID {
		t.Fatalf("root UserId = %q, want account ID %q", parsed.Result.UserId, accountID)
	}
	if parsed.Result.Account != accountID {
		t.Fatalf("Account = %q, want %q", parsed.Result.Account, accountID)
	}
}
