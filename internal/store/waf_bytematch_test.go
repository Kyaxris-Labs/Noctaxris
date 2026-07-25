package store_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestWAFByteMatchBlocksURI(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	acl, err := st.CreateWAFWebACL(account, "us-east-1", "bytematch-acl", "REGIONAL", "", "Allow", []store.WAFRule{
		{
			Name:     "block-admin",
			Priority: 1,
			Action:   "Block",
			ByteMatchStatement: &store.WAFByteMatchStatement{
				SearchString:         "/admin",
				PositionalConstraint: "CONTAINS",
				FieldToMatchType:     "UriPath",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	view := &store.WAFRequestView{URI: "/admin/x"}
	action, err := st.EvaluateWAFRequestWithView(account, acl.ARN, "", view)
	if err != nil || action != "Block" {
		t.Fatalf("admin path: err=%v action=%q want Block", err, action)
	}
	view.URI = "/ok"
	action, err = st.EvaluateWAFRequestWithView(account, acl.ARN, "", view)
	if err != nil || action != "Allow" {
		t.Fatalf("ok path: err=%v action=%q want Allow", err, action)
	}
}

func TestWAFByteMatchExactURI(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	acl, err := st.CreateWAFWebACL(account, "us-east-1", "exact-acl", "REGIONAL", "", "Allow", []store.WAFRule{
		{
			Name:     "exact-health",
			Priority: 1,
			Action:   "Allow",
			ByteMatchStatement: &store.WAFByteMatchStatement{
				SearchString:         "/health",
				PositionalConstraint: "EXACTLY",
				FieldToMatchType:     "UriPath",
			},
		},
		{
			Name:     "block-rest",
			Priority: 2,
			Action:   "Block",
			ByteMatchStatement: &store.WAFByteMatchStatement{
				SearchString:         "/",
				PositionalConstraint: "CONTAINS",
				FieldToMatchType:     "UriPath",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	action, err := st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{URI: "/health"})
	if err != nil || action != "Allow" {
		t.Fatalf("/health: err=%v action=%q want Allow", err, action)
	}
	action, err = st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{URI: "/healthz"})
	if err != nil || action != "Block" {
		t.Fatalf("/healthz: err=%v action=%q want Block", err, action)
	}
}

func TestWAFByteMatchSingleHeader(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	acl, err := st.CreateWAFWebACL(account, "us-east-1", "hdr-acl", "REGIONAL", "", "Block", []store.WAFRule{
		{
			Name:     "allow-bot",
			Priority: 1,
			Action:   "Allow",
			ByteMatchStatement: &store.WAFByteMatchStatement{
				SearchString:         "FriendlyBot",
				PositionalConstraint: "CONTAINS",
				FieldToMatchType:     "SingleHeader",
				HeaderName:           "User-Agent",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	action, err := st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{
		URI: "/",
		Headers: map[string]string{
			"User-Agent": "FriendlyBot/1.0",
		},
	})
	if err != nil || action != "Allow" {
		t.Fatalf("bot UA: err=%v action=%q want Allow", err, action)
	}
	action, err = st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{URI: "/"})
	if err != nil || action != "Block" {
		t.Fatalf("no UA: err=%v action=%q want Block (default)", err, action)
	}
}
