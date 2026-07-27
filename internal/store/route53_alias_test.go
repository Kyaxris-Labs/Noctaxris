package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestRoute53AliasCloudFrontCreateAndRejectUnknown(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	if _, err := st.CreateBucket(account, "r53-cf-origin"); err != nil {
		t.Fatal(err)
	}
	dist, err := st.CreateCloudFrontDistribution(account, "lab", "r53-cf-ref", true, []store.CloudFrontOrigin{{
		ID: "o1", DomainName: "r53-cf-origin", OriginType: "s3",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if dist.Status != store.CloudFrontStatusDeployed || dist.DomainName == "" {
		t.Fatalf("dist status=%q domain=%q", dist.Status, dist.DomainName)
	}

	zone, err := st.CreateRoute53HostedZone(account, "alias.example.com", "r53-alias-ref", false)
	if err != nil {
		t.Fatal(err)
	}

	err = st.ChangeRoute53ResourceRecordSets(account, zone.ID, []store.Route53Change{{
		Action: "CREATE",
		Name:   "www.alias.example.com",
		Type:   "A",
		AliasTarget: &store.Route53AliasTarget{
			DNSName:              dist.DomainName + ".",
			HostedZoneId:         "Z2FDTNDATAQYW2",
			EvaluateTargetHealth: false,
		},
	}})
	if err != nil {
		t.Fatalf("alias CREATE: %v", err)
	}

	sets, err := st.ListRoute53ResourceRecordSets(account, zone.ID)
	if err != nil || len(sets) != 1 {
		t.Fatalf("list: %v %#v", err, sets)
	}
	if sets[0].AliasTarget == nil {
		t.Fatalf("expected AliasTarget, got %#v", sets[0])
	}
	if len(sets[0].Records) != 0 {
		t.Fatalf("alias must not have ResourceRecords: %#v", sets[0].Records)
	}
	wantDNS := strings.TrimSuffix(strings.ToLower(dist.DomainName), ".")
	if sets[0].AliasTarget.DNSName != wantDNS {
		t.Fatalf("AliasTarget.DNSName=%q want %q", sets[0].AliasTarget.DNSName, wantDNS)
	}

	err = st.ChangeRoute53ResourceRecordSets(account, zone.ID, []store.Route53Change{{
		Action: "CREATE",
		Name:   "evil.alias.example.com",
		Type:   "A",
		AliasTarget: &store.Route53AliasTarget{
			DNSName:      "evil.example",
			HostedZoneId: "Z2FDTNDATAQYW2",
		},
	}})
	if !errors.Is(err, store.ErrRoute53BadRequest) {
		t.Fatalf("unknown alias want ErrRoute53BadRequest, got %v", err)
	}
}

func TestRoute53AliasELBCreate(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	lb, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "r53-alb", "internet-facing", "application")
	if err != nil {
		t.Fatal(err)
	}
	zone, err := st.CreateRoute53HostedZone(account, "elb.example.com", "r53-elb-ref", false)
	if err != nil {
		t.Fatal(err)
	}

	err = st.ChangeRoute53ResourceRecordSets(account, zone.ID, []store.Route53Change{{
		Action: "UPSERT",
		Name:   "app.elb.example.com",
		Type:   "A",
		AliasTarget: &store.Route53AliasTarget{
			DNSName:              strings.ToUpper(lb.DNSName),
			HostedZoneId:         "Z35SXDOTRQ7X7K",
			EvaluateTargetHealth: true,
		},
	}})
	if err != nil {
		t.Fatalf("elb alias UPSERT: %v", err)
	}

	sets, err := st.ListRoute53ResourceRecordSets(account, zone.ID)
	if err != nil || len(sets) != 1 || sets[0].AliasTarget == nil {
		t.Fatalf("list: %v %#v", err, sets)
	}
	if !sets[0].AliasTarget.EvaluateTargetHealth {
		t.Fatal("EvaluateTargetHealth not stored")
	}
	wantDNS := strings.ToLower(lb.DNSName)
	if sets[0].AliasTarget.DNSName != wantDNS {
		t.Fatalf("DNSName=%q want %q", sets[0].AliasTarget.DNSName, wantDNS)
	}
}
