package store_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestWAFSizeConstraintBlocksLongURI(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	acl, err := st.CreateWAFWebACL(account, "us-east-1", "size-uri-acl", "REGIONAL", "", "Allow", []store.WAFRule{
		{
			Name:     "block-long-path",
			Priority: 1,
			Action:   "Block",
			SizeConstraintStatement: &store.WAFSizeConstraintStatement{
				FieldToMatchType:   "UriPath",
				ComparisonOperator: "GT",
				Size:               10,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	action, err := st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{URI: "/short"})
	if err != nil || action != "Allow" {
		t.Fatalf("short path: err=%v action=%q want Allow", err, action)
	}
	action, err = st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{URI: "/toolongpath"})
	if err != nil || action != "Block" {
		t.Fatalf("long path: err=%v action=%q want Block", err, action)
	}
}

func TestWAFSizeConstraintHeaderOperators(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	acl, err := st.CreateWAFWebACL(account, "us-east-1", "size-hdr-acl", "REGIONAL", "", "Allow", []store.WAFRule{
		{
			Name:     "ua-eq-5",
			Priority: 1,
			Action:   "Block",
			SizeConstraintStatement: &store.WAFSizeConstraintStatement{
				FieldToMatchType:   "SingleHeader",
				HeaderName:         "User-Agent",
				ComparisonOperator: "EQ",
				Size:               5,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	action, err := st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{
		URI:     "/",
		Headers: map[string]string{"User-Agent": "abcde"},
	})
	if err != nil || action != "Block" {
		t.Fatalf("EQ match: err=%v action=%q want Block", err, action)
	}
	action, err = st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{
		URI:     "/",
		Headers: map[string]string{"User-Agent": "abcdef"},
	})
	if err != nil || action != "Allow" {
		t.Fatalf("EQ miss: err=%v action=%q want Allow", err, action)
	}
}

func TestWAFSizeConstraintNEAndLE(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	acl, err := st.CreateWAFWebACL(account, "us-east-1", "size-ne-acl", "REGIONAL", "", "Block", []store.WAFRule{
		{
			Name:     "allow-exact-path-len",
			Priority: 1,
			Action:   "Allow",
			SizeConstraintStatement: &store.WAFSizeConstraintStatement{
				FieldToMatchType:   "UriPath",
				ComparisonOperator: "LE",
				Size:               4,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	action, err := st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{URI: "/ok"})
	if err != nil || action != "Allow" {
		t.Fatalf("LE match: err=%v action=%q want Allow", err, action)
	}
	action, err = st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{URI: "/nope"})
	if err != nil || action != "Block" {
		t.Fatalf("LE miss: err=%v action=%q want Block (default)", err, action)
	}

	acl2, err := st.CreateWAFWebACL(account, "us-east-1", "size-ne2-acl", "REGIONAL", "", "Allow", []store.WAFRule{
		{
			Name:     "block-not-slash",
			Priority: 1,
			Action:   "Block",
			SizeConstraintStatement: &store.WAFSizeConstraintStatement{
				FieldToMatchType:   "UriPath",
				ComparisonOperator: "NE",
				Size:               1,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	action, err = st.EvaluateWAFRequestWithView(account, acl2.ARN, "", &store.WAFRequestView{URI: "/"})
	if err != nil || action != "Allow" {
		t.Fatalf("NE miss on /: err=%v action=%q want Allow", err, action)
	}
	action, err = st.EvaluateWAFRequestWithView(account, acl2.ARN, "", &store.WAFRequestView{URI: "/x"})
	if err != nil || action != "Block" {
		t.Fatalf("NE match: err=%v action=%q want Block", err, action)
	}
}

func TestWAFIPSetBlocksSourceIP(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	acl, err := st.CreateWAFWebACL(account, "us-east-1", "ipset-acl", "REGIONAL", "", "Allow", []store.WAFRule{
		{
			Name:     "block-lab-net",
			Priority: 1,
			Action:   "Block",
			IPSetReferenceStatement: &store.WAFIPSetReferenceStatement{
				Addresses: []string{"192.0.2.0/24", "198.51.100.10/32"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	action, err := st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{
		URI:      "/",
		SourceIP: "192.0.2.44",
	})
	if err != nil || action != "Block" {
		t.Fatalf("CIDR match: err=%v action=%q want Block", err, action)
	}
	action, err = st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{
		URI:      "/",
		SourceIP: "198.51.100.10",
	})
	if err != nil || action != "Block" {
		t.Fatalf("/32 match: err=%v action=%q want Block", err, action)
	}
	action, err = st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{
		URI:      "/",
		SourceIP: "203.0.113.1",
	})
	if err != nil || action != "Allow" {
		t.Fatalf("outside: err=%v action=%q want Allow", err, action)
	}
	action, err = st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{URI: "/"})
	if err != nil || action != "Allow" {
		t.Fatalf("empty SourceIP: err=%v action=%q want Allow", err, action)
	}
}
