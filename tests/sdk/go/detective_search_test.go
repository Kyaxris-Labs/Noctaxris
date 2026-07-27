package sdk_test

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func TestDetectiveCreateGraphSearchGraph(t *testing.T) {
	requireReady(t)
	ctInject := os.Getenv("NOCTAXRIS_CLOUDTRAIL_INJECT") == "1"
	gdInject := os.Getenv("NOCTAXRIS_GUARDDUTY_INJECT") == "1"
	if !ctInject || !gdInject {
		t.Skip("requires NOCTAXRIS_CLOUDTRAIL_INJECT=1 and NOCTAXRIS_GUARDDUTY_INJECT=1 on the API process for SearchGraph seed")
	}

	cfg := loadAWSConfig(t)
	stsClient := newSTS(t, cfg)
	caller, err := stsClient.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
	if err != nil {
		t.Fatalf("GetCallerIdentity: %v", err)
	}
	if caller.Account == nil || *caller.Account == "" {
		t.Fatal("GetCallerIdentity missing Account")
	}
	accountID := *caller.Account
	prefix := uniquePrefix(t)
	sharedIP := "203.0.113.50"
	roleArn := "arn:aws:iam::" + accountID + ":role/SdkDetective-" + prefix
	eventID := "sdk-det-" + prefix

	createStatus, createBody, createParsed := signedJSONTarget(t, "detective", "AmazonDetective.CreateGraph", map[string]any{})
	if createStatus != 200 {
		t.Fatalf("CreateGraph status=%d body=%s", createStatus, createBody)
	}
	graphArn, _ := createParsed["GraphArn"].(string)
	if graphArn == "" {
		t.Fatalf("CreateGraph missing GraphArn: %s", createBody)
	}

	injCTStatus, injCTBody, _ := signedJSONTarget(t, "cloudtrail", "NoctaxrisCloudTrail.InjectEvents", map[string]any{
		"Events": []map[string]any{{
			"eventTime":       "2026-07-20T12:00:00Z",
			"sourceIPAddress": sharedIP,
			"userIdentity": map[string]any{
				"type": "IAMUser",
				"arn":  roleArn,
			},
			"eventSource": "sts.amazonaws.com",
			"eventName":   "AssumeRole",
			"eventID":     eventID,
			"readOnly":    true,
		}},
	})
	if injCTStatus != 200 {
		t.Fatalf("InjectEvents status=%d body=%s", injCTStatus, injCTBody)
	}

	detStatus, detBody, detParsed := signedJSONTarget(t, "guardduty", "GuardDuty.CreateDetector", map[string]any{})
	if detStatus != 200 {
		t.Fatalf("CreateDetector status=%d body=%s", detStatus, detBody)
	}
	detectorID, _ := detParsed["DetectorId"].(string)
	if detectorID == "" {
		t.Fatalf("CreateDetector missing DetectorId: %s", detBody)
	}

	injGDStatus, injGDBody, _ := signedJSONTarget(t, "guardduty", "NoctaxrisGuardDuty.InjectFindings", map[string]any{
		"DetectorId": detectorID,
		"Findings": []map[string]any{{
			"type":     "UnauthorizedAccess:IAMUser/InstanceCredentialExfiltration.InsideAWS",
			"severity": 8,
			"resource": map[string]any{
				"resourceType": "AccessKey",
				"accessKeyDetails": map[string]any{
					"userName": "sdk-detective",
				},
			},
			"service": map[string]any{
				"action": map[string]any{
					"awsApiCallAction": map[string]any{
						"remoteIpDetails": map[string]any{
							"ipAddressV4": sharedIP,
						},
					},
				},
			},
		}},
	})
	if injGDStatus != 200 {
		t.Fatalf("InjectFindings status=%d body=%s", injGDStatus, injGDBody)
	}

	searchStatus, searchBody, searchParsed := signedJSONTarget(t, "detective", "AmazonDetective.SearchGraph", map[string]any{
		"GraphArn":     graphArn,
		"ResourceArn":  roleArn,
		"IpAddress":    sharedIP,
		"MaxResults":   10,
	})
	if searchStatus != 200 {
		t.Fatalf("SearchGraph status=%d body=%s", searchStatus, searchBody)
	}
	ctEvents, _ := searchParsed["CloudTrailEvents"].([]any)
	if len(ctEvents) < 1 {
		t.Fatalf("SearchGraph CloudTrailEvents empty: %s", searchBody)
	}
	gdFindings, _ := searchParsed["GuardDutyFindings"].([]any)
	if len(gdFindings) < 1 {
		t.Fatalf("SearchGraph GuardDutyFindings empty: %s", searchBody)
	}
}
