package store_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCFNStackS3BucketRoundTrip(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `{"Resources":{"LabBucket":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-lab-bucket-1"}}}}`
	created, err := st.CreateCFNStack(account, "us-east-1", "lab-stack", tpl, "", "CAPABILITY_NAMED_IAM")
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "CREATE_COMPLETE" {
		t.Fatalf("status=%s", created.Status)
	}
	stacks, err := st.DescribeCFNStacks(account, "lab-stack")
	if err != nil || len(stacks) != 1 {
		t.Fatalf("describe: %v %#v", err, stacks)
	}
	if _, err := st.GetBucketByName("cfn-lab-bucket-1"); err != nil {
		t.Fatalf("bucket missing: %v", err)
	}
	if err := st.DeleteCFNStack(account, "lab-stack"); err != nil {
		t.Fatal(err)
	}
}

func TestCFNRejectsUnknownType(t *testing.T) {
	st := openTestStore(t)
	tpl := `{"Resources":{"X":{"Type":"AWS::EC2::Instance","Properties":{}}}}`
	_, err := st.CreateCFNStack("000000000001", "us-east-1", "bad", tpl, "", "CAPABILITY_NAMED_IAM")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCFNStackS3AndIAMRoleRoundTrip(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `{"Resources":{"LabBucket":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-lab-bucket-2"}},"LabRole":{"Type":"AWS::IAM::Role","Properties":{"RoleName":"CfnLabRole2","AssumeRolePolicyDocument":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}}}}}`
	created, err := st.CreateCFNStack(account, "us-east-1", "lab-stack-2", tpl, "", "CAPABILITY_NAMED_IAM")
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Resources) != 2 {
		t.Fatalf("resources=%d", len(created.Resources))
	}
	if _, err := st.GetBucketByName("cfn-lab-bucket-2"); err != nil {
		t.Fatalf("bucket missing: %v", err)
	}
	if _, _, err := st.GetRole(account, "CfnLabRole2"); err != nil {
		t.Fatalf("role missing: %v", err)
	}
	if err := st.DeleteCFNStack(account, "lab-stack-2"); err != nil {
		t.Fatal(err)
	}
}

