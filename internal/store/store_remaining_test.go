package store_test

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestRemainingExportedZerosBatch(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	region := "us-east-1"

	if err := st.EnsureRoot(account, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}

	if got := store.NormalizeECSClusterNamePublic("  MyCluster  "); got == "" {
		t.Fatal("expected normalized cluster name")
	}
	if got := store.NeptuneNestedEndpoint("lab-cluster", "gremlin"); !strings.Contains(got, "lab-cluster") {
		t.Fatalf("neptune endpoint=%q", got)
	}
	client := store.NestedOpenSearchHTTPClient(2*time.Second, func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	})
	if client == nil || client.Timeout != 2*time.Second {
		t.Fatalf("opensearch client=%v", client)
	}
	if !store.ParameterValueIncluded(store.ParamTypeString, false) {
		t.Fatal("string param should include value")
	}
	if store.ParameterValueIncluded(store.ParamTypeSecureString, false) {
		t.Fatal("secure string without decrypt should hide value")
	}
	if !store.ParameterValueIncluded(store.ParamTypeSecureString, true) {
		t.Fatal("secure string with decrypt should include value")
	}

	st.SetCURUsageEnumerators()

	if _, err := st.CreateLogGroup(account, region, "/zeros/logs"); err != nil {
		t.Fatal(err)
	}
	stream, err := st.EnsureLogStream(account, region, "/zeros/logs", "s1")
	if err != nil || stream.LogStreamName != "s1" {
		t.Fatalf("ensure stream=%+v err=%v", stream, err)
	}
	stream2, err := st.EnsureLogStream(account, region, "/zeros/logs", "s1")
	if err != nil || stream2.LogStreamName != "s1" {
		t.Fatalf("ensure stream again=%+v err=%v", stream2, err)
	}
	if _, err := st.PutSubscriptionFilter(account, "/zeros/logs", "f1", "", "arn:aws:lambda:"+region+":"+account+":function:fn", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSubscriptionFilter(account, "/zeros/logs", "f1"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSubscriptionFilter(account, "/zeros/logs", "missing"); !errors.Is(err, store.ErrLogGroupNotFound) {
		t.Fatalf("delete missing filter err=%v", err)
	}

	if _, err := st.CreateBucket(account, "mp-zeros"); err != nil {
		t.Fatal(err)
	}
	up, err := st.CreateMultipartUpload(account, "mp-zeros", "part.bin", store.CreateMultipartUploadMeta{})
	if err != nil {
		t.Fatal(err)
	}
	gotUp, err := st.GetMultipartUpload(account, "mp-zeros", "part.bin", up.UploadID)
	if err != nil || gotUp.UploadID != up.UploadID {
		t.Fatalf("get multipart=%+v err=%v", gotUp, err)
	}
	if _, err := st.GetMultipartUpload(account, "mp-zeros", "part.bin", "missing"); !errors.Is(err, store.ErrNoSuchUpload) {
		t.Fatalf("missing upload err=%v", err)
	}

	if err := st.VerifySESEmailIdentity(account, "zeros@example.com"); err != nil {
		t.Fatal(err)
	}
	msgID, err := st.SendSESRawEmail(account, "zeros@example.com", []string{"to@example.com"}, "From: zeros@example.com\r\n\r\nbody")
	if err != nil || msgID == "" {
		t.Fatalf("raw email id=%q err=%v", msgID, err)
	}
	if _, err := st.SendSESRawEmail(account, "", nil, "x"); err == nil {
		t.Fatal("expected empty source")
	}
	if _, err := st.SendSESRawEmail(account, "missing@example.com", nil, "x"); !errors.Is(err, store.ErrSESIdentityNotFound) {
		t.Fatalf("unverified err=%v", err)
	}

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	if _, err := st.CreateRole(account, "ZerosRole", trust); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRoleMaxSessionDuration(account, "ZerosRole", 7200); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRoleMaxSessionDuration(account, "ZerosRole", 1); err == nil {
		t.Fatal("expected invalid max session")
	}
	if err := st.UpdateRoleMaxSessionDuration(account, "MissingRole", 3600); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing role err=%v", err)
	}

	if _, _, err := st.CreateUser(account, "zeros-user"); err != nil {
		t.Fatal(err)
	}
	ak, _, err := st.CreateUserAccessKey(account, "zeros-user")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateAccessKeyInAccount(account, ak, store.AccessKeyStatusInactive); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateAccessKeyInAccount(account, ak, "bogus"); err == nil {
		t.Fatal("expected bad status")
	}
	if err := st.UpdateAccessKeyInAccount(account, "AKIAMISSING", store.AccessKeyStatusActive); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing key err=%v", err)
	}

	if err := st.EnableOrgPolicyType(store.OrgRootID, "SCP"); err != nil {
		t.Fatal(err)
	}
	types, err := st.ListEnabledOrgPolicyTypes(store.OrgRootID)
	if err != nil || len(types) < 1 {
		t.Fatalf("policy types=%v err=%v", types, err)
	}
	if st.IsOrgMemberAccount(account, "999999999999") {
		t.Fatal("expected non-member")
	}
	reqID, memberID, err := st.CreateMemberAccount(account, "member-zeros@example.com", "MemberZeros")
	if err != nil || reqID == "" || memberID == "" {
		t.Fatalf("create member req=%q id=%q err=%v", reqID, memberID, err)
	}
	if !st.IsOrgMemberAccount(account, memberID) {
		t.Fatal("expected member")
	}

	td, err := st.RegisterTaskDefinition(account, region, store.RegisterTaskDefinitionInput{
		Family: "zeros-td",
		ContainerDefs: []map[string]any{
			{"name": "app", "image": "public.ecr.aws/docker/library/alpine:3.20"},
		},
		TaskRoleARN:      "arn:aws:iam::" + account + ":role/ecsTaskRole",
		ExecutionRoleARN: "arn:aws:iam::" + account + ":role/ecsExecutionRole",
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := st.RunTask(account, region, store.RunTaskInput{
		Cluster:        store.DefaultECSClusterName,
		TaskDefinition: td.ARN,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetTaskRuntimeID(account, task.TaskARN, "ctr-zeros"); err != nil {
		t.Fatal(err)
	}
	rid, err := st.TaskRuntimeID(account, task.TaskARN)
	if err != nil || rid != "ctr-zeros" {
		t.Fatalf("runtime id=%q err=%v", rid, err)
	}
	if err := st.SetTaskRuntimeID(account, task.TaskARN, ""); err == nil {
		t.Fatal("expected empty runtime id error")
	}
	svc, err := st.CreateService(account, region, store.CreateServiceInput{
		Cluster:        store.DefaultECSClusterName,
		ServiceName:    "zeros-svc",
		TaskDefinition: td.ARN,
		DesiredCount:   1,
	})
	if err != nil || svc.ServiceName != "zeros-svc" {
		t.Fatalf("service=%+v err=%v", svc, err)
	}
	active, err := st.ListActiveServicesWithAccount()
	if err != nil || len(active) < 1 {
		t.Fatalf("active services=%v err=%v", active, err)
	}
	esms, err := st.ListEnabledEventSourceMappings()
	if err != nil {
		t.Fatal(err)
	}
	_ = esms

	pool, err := st.CreateCognitoUserPool(account, region, "zeros-pool")
	if err != nil {
		t.Fatal(err)
	}
	clientRow, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "zeros-app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "alice", "TempPass1!"); err != nil {
		t.Fatal(err)
	}
	acct, gotPool, err := st.ResolveCognitoClientContext(clientRow.ClientID)
	if err != nil || acct != account || gotPool.PoolID != pool.PoolID {
		t.Fatalf("resolve client acct=%q pool=%+v err=%v", acct, gotPool, err)
	}
	acct2, poolID, _, status, err := st.LookupCognitoUserByClient(clientRow.ClientID, "alice")
	if err != nil || acct2 != account || poolID != pool.PoolID || status == "" {
		t.Fatalf("lookup user acct=%q pool=%q status=%q err=%v", acct2, poolID, status, err)
	}
	if _, _, err := st.ResolveCognitoClientContext("missing-client"); err == nil {
		t.Fatal("expected missing client")
	}

	pem, err := st.LabIoTCACertificatePEM()
	if err != nil || !strings.Contains(pem, "BEGIN CERTIFICATE") {
		t.Fatalf("ca pem err=%v len=%d", err, len(pem))
	}
	thing, err := st.CreateIoTThing(account, region, "zeros-global-thing", nil)
	if err != nil {
		t.Fatal(err)
	}
	global, err := st.DescribeIoTThingByNameGlobal(thing.ThingName)
	if err != nil || global.ThingName != thing.ThingName {
		t.Fatalf("global thing=%+v err=%v", global, err)
	}
	if _, err := st.InferSingleActiveCertificateForThing(account, region, thing.ThingName); err == nil {
		t.Fatal("expected no single active cert")
	}

	allows := st.RoleSessionAllows(account, "arn:aws:iam::"+account+":role/ZerosRole", "s3:PutObject", "arn:aws:s3:::mp-zeros", "sess", region)
	_ = allows

	bs := `{"version":"0.2","batch":{"build-list":[{"identifier":"a"},{"identifier":"b"}]},"phases":{"build":{"commands":["echo hi"]}}}`
	if _, err := st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
		Name:        "zeros-batch",
		ServiceRole: "arn:aws:iam::" + account + ":role/CodeBuildRole",
		SourceType:  "NO_SOURCE",
		Buildspec:   bs,
		Image:       "alpine:3.20",
		Artifacts:   map[string]any{"type": "NO_ARTIFACTS"},
	}); err != nil {
		t.Fatal(err)
	}
	batch, builds, err := st.StartCodeBuildBuildBatch(account, region, store.StartCodeBuildBuildBatchOpts{
		ProjectName: "zeros-batch",
	})
	if err != nil || len(builds) < 1 {
		t.Fatalf("batch=%+v builds=%v err=%v", batch, builds, err)
	}
	gotBatch, err := st.GetCodeBuildBuildBatch(account, batch.ID)
	if err != nil || gotBatch.ID != batch.ID {
		t.Fatalf("get batch=%+v err=%v", gotBatch, err)
	}
	if _, err := st.GetCodeBuildBuildBatch(account, "missing-batch"); err == nil {
		t.Fatal("expected missing batch")
	}
}
