package store_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestEnsureSchemaWrappersCoverage(t *testing.T) {
	st := openTestStore(t)
	ensurers := []func() error{
		st.EnsureAPIGatewayRESTSchema,
		st.EnsureAPIGatewayV2Schema,
		st.EnsureAppSyncSchema,
		st.EnsureASGSchema,
		st.EnsureBatchSchema,
		st.EnsureBedrockSchema,
		st.EnsureCFNSchema,
		st.EnsureCloudControlSchema,
		st.EnsureCloudFrontSchema,
		st.EnsureCloudWatchSchema,
		st.EnsureCognitoMFASchema,
		st.EnsureConfigSchema,
		st.EnsureDocDBSchema,
		st.EnsureECRSchema,
		st.EnsureECSSchema,
		st.EnsureEKSSchema,
		st.EnsureElastiCacheSchema,
		st.EnsureELBv2Schema,
		st.EnsureEMRSchema,
		st.EnsureEventsSchema,
		st.EnsureKMSKeyMaterialSchema,
		st.EnsureLightsailSchema,
		st.EnsureLogsResourcePolicySchema,
		st.EnsureLogsSchema,
		st.EnsureLogsSubscriptionSchema,
		st.EnsureMemoryDBSchema,
		st.EnsureMQSchema,
		st.EnsureNeptuneSchema,
		st.EnsureOpenSearchSchema,
		st.EnsureOrgAccountPlacementSchema,
		st.EnsureRDSSchema,
		st.EnsureS3VersioningSchema,
		st.EnsureSchedulerSchema,
		st.EnsureSecretsRecoverySchema,
		st.EnsureSecretsRotationScheduleSchema,
		st.EnsureSecretsRotationSchema,
		st.EnsureSecretsVersionsSchema,
		st.EnsureSFNSchema,
		st.EnsureSFNTaskTokenSchema,
		st.EnsureSNSSchema,
		st.EnsureTaggingSchema,
		st.EnsureTextractSchema,
		st.EnsureTransferSchema,
	}
	for i, fn := range ensurers {
		if err := fn(); err != nil {
			t.Fatalf("ensurer[%d]: %v", i, err)
		}
	}
}

func TestAppSyncAPIListGetDeleteCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	api, err := st.CreateAppSyncGraphqlAPI(account, "us-east-1", "zeros-api", store.AppSyncAuthAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListAppSyncGraphqlAPIs(account)
	if err != nil || len(listed) < 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	gotAcct, got, err := st.GetAppSyncGraphqlAPIByID(api.APIID)
	if err != nil || gotAcct != account || got.APIID != api.APIID {
		t.Fatalf("get by id=%+v acct=%q err=%v", got, gotAcct, err)
	}
	if _, _, err := st.GetAppSyncGraphqlAPIByID("missing-api"); err == nil {
		t.Fatal("expected missing api")
	}
	if err := st.DeleteAppSyncGraphqlAPI(account, api.APIID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAppSyncGraphqlAPI(account, "missing-api"); err == nil {
		t.Fatal("expected delete missing")
	}
}

func TestCloudControlGetListLiveCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureRoot(account, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureCloudControlSchema(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "cc-live-bucket"); err != nil {
		t.Fatal(err)
	}
	got, err := st.CloudControlGetResource(account, "AWS::S3::Bucket", "cc-live-bucket")
	if err != nil || got.Identifier != "cc-live-bucket" {
		t.Fatalf("live get=%+v err=%v", got, err)
	}
	list, err := st.CloudControlListResources(account, "AWS::S3::Bucket")
	if err != nil || len(list) < 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if _, err := st.CloudControlGetResource(account, "AWS::EC2::Instance", "i-x"); err == nil {
		t.Fatal("expected unsupported")
	}
	if _, err := st.CloudControlGetResource(account, "AWS::S3::Bucket", "missing-bucket"); !errors.Is(err, store.ErrCloudControlNotFound) {
		t.Fatalf("missing err=%v", err)
	}
	res, _, err := st.CloudControlCreateResource(account, "AWS::S3::Bucket", `{"BucketName":"cc-stored-bucket"}`)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := st.CloudControlGetResource(account, "AWS::S3::Bucket", res.Identifier)
	if err != nil || stored.Identifier != res.Identifier {
		t.Fatalf("stored get=%+v err=%v", stored, err)
	}
}

func TestCloudTrailDescribeDeleteAndARNHelpers(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "ct-zeros"); err != nil {
		t.Fatal(err)
	}
	arn := store.CloudTrailTrailARN("us-east-1", account, "zeros-trail")
	if !strings.Contains(arn, "trail/zeros-trail") {
		t.Fatalf("arn=%q", arn)
	}
	orgARN := store.CloudTrailTrailARNFor("us-east-1", account, "org-trail", true)
	if !strings.Contains(orgARN, "organization") && !strings.Contains(orgARN, "org-trail") {
		t.Fatalf("org arn=%q", orgARN)
	}
	trail, err := st.CreateCloudTrailTrail(account, store.CloudTrailTrail{
		Name: "zeros-trail", S3BucketName: "ct-zeros",
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := st.DescribeCloudTrailTrails(account, nil)
	if err != nil || len(listed) < 1 {
		t.Fatalf("describe all=%v err=%v", listed, err)
	}
	byName, err := st.DescribeCloudTrailTrails(account, []string{trail.Name, "missing"})
	if err != nil || len(byName) != 1 {
		t.Fatalf("describe names=%v err=%v", byName, err)
	}
	if err := st.DeleteCloudTrailTrail(account, trail.Name); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCloudTrailTrail(account, "missing"); err == nil {
		t.Fatal("expected missing trail delete error")
	}
}

func TestELBv2DeleteGetMatchCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	lb, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "zeros-alb", "internet-facing", "application")
	if err != nil {
		t.Fatal(err)
	}
	gotLB, err := st.GetELBv2LoadBalancerByName(account, "zeros-alb")
	if err != nil || gotLB.ARN != lb.ARN {
		t.Fatalf("get by name=%+v err=%v", gotLB, err)
	}
	if _, err := st.GetELBv2LoadBalancerByName(account, "missing-alb"); err == nil {
		t.Fatal("expected missing lb")
	}
	attrs, err := st.DescribeELBv2LoadBalancerAttributes(account, lb.ARN)
	if err != nil || len(attrs) < 1 {
		t.Fatalf("attrs=%v err=%v", attrs, err)
	}
	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "zeros-tg", "ip", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := st.CreateELBv2Listener(account, "us-east-1", lb.ARN, tg.ARN, "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	gotL, err := st.GetELBv2ListenerByARN(account, listener.ListenerARN)
	if err != nil || gotL.ListenerARN != listener.ListenerARN {
		t.Fatalf("get listener=%+v err=%v", gotL, err)
	}
	rule := store.ELBv2Rule{PathPatterns: []string{"/api/*"}, HostHeaders: []string{"lab.example"}}
	if !store.MatchELBv2RulePathHost(rule, "/api/x", "lab.example") {
		t.Fatal("expected path+host match")
	}
	if store.MatchELBv2RulePathHost(rule, "/other", "lab.example") {
		t.Fatal("expected path miss")
	}
	if err := st.DeleteELBv2Listener(account, listener.ListenerARN); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteELBv2Listener(account, "arn:aws:elasticloadbalancing:us-east-1:000000000001:listener/app/x/y/z"); err == nil {
		t.Fatal("expected missing listener delete")
	}
	if err := st.DeleteELBv2TargetGroup(account, tg.ARN); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteELBv2TargetGroup(account, "arn:aws:elasticloadbalancing:us-east-1:000000000001:targetgroup/missing/0123456789abcdef"); err == nil {
		t.Fatal("expected missing tg delete")
	}
}

func TestASGUpdateDeleteListCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	region := "us-east-1"
	if _, err := st.CreateLaunchConfiguration(account, region, "zeros-lc", "ami-1", "t3.micro", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLaunchConfiguration(account, region, "zeros-lc-2", "ami-2", "t3.small", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	g, err := st.CreateAutoScalingGroup(account, region, "zeros-asg", "zeros-lc", 1, 3, 1, []string{"us-east-1a"}, "", "EC2", 300)
	if err != nil {
		t.Fatal(err)
	}
	min, max, desired := 0, 4, 2
	lc2 := "zeros-lc-2"
	updated, err := st.UpdateAutoScalingGroup(account, region, g.AutoScalingGroupName, &min, &max, &desired, &lc2, []string{"us-east-1a", "us-east-1b"}, nil)
	if err != nil || updated.DesiredCapacity != 2 || updated.LaunchConfigurationName != lc2 {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	if _, err := st.UpdateAutoScalingGroup(account, region, "missing-asg", &min, &max, &desired, nil, nil, nil); !errors.Is(err, store.ErrASGNotFound) {
		t.Fatalf("missing update err=%v", err)
	}
	groups, err := st.ListASGGroupsWithAccount()
	if err != nil || len(groups) < 1 {
		t.Fatalf("list with account=%v err=%v", groups, err)
	}
	if err := st.DeleteLaunchConfiguration(account, region, "zeros-lc"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteLaunchConfiguration(account, region, "missing-lc"); !errors.Is(err, store.ErrASGNotFound) {
		t.Fatalf("delete missing lc err=%v", err)
	}
}

func TestECRDeleteRepositoryAndBatchDeleteImage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateRepository(account, "us-east-1", "zeros-repo"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutImage(account, "zeros-repo", "sha256:"+strings.Repeat("a", 64), []string{"v1"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchDeleteImage(account, "zeros-repo", nil, []string{"v1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchDeleteImage(account, "zeros-repo", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchDeleteImage(account, "missing-repo", []string{"sha256:x"}, nil); err == nil {
		t.Fatal("expected missing repo")
	}
	if err := st.DeleteRepository(account, "zeros-repo"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteRepository(account, "missing-repo"); err == nil {
		t.Fatal("expected missing repo delete")
	}
}

func TestSFNListDeleteCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	def := `{"StartAt":"Ok","States":{"Ok":{"Type":"Succeed"}}}`
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "zeros-sm", def, "")
	if err != nil {
		t.Fatal(err)
	}
	list, err := st.ListSFNStateMachines(account)
	if err != nil || len(list) < 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.DeleteSFNStateMachine(account, sm.StateMachineARN); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSFNStateMachine(account, "arn:aws:states:us-east-1:000000000001:stateMachine:missing"); err == nil {
		t.Fatal("expected missing sm delete")
	}
}

func TestIDPGetDeleteListCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	oidcARN, err := st.PutOIDCProvider(account, "https://oidc.example.com", "client-a")
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetOIDCProvider(oidcARN)
	if err != nil || got.ClientID != "client-a" {
		t.Fatalf("get oidc=%+v err=%v", got, err)
	}
	if err := st.DeleteOIDCProvider(oidcARN); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteOIDCProvider(oidcARN); err == nil {
		t.Fatal("expected second delete fail")
	}
	samlARN, err := st.PutSAMLProvider(account, "ZerosSAML", "<EntityDescriptor/>")
	if err != nil {
		t.Fatal(err)
	}
	list, err := st.ListSAMLProviders(account)
	if err != nil || len(list) != 1 || list[0].ProviderARN != samlARN {
		t.Fatalf("list saml=%v err=%v", list, err)
	}
	if err := st.DeleteSAMLProvider(samlARN); err != nil {
		t.Fatal(err)
	}
}

func TestKinesisPutRecordsAndDecodeHelpers(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateKinesisStream(account, "us-east-1", "zeros-stream", 1); err != nil {
		t.Fatal(err)
	}
	results, failed, err := st.PutKinesisRecords(account, "zeros-stream", []store.PutKinesisRecordsEntry{
		{Data: []byte("one"), PartitionKey: "pk1"},
		{Data: []byte("two"), PartitionKey: "pk2"},
	})
	if err != nil || failed != 0 || len(results) != 2 {
		t.Fatalf("put records results=%v failed=%d err=%v", results, failed, err)
	}
	raw := base64.StdEncoding.EncodeToString([]byte("hello"))
	decoded, err := store.DecodeKinesisData(raw)
	if err != nil || string(decoded) != "hello" {
		t.Fatalf("decode kinesis=%q err=%v", decoded, err)
	}
	if _, err := store.DecodeKinesisData(123); err == nil {
		t.Fatal("expected decode type error")
	}
	fh, err := store.DecodeFirehoseData(raw)
	if err != nil || string(fh) != "hello" {
		t.Fatalf("decode firehose=%q err=%v", fh, err)
	}
	if _, err := store.DecodeFirehoseData(1.5); err == nil {
		t.Fatal("expected firehose decode error")
	}
	if got := store.ParseFirehoseOpenSearchDomainRef("arn:aws:es:us-east-1:1:domain/lab-os", ""); got != "lab-os" {
		t.Fatalf("parse domain=%q", got)
	}
	if got := store.ParseFirehoseOpenSearchDomainRef("", "named"); got != "named" {
		t.Fatalf("parse named=%q", got)
	}
}

func TestCFNListStacksAndS3HeadObjectARN(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `{"Resources":{"B":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-zeros-b"}}}}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "zeros-stack", tpl, "", ""); err != nil {
		t.Fatal(err)
	}
	stacks, err := st.ListCFNStacks(account)
	if err != nil || len(stacks) < 1 {
		t.Fatalf("stacks=%v err=%v", stacks, err)
	}
	if _, err := st.CreateBucket(account, "head-zeros"); err != nil {
		t.Fatal(err)
	}
	if err := st.HeadBucket(account, "head-zeros"); err != nil {
		t.Fatal(err)
	}
	if err := st.HeadBucket(account, "missing-head"); err == nil {
		t.Fatal("expected missing head")
	}
	arn := store.ObjectARN("head-zeros", "a/b")
	if !strings.Contains(arn, "head-zeros/a/b") {
		t.Fatalf("object arn=%q", arn)
	}
}

func TestSNSListSubscriptionsRemovePermission(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	topic, err := st.CreateTopic(account, "us-east-1", "zeros-topic", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.Subscribe(account, topic.TopicARN, "email", "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	subs, err := st.ListSubscriptions(account)
	if err != nil || len(subs) < 1 {
		t.Fatalf("subs=%v err=%v", subs, err)
	}
	principal := `{"AWS":"*"}`
	if err := st.AddTopicPermission(account, "zeros-topic", "zeros-label", principal, "sns:Publish"); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveTopicPermission(account, "zeros-topic", "zeros-label"); err != nil {
		t.Fatal(err)
	}
	// Missing label is idempotent (no error).
	if err := st.RemoveTopicPermission(account, "zeros-topic", "missing-label"); err != nil {
		t.Fatal(err)
	}
	name, err := store.TopicNameFromARN(topic.TopicARN)
	if err != nil || name != "zeros-topic" {
		t.Fatalf("topic name=%q err=%v", name, err)
	}
}

func TestAthenaStopQueryAndIoTListUpdate(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureAthenaSchema(); err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.StopAthenaQueryExecution(account, exec.QueryExecutionID); err != nil {
		t.Fatal(err)
	}
	if err := st.StopAthenaQueryExecution(account, exec.QueryExecutionID); err != nil {
		t.Fatal(err)
	}
	if err := st.StopAthenaQueryExecution(account, "missing-qid"); err == nil {
		t.Fatal("expected missing stop")
	}

	thing, err := st.CreateIoTThing(account, "us-east-1", "zeros-thing", map[string]string{"env": "lab"})
	if err != nil {
		t.Fatal(err)
	}
	things, err := st.ListIoTThings(account, "us-east-1")
	if err != nil || len(things) < 1 {
		t.Fatalf("things=%v err=%v", things, err)
	}
	updated, err := st.UpdateIoTThing(account, "us-east-1", thing.ThingName, map[string]string{"env": "prod"}, nil)
	if err != nil || updated.Attributes["env"] != "prod" {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	certs, err := st.ListIoTCertificates(account, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	_ = certs
	pols, err := st.ListIoTPolicies(account, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	_ = pols
}

func TestLightsailGetRebootAndTransferList(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	region := "us-east-1"
	created, err := st.CreateLightsailInstances(account, region, []string{"zeros-web"}, "us-east-1a", "ubuntu_22_04", "nano_3_0")
	if err != nil || len(created) != 1 {
		t.Fatalf("create=%v err=%v", created, err)
	}
	insts, err := st.GetLightsailInstances(account, region)
	if err != nil || len(insts) < 1 {
		t.Fatalf("instances=%v err=%v", insts, err)
	}
	disks, err := st.GetLightsailDisks(account, region)
	if err != nil {
		t.Fatal(err)
	}
	_ = disks
	keys, err := st.GetLightsailKeyPairs(account, region)
	if err != nil {
		t.Fatal(err)
	}
	_ = keys
	ips, err := st.GetLightsailStaticIPs(account, region)
	if err != nil {
		t.Fatal(err)
	}
	_ = ips
	reb, err := st.RebootLightsailInstance(account, region, "zeros-web")
	if err != nil || reb.Name != "zeros-web" {
		t.Fatalf("reboot=%+v err=%v", reb, err)
	}
	if _, err := st.RebootLightsailInstance(account, region, "missing-web"); err == nil {
		t.Fatal("expected missing reboot")
	}

	sv, err := st.CreateTransferServer(account, region, []string{"SFTP"})
	if err != nil {
		t.Fatal(err)
	}
	servers, err := st.ListTransferServers(account)
	if err != nil || len(servers) < 1 {
		t.Fatalf("servers=%v err=%v", servers, err)
	}
	userARN := store.TransferUserARN(region, account, sv.ServerID, "alice")
	if !strings.Contains(userARN, "user/"+sv.ServerID+"/alice") {
		t.Fatalf("user arn=%q", userARN)
	}
}

func TestExportedHelperZerosCoverage(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	if p := store.CognitoJWKSPath("us-east-1", "us-east-1_AbCdEf"); !strings.Contains(p, "jwks") {
		t.Fatalf("jwks path=%q", p)
	}
	msg := store.LambdaRuntimeValidationMessage()
	if msg == "" {
		t.Fatal("expected runtime validation message")
	}
	suffix, err := store.NewSecretARNSuffix()
	if err != nil || suffix == "" {
		t.Fatalf("secret suffix=%q err=%v", suffix, err)
	}
	acct, table, ok := store.ParseTableARN("arn:aws:dynamodb:us-east-1:" + account + ":table/lab")
	if !ok || acct != account || table != "lab" {
		t.Fatalf("parse table arn acct=%q table=%q ok=%v", acct, table, ok)
	}
	if _, _, ok := store.ParseTableARN("bad"); ok {
		t.Fatal("expected parse fail")
	}
	tbl := store.DynamoTable{RangeKeyName: "sk", GSIRangeKeyName: "gsi_sk", GSI2RangeKeyName: "gsi2_sk"}
	if !tbl.HasRangeKey() || !tbl.GSIHasRangeKey() || !tbl.GSI2HasRangeKey() {
		t.Fatal("expected range keys")
	}
	gsi := store.DynamoGSI{RangeKeyName: "sk"}
	if !gsi.HasRangeKey() {
		t.Fatal("expected gsi range")
	}
	trailARN := store.CloudTrailTrailARN("us-east-1", account, "x")
	if trailARN == "" {
		t.Fatal("empty trail arn")
	}

	key, err := st.CreateKey(account, "arn:aws:iam::"+account+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	sealed, _, err := st.SealPlaintextWithKMS(account, key.KeyID, []byte("plain"), nil)
	if err != nil {
		t.Fatal(err)
	}
	kid, err := store.KeyIDFromCiphertext(sealed)
	if err != nil || kid != key.KeyID {
		t.Fatalf("key id=%q err=%v want=%q", kid, err, key.KeyID)
	}
	if _, err := store.KeyIDFromCiphertext([]byte{1}); err == nil {
		t.Fatal("expected short ciphertext error")
	}
}
