package sdk_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func TestSecurityHubBatchImportAndGetFindings(t *testing.T) {
	requireReady(t)
	prefix := uniquePrefix(t)
	region := envOr("AWS_DEFAULT_REGION", "us-east-1")
	findingID := fmt.Sprintf("sdk-sh-%s", prefix)
	generatorID := "noctaxris-sdk-lab"
	now := time.Now().UTC().Format(time.RFC3339)

	stsClient := newSTS(t, loadAWSConfig(t))
	caller, err := stsClient.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
	if err != nil {
		t.Fatalf("GetCallerIdentity: %v", err)
	}
	accountID := ""
	if caller.Account != nil {
		accountID = *caller.Account
	}
	if accountID == "" {
		t.Fatal("GetCallerIdentity missing Account")
	}
	productArn := fmt.Sprintf("arn:aws:securityhub:%s:%s:product/%s/default", region, accountID, accountID)

	importStatus, importBody, importParsed := signedJSONTarget(t, "securityhub", "SecurityHub.BatchImportFindings", map[string]any{
		"Findings": []map[string]any{{
			"SchemaVersion": "2018-10-08",
			"Id":            findingID,
			"ProductArn":    productArn,
			"GeneratorId":   generatorID,
			"AwsAccountId":  accountID,
			"Types":         []string{"Software and Configuration Checks/Vulnerabilities/CVE"},
			"CreatedAt":     now,
			"UpdatedAt":     now,
			"Severity":      map[string]any{"Label": "HIGH"},
			"Title":         "sdk lab finding",
			"Description":   "security hub sdk import",
			"Resources": []map[string]any{{
				"Type": "AwsS3Bucket",
				"Id":   "arn:aws:s3:::sdk-sh-lab",
			}},
		}},
	})
	if importStatus != 200 {
		t.Fatalf("BatchImportFindings status=%d body=%s", importStatus, importBody)
	}
	successCount, _ := importParsed["SuccessCount"].(float64)
	failedCount, _ := importParsed["FailedCount"].(float64)
	if successCount != 1 || failedCount != 0 {
		t.Fatalf("BatchImportFindings parsed=%v", importParsed)
	}

	getStatus, getBody, getParsed := signedJSONTarget(t, "securityhub", "SecurityHub.GetFindings", map[string]any{
		"GeneratorId":   generatorID,
		"SeverityLabel": "HIGH",
		"ResourceType":  "AwsS3Bucket",
	})
	if getStatus != 200 {
		t.Fatalf("GetFindings status=%d body=%s", getStatus, getBody)
	}
	findings, _ := getParsed["Findings"].([]any)
	found := false
	for _, item := range findings {
		f, _ := item.(map[string]any)
		if f == nil {
			continue
		}
		id, _ := f["Id"].(string)
		title, _ := f["Title"].(string)
		if id == findingID || title == "sdk lab finding" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("GetFindings missing imported finding: %s body=%s", findingID, getBody)
	}
}
