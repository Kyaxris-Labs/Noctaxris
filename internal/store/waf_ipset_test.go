package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestWAFIPSetCRUD(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	created, err := st.CreateWAFIPSet(account, "us-east-1", "lab-block", "REGIONAL", "blocked nets", "IPV4", []string{
		"192.0.2.0/24",
		"198.51.100.10",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "lab-block" || created.ID == "" || created.ARN == "" || created.LockToken == "" {
		t.Fatalf("create summary incomplete: %+v", created)
	}
	if created.IPAddressVersion != "IPV4" || created.Scope != "REGIONAL" {
		t.Fatalf("create fields: version=%q scope=%q", created.IPAddressVersion, created.Scope)
	}
	if !strings.Contains(created.ARN, ":regional/ipset/lab-block/") {
		t.Fatalf("ARN shape: %q", created.ARN)
	}
	if len(created.Addresses) != 2 {
		t.Fatalf("addresses: %v", created.Addresses)
	}

	got, err := st.GetWAFIPSet(account, "lab-block", "REGIONAL", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ARN != created.ARN || got.LockToken != created.LockToken {
		t.Fatalf("get mismatch: %+v vs %+v", got, created)
	}

	byARN, err := st.GetWAFIPSetByARN(account, created.ARN)
	if err != nil || byARN.ID != created.ID {
		t.Fatalf("get by arn: err=%v id=%q", err, byARN.ID)
	}

	updated, err := st.UpdateWAFIPSet(account, "lab-block", "REGIONAL", created.ID, created.LockToken, []string{"203.0.113.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.LockToken == created.LockToken {
		t.Fatal("expected new lock token after update")
	}
	if len(updated.Addresses) != 1 || updated.Addresses[0] != "203.0.113.0/24" {
		t.Fatalf("updated addresses: %v", updated.Addresses)
	}

	listed, err := st.ListWAFIPSets(account, "REGIONAL")
	if err != nil || len(listed) != 1 {
		t.Fatalf("list: err=%v n=%d", err, len(listed))
	}

	if err := st.DeleteWAFIPSet(account, "lab-block", "REGIONAL", created.ID, updated.LockToken); err != nil {
		t.Fatal(err)
	}
	_, err = st.GetWAFIPSet(account, "lab-block", "REGIONAL", "")
	if !errors.Is(err, store.ErrWAFNotFound) {
		t.Fatalf("after delete: err=%v want ErrWAFNotFound", err)
	}
}

func TestWAFIPSetCreateRejectsBadVersion(t *testing.T) {
	st := openTestStore(t)
	_, err := st.CreateWAFIPSet("000000000001", "us-east-1", "bad", "REGIONAL", "", "IPV5", nil)
	if !errors.Is(err, store.ErrWAFBadRequest) {
		t.Fatalf("err=%v want ErrWAFBadRequest", err)
	}
}

func TestWAFIPSetARNReferenceBlocksSourceIP(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	ipset, err := st.CreateWAFIPSet(account, "us-east-1", "block-net", "REGIONAL", "", "IPV4", []string{"192.0.2.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	acl, err := st.CreateWAFWebACL(account, "us-east-1", "ipset-arn-acl", "REGIONAL", "", "Allow", []store.WAFRule{
		{
			Name:     "block-from-ipset",
			Priority: 1,
			Action:   "Block",
			IPSetReferenceStatement: &store.WAFIPSetReferenceStatement{
				ARN: ipset.ARN,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	action, err := st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{
		URI: "/", SourceIP: "192.0.2.44",
	})
	if err != nil || action != "Block" {
		t.Fatalf("ARN match: err=%v action=%q want Block", err, action)
	}
	action, err = st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{
		URI: "/", SourceIP: "203.0.113.1",
	})
	if err != nil || action != "Allow" {
		t.Fatalf("ARN miss: err=%v action=%q want Allow", err, action)
	}
}

func TestWAFIPSetUnknownARNFailsClosed(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	phantom := "arn:aws:wafv2:us-east-1:" + account + ":regional/ipset/missing/00000000-0000-0000-0000-000000000000"
	acl, err := st.CreateWAFWebACL(account, "us-east-1", "ipset-phantom-acl", "REGIONAL", "", "Allow", []store.WAFRule{
		{
			Name:                    "ref-missing",
			Priority:                1,
			Action:                  "Block",
			IPSetReferenceStatement: &store.WAFIPSetReferenceStatement{ARN: phantom},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.EvaluateWAFRequestWithView(account, acl.ARN, "", &store.WAFRequestView{
		URI: "/", SourceIP: "192.0.2.1",
	})
	if err == nil {
		t.Fatal("expected error for unknown IPSet ARN")
	}
	if !strings.Contains(err.Error(), "unknown IPSet ARN") {
		t.Fatalf("err=%v", err)
	}
}
