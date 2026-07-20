package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openStreamDV6Store(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCloudControlBucketAndRole(t *testing.T) {
	st := openStreamDV6Store(t)
	account := "000000000001"

	_, err := st.CloudControlCreateResource(account, "AWS::EC2::Instance", `{"InstanceType":"t3.micro"}`)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unknown type err=%v", err)
	}

	bucket, err := st.CloudControlCreateResource(account, "AWS::S3::Bucket", `{"BucketName":"cc-lab-bucket"}`)
	if err != nil {
		t.Fatal(err)
	}
	if bucket.Identifier != "cc-lab-bucket" {
		t.Fatalf("identifier=%q", bucket.Identifier)
	}
	got, err := st.CloudControlGetResource(account, "AWS::S3::Bucket", "cc-lab-bucket")
	if err != nil || !strings.Contains(got.Properties, "cc-lab-bucket") {
		t.Fatalf("get bucket=%+v err=%v", got, err)
	}
	list, err := st.CloudControlListResources(account, "AWS::S3::Bucket")
	if err != nil || len(list) == 0 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	role, err := st.CloudControlCreateResource(account, "AWS::IAM::Role", `{"RoleName":"CCLabRole"}`)
	if err != nil {
		t.Fatal(err)
	}
	if role.Identifier != "CCLabRole" {
		t.Fatalf("role id=%q", role.Identifier)
	}
	if err := st.CloudControlDeleteResource(account, "AWS::S3::Bucket", "cc-lab-bucket"); err != nil {
		t.Fatal(err)
	}
}

func TestBCMExportWritesSample(t *testing.T) {
	st := openStreamDV6Store(t)
	account := "000000000001"

	exp, err := st.CreateBCMExport(account, "us-east-1", "lab-cur", "sample", "CSV")
	if err != nil {
		t.Fatal(err)
	}
	if exp.ExportARN == "" || exp.FilePath == "" {
		t.Fatalf("export=%+v", exp)
	}
	raw, err := os.ReadFile(exp.FilePath)
	if err != nil || !strings.Contains(string(raw), "AmazonS3") {
		t.Fatalf("sample file=%q err=%v", string(raw), err)
	}
	list, err := st.ListBCMExports(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.DeleteBCMExport(account, exp.ExportARN); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(exp.FilePath); !os.IsNotExist(err) {
		t.Fatalf("expected sample removed, err=%v path=%s", err, filepath.Clean(exp.FilePath))
	}
}

func TestCostExplorerSeeded(t *testing.T) {
	st := openStreamDV6Store(t)

	rows, err := st.CostExplorerGetCostAndUsage("MONTHLY", []string{"UnblendedCost"})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if rows[0].Total["UnblendedCost"].Amount != "12.34" {
		t.Fatalf("amount=%q", rows[0].Total["UnblendedCost"].Amount)
	}
	total, _, _, err := st.CostExplorerGetCostForecast("UNBLENDED_COST")
	if err != nil || total.Amount != "45.67" {
		t.Fatalf("forecast=%+v err=%v", total, err)
	}
}

func TestBudgetsCRUD(t *testing.T) {
	st := openStreamDV6Store(t)
	account := "000000000001"

	b, err := st.CreateBudget(account, "lab-budget", "COST", "MONTHLY", "100.0", "USD", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.DescribeBudget(account, b.BudgetName)
	if err != nil || got.LimitAmount != "100.0" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	list, err := st.DescribeBudgets(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.DeleteBudget(account, "lab-budget"); err != nil {
		t.Fatal(err)
	}
}

func TestCodeDeployAppGroupDeployment(t *testing.T) {
	st := openStreamDV6Store(t)
	account := "000000000001"

	app, err := st.CreateCodeDeployApplication(account, "LabApp", "ECS")
	if err != nil {
		t.Fatal(err)
	}
	dg, err := st.CreateCodeDeployDeploymentGroup(account, app.ApplicationName, "LabDG", "", "web", "default", "")
	if err != nil {
		t.Fatal(err)
	}
	if dg.ECSServiceName != "web" {
		t.Fatalf("ecs service=%q", dg.ECSServiceName)
	}
	dep, err := st.CreateCodeDeployDeployment(account, app.ApplicationName, dg.DeploymentGroupName, "lab deploy")
	if err != nil {
		t.Fatal(err)
	}
	if dep.Status != "Succeeded" {
		t.Fatalf("status=%q", dep.Status)
	}
	got, err := st.GetCodeDeployDeployment(account, dep.DeploymentID)
	if err != nil || got.DeploymentID != dep.DeploymentID {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	ids, err := st.ListCodeDeployDeployments(account, app.ApplicationName)
	if err != nil || len(ids) != 1 {
		t.Fatalf("list=%v err=%v", ids, err)
	}
}
