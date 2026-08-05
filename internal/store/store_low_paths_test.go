package store_test

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestIAMForensicsCredentialReportRichCoverage(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000097"
	if err := st.EnsureRoot(account, "AKIAROOTEXAMPLE97", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(account, "rich-user"); err != nil {
		t.Fatal(err)
	}
	seed := []byte("01234567890123456789012345678901")
	serial, err := st.CreateVirtualMFADevice(account, seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnableMFADevice(account, serial, "rich-user"); err != nil {
		t.Fatal(err)
	}
	ak1, _, err := st.CreateUserAccessKey(account, "rich-user")
	if err != nil {
		t.Fatal(err)
	}
	ak2, _, err := st.CreateUserAccessKey(account, "rich-user")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateAccessKeyInAccount(account, ak2, store.AccessKeyStatusInactive); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := st.RecordAccessKeyLastUsed(ak1, "ec2", "eu-west-1", at); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordAccessKeyLastUsed("ASIASESSIONKEY", "x", "y", at); err != nil {
		t.Fatal(err)
	}
	if err := st.GenerateCredentialReport(account, at); err != nil {
		t.Fatal(err)
	}
	csvBytes, _, _, err := st.GetCredentialReport(account)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(csvBytes))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	if rows[1][7] != "true" {
		t.Fatalf("mfa column=%q", rows[1][7])
	}
	rowJoined := strings.Join(rows[1], ",")
	if !strings.Contains(rowJoined, "ec2") || !strings.Contains(rowJoined, "true") {
		t.Fatalf("keys row=%v", rows[1])
	}
	if _, err := st.GetAccessKeyLastUsed(account, "AKIANOSUCHKEY"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing key err=%v", err)
	}
}

func TestCloudFrontLoggingNegativeAndNoopCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "cf-log-bkt"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "origin"); err != nil {
		t.Fatal(err)
	}
	d, err := st.CreateCloudFrontDistribution(account, "lab", "cf-neg", true, []store.CloudFrontOrigin{{
		ID: "o1", DomainName: "origin", OriginType: "s3",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetCloudFrontDistributionLogging(account, d.ID, store.CloudFrontLoggingConfig{
		Enabled: true,
		Bucket:  "",
	}); err == nil {
		t.Fatal("expected bucket required")
	}
	if err := st.SetCloudFrontDistributionLogging(account, "missing-dist", store.CloudFrontLoggingConfig{
		Enabled: false,
	}); !errors.Is(err, store.ErrCloudFrontNotFound) {
		t.Fatalf("not found err=%v", err)
	}
	if err := st.SetCloudFrontDistributionLogging(account, d.ID, store.CloudFrontLoggingConfig{
		Enabled: true,
		Bucket:  "no-such-bucket",
	}); err == nil {
		t.Fatal("expected missing bucket")
	}
	d.LoggingEnabled = false
	if err := st.AppendCloudFrontAccessLog(account, d, store.CloudFrontAccessLogInput{Status: 200}); err != nil {
		t.Fatal(err)
	}
}

func TestAthenaDuckQueryExecutionBranchesCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	seedAthenaPeopleCSV(t, st, account)

	if _, err := st.StartAthenaDuckQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT 1",
		Database:    "labdb",
	}, nil); err == nil {
		t.Fatal("nil runner")
	}
	_, err := st.StartAthenaDuckQueryExecution(account, store.AthenaStartInput{
		QueryString: "   ",
		Database:    "labdb",
	}, func(_, _ string) ([]store.AthenaColumnInfo, [][]string, error) {
		return nil, nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "QueryString") {
		t.Fatalf("empty query: err=%v", err)
	}
	exec, err := st.StartAthenaDuckQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT name FROM people",
	}, func(_, _ string) ([]store.AthenaColumnInfo, [][]string, error) {
		return nil, nil, nil
	})
	if err != nil || exec.State != "FAILED" {
		t.Fatalf("no db: %+v err=%v", exec, err)
	}
	if _, err := st.CreateGlueDatabase(account, "emptydb", ""); err != nil {
		t.Fatal(err)
	}
	exec, err = st.StartAthenaDuckQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT 1 FROM emptydb.nope",
		Database:    "emptydb",
	}, func(_, _ string) ([]store.AthenaColumnInfo, [][]string, error) {
		return nil, nil, nil
	})
	if err != nil || !strings.Contains(exec.StateChangeReason, "TABLE_NOT_FOUND") {
		t.Fatalf("no tables: %+v err=%v", exec, err)
	}
	if _, err := st.CreateBucket(account, "athena-out"); err != nil {
		t.Fatal(err)
	}
	exec, err = st.StartAthenaDuckQueryExecution(account, store.AthenaStartInput{
		QueryString:    "SELECT name FROM labdb.people",
		Database:       "labdb",
		OutputLocation: "s3://athena-out/results/",
	}, func(querySQL, setupSQL string) ([]store.AthenaColumnInfo, [][]string, error) {
		if !strings.Contains(setupSQL, "people") {
			t.Fatalf("setup=%q", setupSQL)
		}
		return []store.AthenaColumnInfo{{Name: "name", Type: "varchar"}}, [][]string{{"alice"}}, nil
	})
	if err != nil || exec.State != "SUCCEEDED" {
		t.Fatalf("with output: state=%s err=%v reason=%s", exec.State, err, exec.StateChangeReason)
	}
}

func TestSecurityHubValidationAndFiltersCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, _, err := st.BatchImportSecurityHubFindings(account, "", nil); err == nil {
		t.Fatal("empty findings")
	}
	big := make([]store.SecurityHubFinding, 101)
	for i := range big {
		big[i] = store.SecurityHubFinding{
			GeneratorId: "g", Types: []string{"T"}, Title: "t", Description: "d",
		}
	}
	if _, _, err := st.BatchImportSecurityHubFindings(account, "", big); err == nil {
		t.Fatal("cap 100")
	}
	ok, failed, err := st.BatchImportSecurityHubFindings(account, "us-east-1", []store.SecurityHubFinding{
		{Id: "bad-1", Types: []string{"T"}, Title: "t", Description: "d"},
		{GeneratorId: "g", Title: "t", Description: "d"},
		{GeneratorId: "g", Types: []string{"T"}, Description: "d"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ok) != 0 || len(failed) != 3 {
		t.Fatalf("ok=%v failed=%v", ok, failed)
	}
	ok, failed, err = st.BatchImportSecurityHubFindings(account, "", []store.SecurityHubFinding{
		{
			GeneratorId: "filter-gen",
			Types:       []string{"Software and Configuration Checks"},
			Title:       "filtered",
			Description: "desc",
			ProductArn:  "arn:aws:securityhub:us-east-1:000000000001:product/custom/prod",
		},
	})
	if err != nil || len(ok) != 1 || len(failed) != 0 {
		t.Fatalf("import ok=%v failed=%v err=%v", ok, failed, err)
	}
	got, err := st.GetSecurityHubFindings(account, store.SecurityHubFindingsFilter{
		ProductArn:    "arn:aws:securityhub:us-east-1:000000000001:product/custom/prod",
		GeneratorId:   "filter-gen",
		SeverityLabel: "MEDIUM",
		ResourceType:  "Other",
		MaxResults:    5,
	})
	if err != nil || len(got) != 1 || got[0].Title != "filtered" {
		t.Fatalf("findings=%+v err=%v", got, err)
	}
}

func TestLambdaFunctionURLAndACMCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureACMSchema(); err != nil {
		t.Fatal(err)
	}
	cert, err := st.RequestACMCertificate(account, "", "acm.example.com")
	if err != nil {
		t.Fatal(err)
	}
	list, err := st.ListACMCertificates(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if _, err := st.DescribeACMCertificate(account, cert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteACMCertificate(account, cert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteACMCertificate(account, cert.CertificateARN); !errors.Is(err, store.ErrACMNotFound) {
		t.Fatalf("delete twice err=%v", err)
	}
	if err := st.EnsureLambdaFunctionURLSchema(); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteFunctionURLConfig(account, "nope"); !errors.Is(err, store.ErrNoSuchFunctionURL) {
		t.Fatalf("delete missing err=%v", err)
	}
}

func TestMFAListAndDeactivateCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000088"
	if err := st.EnsureRoot(account, "AKIAROOTEXAMPLE88", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(account, "mfa-list"); err != nil {
		t.Fatal(err)
	}
	seed := []byte("01234567890123456789012345678901")
	serial, err := st.CreateVirtualMFADevice(account, seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnableMFADevice(account, serial, "mfa-list"); err != nil {
		t.Fatal(err)
	}
	byUser, err := st.ListMFADevices(account, "mfa-list")
	if err != nil || len(byUser) != 1 || !byUser[0].Enabled {
		t.Fatalf("by user=%+v err=%v", byUser, err)
	}
	all, err := st.ListMFADevices(account, "")
	if err != nil || len(all) != 1 {
		t.Fatalf("all=%+v err=%v", all, err)
	}
	if err := st.DeactivateMFADevice(account, serial, "mfa-list"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeactivateMFADevice(account, serial, "mfa-list"); err == nil {
		t.Fatal("double deactivate")
	}
	if _, err := st.CreateVirtualMFADevice("bad", seed); err == nil {
		t.Fatal("bad account")
	}
}

func TestSESIdentityValidationCoverage(t *testing.T) {
	st := openSESStore(t)
	account := "000000000001"
	if _, err := st.CreateSESIdentityV2(account, ""); err == nil {
		t.Fatal("empty identity")
	}
	if _, err := st.CreateSESIdentityV2(account, "bad@nodot"); err == nil {
		t.Fatal("email without dot")
	}
	if err := st.VerifySESEmailIdentity(account, "sender@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := st.VerifySESEmailIdentity(account, "sender@example.com"); err != nil {
		t.Fatal("idempotent verify")
	}
	domain, err := st.CreateSESIdentityV2(account, "ses-cov.example")
	if err != nil || domain.Verified {
		t.Fatalf("domain=%+v err=%v", domain, err)
	}
	emails, err := st.ListSESIdentities(account, "EmailAddress")
	if err != nil || len(emails) < 1 {
		t.Fatalf("list emails=%v err=%v", emails, err)
	}
	n, err := st.CountSESMessages(account)
	if err != nil || n < 0 {
		t.Fatalf("count=%d err=%v", n, err)
	}
	if err := st.SetSESIdentityNotificationTopic(account, "sender@example.com", "Complaint", "arn:aws:sns:us-east-1:1:t"); err == nil {
		t.Fatal("only bounce supported")
	}
}

func TestLightsailReleaseStaticIPAndPortsCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	region := "us-east-1"
	if err := st.EnsureLightsailSchema(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLightsailInstances(account, region, []string{"port-host"}, "us-east-1a", "ubuntu_22_04", "nano_3_0"); err != nil {
		t.Fatal(err)
	}
	ip, err := st.AllocateLightsailStaticIP(account, region, "rel-ip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AttachLightsailStaticIP(account, region, ip.Name, "port-host"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReleaseLightsailStaticIP(account, region, ip.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetLightsailStaticIP(account, region, ip.Name); !errors.Is(err, store.ErrLightsailNotFound) {
		t.Fatalf("released err=%v", err)
	}
	if _, _, err := st.OpenLightsailInstancePublicPorts(account, region, "port-host", map[string]any{
		"fromPort": 443, "toPort": 443, "protocol": "tcp",
	}); err != nil {
		t.Fatal(err)
	}
	ports, err := st.GetLightsailInstancePortStates(account, region, "port-host")
	if err != nil || len(ports) != 1 || ports[0].FromPort != 443 {
		t.Fatalf("ports=%+v err=%v", ports, err)
	}
	if _, err := st.CloseLightsailInstancePublicPorts(account, region, "port-host", map[string]any{
		"fromPort": 443, "toPort": 443, "protocol": "tcp",
	}); err != nil {
		t.Fatal(err)
	}
	kp, priv, err := st.CreateLightsailKeyPair(account, region, "cov-kp")
	if err != nil || priv == "" || kp.Name != "cov-kp" {
		t.Fatalf("kp=%+v priv empty=%v err=%v", kp, priv == "", err)
	}
	if _, err := st.DeleteLightsailKeyPair(account, region, "cov-kp"); err != nil {
		t.Fatal(err)
	}
}

func TestLambdaFunctionURLCorsCoverage(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return {}"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "url-cors-fn",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + account + ":role/lambda",
		Handler: "app.handler", Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateFunctionURLConfig(store.CreateFunctionURLInput{
		AccountID:        account,
		FunctionName:     fn.FunctionName,
		AuthType:         store.FunctionURLAuthIAM,
		EndpointHost:     "example.com:443",
		CorsAllowOrigins: []string{"https://app.example.com", "https://app.example.com"},
	})
	if err != nil || u.AuthType != store.FunctionURLAuthIAM || len(u.CorsAllowOrigins) != 1 {
		t.Fatalf("url=%+v err=%v", u, err)
	}
	if _, err := st.CreateFunctionURLConfig(store.CreateFunctionURLInput{
		AccountID: account, FunctionName: fn.FunctionName, AuthType: "OAUTH",
	}); err == nil || !errors.Is(err, store.ErrInvalidFunctionURLAuthType) {
		t.Fatalf("bad auth err=%v", err)
	}
	filtered, err := st.ListFunctionURLConfigs(account, fn.FunctionName)
	if err != nil || len(filtered) != 1 {
		t.Fatalf("list filtered=%v err=%v", filtered, err)
	}
}

func TestCloudFrontLoggingEnabledSetCoverage(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "cf-out"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "origin"); err != nil {
		t.Fatal(err)
	}
	d, err := st.CreateCloudFrontDistribution(account, "lab", "cf-set-log", true, []store.CloudFrontOrigin{{
		ID: "o1", DomainName: "origin", OriginType: "s3",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetCloudFrontDistributionLogging(account, d.ID, store.CloudFrontLoggingConfig{
		Enabled: true, Bucket: "s3://cf-out/prefix", Prefix: "logs/",
	}); err != nil {
		t.Fatal(err)
	}
	d.LoggingEnabled = true
	d.LoggingBucket = ""
	if err := st.AppendCloudFrontAccessLog(account, d, store.CloudFrontAccessLogInput{Status: 200}); err == nil {
		t.Fatal("expected missing logging bucket on distribution")
	}
}

func TestEnableMFADeviceErrorPathsCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000087"
	if err := st.EnsureRoot(account, "AKIAROOTEXAMPLE87", "secret"); err != nil {
		t.Fatal(err)
	}
	seed := []byte("01234567890123456789012345678901")
	serial, err := st.CreateVirtualMFADevice(account, seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnableMFADevice(account, serial, "nobody"); err == nil {
		t.Fatal("missing user")
	}
	if err := st.EnableMFADevice(account, "arn:aws:iam::"+account+":mfa/missing", "root"); err == nil {
		t.Fatal("missing device")
	}
}
