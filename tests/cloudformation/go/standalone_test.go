package cfn_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCloudFormationStandaloneBucket(t *testing.T) {
	requireReady(t)
	cfg := loadCFG(t)
	cfn := newCFN(t, cfg)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	short := suffix
	if len(short) > 12 {
		short = short[:12]
	}
	bucket := "noctaxris-sa-" + short
	if len(bucket) > 63 {
		bucket = bucket[:63]
	}
	stackName := "cfn-sa-" + short

	// Lab CreateStack does not apply the Parameters section, so fill BucketName
	// from the template default prefix before deploy. Credentials come from the
	// environment via loadCFG.
	body := renderTemplate(t, "standalone-bucket.yaml", templateVars{})
	if !strings.Contains(body, "Default: noctaxris-sa") {
		t.Fatal("standalone template missing safe BucketName default prefix")
	}
	body = strings.ReplaceAll(body, "!Ref BucketName", bucket)
	if strings.Contains(body, "!Ref BucketName") {
		t.Fatal("BucketName parameter was not applied")
	}
	createDescribeDeleteStack(t, cfn, stackName, body)
}
