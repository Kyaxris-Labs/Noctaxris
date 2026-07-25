package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRoute53AliasCloudFrontAndRejectUnknown(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateBucket(testAccountID, "r53-srv-cf-origin"); err != nil {
		t.Fatal(err)
	}
	cf := mustJSONTarget(t, handler, "CloudFront_2016_01_28.CreateDistribution", "cloudfront", map[string]any{
		"DistributionConfig": map[string]any{
			"CallerReference": "r53-srv-cf-ref",
			"Comment":         "lab",
			"Enabled":         true,
			"Origins": map[string]any{
				"Items": []map[string]any{
					{"Id": "o1", "DomainName": "r53-srv-cf-origin", "OriginType": "s3"},
				},
			},
		},
	}, now)
	if cf.Code != http.StatusOK {
		t.Fatalf("CreateDistribution status=%d body=%q", cf.Code, cf.Body.String())
	}
	var cfResp map[string]any
	_ = json.Unmarshal(cf.Body.Bytes(), &cfResp)
	dist, _ := cfResp["Distribution"].(map[string]any)
	domain, _ := dist["DomainName"].(string)
	if domain == "" {
		t.Fatalf("missing DomainName: %s", cf.Body.String())
	}

	create := mustJSONTarget(t, handler, "AWSRoute53.CreateHostedZone", "route53", map[string]any{
		"Name":            "srv-alias.example.com",
		"CallerReference": "srv-alias-ref",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateHostedZone status=%d body=%q", create.Code, create.Body.String())
	}
	var zoneResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &zoneResp)
	hz, _ := zoneResp["HostedZone"].(map[string]any)
	zoneID, _ := hz["Id"].(string)
	zoneID = strings.TrimPrefix(zoneID, "/hostedzone/")

	ok := mustJSONTarget(t, handler, "AWSRoute53.ChangeResourceRecordSets", "route53", map[string]any{
		"HostedZoneId": zoneID,
		"ChangeBatch": map[string]any{
			"Changes": []map[string]any{{
				"Action": "CREATE",
				"ResourceRecordSet": map[string]any{
					"Name": "www.srv-alias.example.com",
					"Type": "A",
					"AliasTarget": map[string]any{
						"DNSName":              domain,
						"HostedZoneId":         "Z2FDTNDATAQYW2",
						"EvaluateTargetHealth": false,
					},
				},
			}},
		},
	}, now)
	if ok.Code != http.StatusOK {
		t.Fatalf("alias CREATE status=%d body=%q", ok.Code, ok.Body.String())
	}

	list := mustJSONTarget(t, handler, "AWSRoute53.ListResourceRecordSets", "route53", map[string]any{
		"HostedZoneId": zoneID,
	}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListResourceRecordSets status=%d body=%q", list.Code, list.Body.String())
	}
	body := list.Body.String()
	if !strings.Contains(body, "AliasTarget") || !strings.Contains(strings.ToLower(body), strings.ToLower(domain)) {
		t.Fatalf("list missing AliasTarget/domain: %q", body)
	}
	if strings.Contains(body, "ResourceRecords") {
		t.Fatalf("alias list must omit ResourceRecords: %q", body)
	}

	evil := mustJSONTarget(t, handler, "AWSRoute53.ChangeResourceRecordSets", "route53", map[string]any{
		"HostedZoneId": zoneID,
		"ChangeBatch": map[string]any{
			"Changes": []map[string]any{{
				"Action": "CREATE",
				"ResourceRecordSet": map[string]any{
					"Name": "evil.srv-alias.example.com",
					"Type": "A",
					"AliasTarget": map[string]any{
						"DNSName":              "evil.example",
						"HostedZoneId":         "Z2FDTNDATAQYW2",
						"EvaluateTargetHealth": false,
					},
				},
			}},
		},
	}, now)
	if evil.Code == http.StatusOK {
		t.Fatalf("unknown alias must fail closed: %q", evil.Body.String())
	}
	if !strings.Contains(evil.Body.String(), "InvalidChangeBatch") && !strings.Contains(evil.Header().Get("x-amzn-ErrorType"), "InvalidChangeBatch") {
		t.Fatalf("want InvalidChangeBatch, got status=%d body=%q type=%q",
			evil.Code, evil.Body.String(), evil.Header().Get("x-amzn-ErrorType"))
	}
}
