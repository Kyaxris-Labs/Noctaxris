package store_test

import (
	"encoding/json"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDetectiveCreateGraphIdempotent(t *testing.T) {
	st := openKMSStore(t)
	g1, err := st.CreateDetectiveGraph("000000000001", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if g1.GraphARN == "" || g1.GraphID == "" {
		t.Fatalf("graph=%+v", g1)
	}
	g2, err := st.CreateDetectiveGraph("000000000001", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if g1.GraphARN != g2.GraphARN {
		t.Fatalf("want same arn %q got %q", g1.GraphARN, g2.GraphARN)
	}
	list, err := st.ListDetectiveGraphs("000000000001", "us-east-1", 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
}

func TestDetectiveSearchGraphJoinsCloudTrailAndGuardDuty(t *testing.T) {
	st := openKMSStore(t)
	graph, err := st.CreateDetectiveGraph("000000000001", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	det, err := st.CreateGuardDutyDetector("000000000001")
	if err != nil {
		t.Fatal(err)
	}
	roleArn := "arn:aws:iam::000000000001:role/ForensicRole"
	_, err = st.InjectGuardDutyFindings("000000000001", det.DetectorID, "us-east-1", []store.GuardDutyFinding{
		{
			Type:     "UnauthorizedAccess:IAMUser/InstanceCredentialExfiltration.InsideAWS",
			Severity: 8,
			Resource: map[string]any{
				"resourceType": "AccessKey",
				"AccessKeyDetails": map[string]any{
					"UserName": "alice",
				},
			},
			Service: map[string]any{
				"Action": map[string]any{
					"AwsApiCallAction": map[string]any{
						"RemoteIpDetails": map[string]any{
							"IpAddressV4": "203.0.113.50",
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	dataRoot := st.DataRoot()
	writeCloudTrailFixture(t, dataRoot,
		`{"eventVersion":"1.08","eventTime":"2026-07-20T12:00:00Z","eventSource":"sts.amazonaws.com","eventName":"AssumeRole","eventID":"ev-detective-1","recipientAccountId":"000000000001","sourceIPAddress":"203.0.113.50","userIdentity":{"type":"IAMUser","arn":"`+roleArn+`"}}`,
	)

	result, err := st.SearchDetectiveGraph("000000000001", dataRoot, graph.GraphARN, store.DetectiveSearchFilter{
		ResourceArn: roleArn,
		IpAddress:   "203.0.113.50",
		MaxResults:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CloudTrailEvents) != 1 {
		t.Fatalf("ct events=%d", len(result.CloudTrailEvents))
	}
	if len(result.GuardDutyFindings) != 1 {
		t.Fatalf("gd findings=%d", len(result.GuardDutyFindings))
	}
	var ct map[string]any
	if err := json.Unmarshal(result.CloudTrailEvents[0], &ct); err != nil {
		t.Fatal(err)
	}
	if ct["eventID"] != "ev-detective-1" {
		t.Fatalf("event=%v", ct["eventID"])
	}
}

func TestDetectiveSearchGraphRequiresCriterion(t *testing.T) {
	st := openKMSStore(t)
	graph, err := st.CreateDetectiveGraph("000000000001", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.SearchDetectiveGraph("000000000001", st.DataRoot(), graph.GraphARN, store.DetectiveSearchFilter{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
