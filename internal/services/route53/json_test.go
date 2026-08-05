package route53_test

import (
	"encoding/json"
	"testing"

	route53svc "github.com/Kyaxris-Labs/Noctaxris/internal/services/route53"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestRoute53JSON(t *testing.T) {
	z := store.Route53HostedZone{ID: "Z123", Name: "example.com.", PrivateZone: true, CallerRef: "ref"}
	raw, err := route53svc.CreateHostedZoneJSON(z)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	hz, _ := out["HostedZone"].(map[string]any)
	if hz["Id"] != "/hostedzone/Z123" {
		t.Fatalf("create=%v", out)
	}

	delRaw, _ := route53svc.DeleteHostedZoneJSON("Z123")
	_ = json.Unmarshal(delRaw, &out)

	listRaw, _ := route53svc.ListHostedZonesJSON([]store.Route53HostedZone{z})
	_ = json.Unmarshal(listRaw, &out)
	zones, _ := out["HostedZones"].([]any)
	if len(zones) != 1 {
		t.Fatalf("zones=%v", out)
	}
	_, _ = route53svc.ListHostedZonesJSON(nil)

	chgRaw, _ := route53svc.ChangeResourceRecordSetsJSON("C999")
	_ = json.Unmarshal(chgRaw, &out)

	injRaw, _ := route53svc.InjectQueryLogsJSON(2, "/aws/route53", "stream-1")
	_ = json.Unmarshal(injRaw, &out)
	if out["Delivered"] != float64(2) {
		t.Fatalf("inject=%v", out)
	}

	rr := store.Route53ResourceRecordSet{Name: "a.example.com.", Type: "A", TTL: 60, Records: []string{"1.2.3.4"}}
	rrRaw, _ := route53svc.ListResourceRecordSetsJSON([]store.Route53ResourceRecordSet{rr})
	_ = json.Unmarshal(rrRaw, &out)
	sets, _ := out["ResourceRecordSets"].([]any)
	if len(sets) != 1 {
		t.Fatalf("rr=%v", out)
	}
	_, _ = route53svc.ListResourceRecordSetsJSON(nil)
}