func TestCodePipelineRequiresCodeBuild(t *testing.T) {
	st := openTestStore(t)
	_, err := st.CreateCodePipeline("000000000001", "us-east-1", store.CodePipelineDeclaration{
		Name: "p1",
		Stages: []store.CodePipelineStageDecl{{
			Name: "Build",
			Actions: []store.CodePipelineActionDecl{{
				Name: "Src",
				ActionTypeID: store.CodePipelineActionTypeID{
					Category: "Source", Owner: "AWS", Provider: "S3", Version: "1",
				},
				Configuration: map[string]string{},
			}},
		}},
	})
	if err == nil {
		t.Fatal("expected CodeBuild required")
	}
	p, err := st.CreateCodePipeline("000000000001", "us-east-1", store.CodePipelineDeclaration{
		Name: "p1",
		Stages: []store.CodePipelineStageDecl{{
			Name: "Build",
			Actions: []store.CodePipelineActionDecl{{
				Name: "Build",
				ActionTypeID: store.CodePipelineActionTypeID{
					Category: "Build", Owner: "AWS", Provider: "CodeBuild", Version: "1",
				},
				Configuration: map[string]string{"ProjectName": "lab-proj"},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	e, err := st.StartCodePipelineExecution("000000000001", p.Name, nil)
	if err != nil || e.Status != "Succeeded" {
		t.Fatalf("exec: %v %#v", err, e)
	}
}

func TestFirehosePutToS3(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "fh-dest"); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"s3:PutObject","Resource":"*"}]}`
	if err := st.PutBucketPolicy(account, "fh-dest", policy); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateFirehoseStream(account, "us-east-1", "lab-stream", "", "S3", "fh-dest", "out/", "")
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.PutFirehoseRecord(account, "lab-stream", []byte("hello"))
	if err != nil || id == "" {
		t.Fatalf("put: %v %s", err, id)
	}
}

func TestFirehosePutToLambdaEnqueuesInvoke(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "fh-handler",
		RoleARN:      "arn:aws:iam::000000000001:role/lambda-exec",
		Runtime:      store.LambdaRuntimePython312,
		Handler:      "app.handler",
		Timeout:      3,
		Memory:       128,
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFunctionPermission(account, "fh-handler", "fh-allow", "lambda:InvokeFunction", "firehose.amazonaws.com", "", ""); err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateFirehoseStream(account, "us-east-1", "lab-lam", "", "Lambda", "", "", fn.FunctionARN)
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.PutFirehoseRecord(account, "lab-lam", []byte("hello-fh"))
	if err != nil || id == "" {
		t.Fatalf("put: %v %s", err, id)
	}
	job, err := st.LatestAsyncInvocation(account, "fh-handler")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(job.EventJSON, "hello-fh") && !strings.Contains(job.EventJSON, "aGVsbG8tZmg=") {
		t.Fatalf("EventJSON=%q want firehose payload", job.EventJSON)
	}
	if !strings.Contains(job.EventJSON, `"recordId"`) {
		t.Fatalf("EventJSON=%q want recordId", job.EventJSON)
	}
}

func TestFirehosePutDeniedWithoutRoleOrPolicy(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "fh-deny"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFirehoseStream(account, "us-east-1", "deny-stream", "", "S3", "fh-deny", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutFirehoseRecord(account, "deny-stream", []byte("hello")); err == nil {
		t.Fatal("expected Put denied without RoleARN or bucket policy")
	}
}

func TestGlueCatalogRoundTrip(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateGlueDatabase(account, "labdb", "d"); err != nil {
		t.Fatal(err)
	}
	tbl, err := st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName:    "labdb",
		Name:            "t1",
		StorageLocation: "s3://b/p",
		Columns:         []store.GlueColumn{{Name: "id", Type: "string"}},
		PartitionKeys:   []store.GlueColumn{{Name: "dt", Type: "string"}},
		InputFormat:     "org.apache.hadoop.mapred.TextInputFormat",
		OutputFormat:    "org.apache.hadoop.hive.ql.io.HiveIgnoreKeyTextOutputFormat",
		SerDeInfo: store.GlueSerDeInfo{
			SerializationLibrary: "org.apache.hadoop.hive.serde2.lazy.LazySimpleSerDe",
			Parameters:           map[string]string{"field.delim": ","},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetGlueTable(account, "labdb", tbl.Name)
	if err != nil || len(got.Columns) != 1 || len(got.PartitionKeys) != 1 {
		t.Fatalf("get: %v %#v", err, got)
	}
	if got.InputFormat == "" || got.SerDeInfo.SerializationLibrary == "" {
		t.Fatalf("missing storage descriptor fields: %#v", got)
	}
}

func TestWAFEvaluateBlock(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	acl, err := st.CreateWAFWebACL(account, "us-east-1", "acl1", "REGIONAL", "", "Allow", []store.WAFRule{
		{Name: "block-bad", Priority: 1, Action: "Block", Label: "bad"},
	})
	if err != nil {
		t.Fatal(err)
	}
	action, err := st.EvaluateWAFRequest(account, acl.ARN, "bad")
	if err != nil || action != "Block" {
		t.Fatalf("eval: %v %s", err, action)
	}
	action, err = st.EvaluateWAFRequest(account, acl.ARN, "ok")
	if err != nil || action != "Allow" {
		t.Fatalf("default: %v %s", err, action)
	}
}

func TestConfigRecorderAndCompliance(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "config-bucket"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "config-q", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutConfigRecorder(account, "default", "", "ALL"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutConfigDeliveryChannel(account, "default", "config-bucket", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.StartConfigRecorder(account, "default"); err != nil {
		t.Fatal(err)
	}
	rec, err := st.GetConfigRecorder(account, "default")
	if err != nil || !rec.Recording {
		t.Fatalf("recorder: %v %#v", err, rec)
	}
	listed, err := st.ListObjectsV2(account, "config-bucket", "AWSLogs/", "")
	if err != nil || len(listed.Contents) == 0 {
		t.Fatalf("list config history objects: %v %#v", err, listed)
	}
	var foundKey string
	for _, obj := range listed.Contents {
		if strings.Contains(obj.Key, "noctaxris-config-snapshot-default-") {
			foundKey = obj.Key
			break
		}
	}
	if foundKey == "" {
		t.Fatalf("no snapshot key under AWSLogs/: %#v", listed.Contents)
	}
	_, data, err := st.GetObject(account, "config-bucket", foundKey)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	var snap store.ConfigSnapshotLite
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.ConfigurationRecorderName != "default" || snap.AccountID != account {
		t.Fatalf("snapshot meta: %#v", snap)
	}
	if len(snap.S3BucketNames) == 0 || !containsString(snap.S3BucketNames, "config-bucket") {
		t.Fatalf("snapshot buckets: %#v", snap.S3BucketNames)
	}
	if len(snap.SQSQueueNames) == 0 || !containsString(snap.SQSQueueNames, "config-q") {
		t.Fatalf("snapshot queues: %#v", snap.SQSQueueNames)
	}
	results, err := st.DescribeConfigComplianceByRule(account, "lab")
	if err != nil || len(results) != 0 {
		t.Fatalf("compliance without stored rule: %v %#v", err, results)
	}
	if err := st.PutConfigRule(account, "lab", "lab rule"); err != nil {
		t.Fatal(err)
	}
	results, err = st.DescribeConfigComplianceByRule(account, "lab")
	if err != nil || len(results) != 1 || results[0].ComplianceType != "NOT_APPLICABLE" {
		t.Fatalf("compliance for stored rule: %v %#v", err, results)
	}
	if _, err := st.PutConfigDeliveryChannel(account, "missing", "no-such-bucket", "", ""); err == nil {
		t.Fatal("expected missing bucket rejection")
	}
}

func TestConfigStartFailsWhenHistoryPutFails(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "config-bucket"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutConfigRecorder(account, "default", "", "ALL"); err != nil {
		t.Fatal(err)
	}
	longPrefix := strings.Repeat("p/", 480)
	if _, err := st.PutConfigDeliveryChannel(account, "default", "config-bucket", longPrefix, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.StartConfigRecorder(account, "default"); err == nil {
		t.Fatal("expected start failure when snapshot key is invalid")
	}
	rec, err := st.GetConfigRecorder(account, "default")
	if err != nil || rec.Recording {
		t.Fatalf("recording must stay false after failed start: %v %#v", err, rec)
	}
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
