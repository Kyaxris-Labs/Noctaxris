package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCloudControlCoverageWave2GetListLiveFallback(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := store.EnsureCloudControlSchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}
	if err := st.EnsureCloudControlSchema(); err != nil {
		t.Fatal(err)
	}

	if _, err := st.CloudControlGetResource(account, "AWS::Noctaxris::Nope", "x"); !errors.Is(err, store.ErrCloudControlTypeUnsupported) {
		t.Fatalf("unsupported get: %v", err)
	}
	if _, err := st.CloudControlListResources(account, "AWS::Noctaxris::Nope"); !errors.Is(err, store.ErrCloudControlTypeUnsupported) {
		t.Fatalf("unsupported list: %v", err)
	}

	if _, err := st.CreateBucket(account, "cc-wave2-live"); err != nil {
		t.Fatal(err)
	}
	live, err := st.CloudControlGetResource(account, "AWS::S3::Bucket", "cc-wave2-live")
	if err != nil || live.Identifier != "cc-wave2-live" || !strings.Contains(live.Properties, "cc-wave2-live") {
		t.Fatalf("live get=%+v err=%v", live, err)
	}
	if _, err := st.CloudControlGetResource(account, "AWS::S3::Bucket", "missing-bucket"); !errors.Is(err, store.ErrCloudControlNotFound) {
		t.Fatalf("missing live get: %v", err)
	}

	listed, err := st.CloudControlListResources(account, "AWS::S3::Bucket")
	if err != nil || len(listed) == 0 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	found := false
	for _, r := range listed {
		if r.Identifier == "cc-wave2-live" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected live bucket in list: %+v", listed)
	}

	// Duplicate create maps through cloudControlMapProvisionErr (AlreadyExists or BadRequest).
	desired := `{"BucketName":"cc-wave2-live"}`
	if _, _, err := st.CloudControlCreateResource(account, "AWS::S3::Bucket", desired); err == nil {
		t.Fatal("dup create must fail")
	} else if !errors.Is(err, store.ErrCloudControlAlreadyExists) && !errors.Is(err, store.ErrCloudControlBadRequest) {
		t.Fatalf("dup create err=%v", err)
	}

	res, token, err := st.CloudControlCreateResource(account, "AWS::S3::Bucket", `{"BucketName":"cc-wave2-tracked"}`)
	if err != nil || token == "" || res.Identifier == "" {
		t.Fatalf("create=%+v token=%q err=%v", res, token, err)
	}
	got, err := st.CloudControlGetResource(account, "AWS::S3::Bucket", res.Identifier)
	if err != nil || got.Identifier != res.Identifier {
		t.Fatalf("tracked get=%+v err=%v", got, err)
	}
	listed2, err := st.CloudControlListResources(account, "AWS::S3::Bucket")
	if err != nil || len(listed2) < 2 {
		t.Fatalf("list after create=%v err=%v", listed2, err)
	}
}

func TestDynamoDBCoverageWave2HelpersGSIKeysARN(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"

	gsi := store.DynamoGSI{IndexName: "ByStatus", HashKeyName: "status", HashKeyType: store.KeyTypeString, RangeKeyName: "ts", RangeKeyType: store.KeyTypeString}
	if !gsi.HasRangeKey() {
		t.Fatal("GSI HasRangeKey expected true")
	}
	gsiHashOnly := store.DynamoGSI{IndexName: "ByPK", HashKeyName: "pk", HashKeyType: store.KeyTypeString}
	if gsiHashOnly.HasRangeKey() {
		t.Fatal("hash-only GSI HasRangeKey expected false")
	}

	if _, _, ok := store.ParseTableARN("not-arn"); ok {
		t.Fatal("invalid ARN")
	}
	if _, _, ok := store.ParseTableARN("arn:aws:dynamodb:us-east-1:1:table/"); ok {
		t.Fatal("empty table name")
	}
	if _, _, ok := store.ParseTableARN("arn:aws:dynamodb:us-east-1:1:table/a/b"); ok {
		t.Fatal("nested resource")
	}
	acct, name, ok := store.ParseTableARN("arn:aws:dynamodb:us-east-1:000000000001:table/Orders")
	if !ok || acct != account || name != "Orders" {
		t.Fatalf("parse acct=%q name=%q ok=%v", acct, name, ok)
	}

	gsis := []store.DynamoGSI{
		{IndexName: "G1", HashKeyName: "g1pk", HashKeyType: store.KeyTypeString, RangeKeyName: "g1sk", RangeKeyType: store.KeyTypeString},
		{IndexName: "G2", HashKeyName: "g2pk", HashKeyType: store.KeyTypeString, RangeKeyName: "g2sk", RangeKeyType: store.KeyTypeString},
	}
	tbl, err := st.CreateTableWithGSIs(account, "us-east-1", "Wave2Orders", "pk", store.KeyTypeString, "sk", store.KeyTypeString, "", "", gsis)
	if err != nil {
		t.Fatal(err)
	}
	if !tbl.GSIHasRangeKey() || !tbl.GSI2HasRangeKey() {
		t.Fatalf("want GSI range keys: %+v", tbl)
	}
	if err := st.PutItemBytesMultiGSI(account, "Wave2Orders",
		`{"S":"u1"}`, `{"S":"o1"}`, `{"S":"OPEN"}`, `{"S":"t1"}`, `{"S":"A"}`, `{"S":"t2"}`,
		[]byte(`{"pk":{"S":"u1"},"sk":{"S":"o1"}}`), false, nil); err != nil {
		t.Fatal(err)
	}
	gpk, gsk, err := st.GetItemGSIKeys(account, "Wave2Orders", `{"S":"u1"}`, `{"S":"o1"}`)
	if err != nil || gpk == "" || gsk == "" {
		t.Fatalf("gsi keys pk=%q sk=%q err=%v", gpk, gsk, err)
	}
	g2pk, g2sk, err := st.GetItemGSISlotKeys(account, "Wave2Orders", `{"S":"u1"}`, `{"S":"o1"}`, 2)
	if err != nil || g2pk == "" || g2sk == "" {
		t.Fatalf("gsi2 keys pk=%q sk=%q err=%v", g2pk, g2sk, err)
	}
	if _, _, err := st.GetItemGSIKeys(account, "Wave2Orders", `{"S":"u1"}`, `{"S":"missing"}`); !errors.Is(err, store.ErrNoSuchItem) {
		t.Fatalf("missing item: %v", err)
	}
}

func TestELBv2CoverageWave2DeleteGetMatch(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := store.EnsureELBv2Schema(nil); err == nil {
		t.Fatal("nil db must fail")
	}
	if err := st.EnsureELBv2Schema(); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteELBv2TargetGroup(account, "arn:aws:elasticloadbalancing:us-east-1:1:targetgroup/x/y"); !errors.Is(err, store.ErrELBv2TGNotFound) {
		t.Fatalf("delete missing tg: %v", err)
	}
	if err := st.DeleteELBv2Listener(account, "arn:aws:elasticloadbalancing:us-east-1:1:listener/app/x/y/z"); !errors.Is(err, store.ErrELBv2NotFound) {
		t.Fatalf("delete missing listener: %v", err)
	}
	if _, err := st.GetELBv2LoadBalancerByName(account, "missing"); !errors.Is(err, store.ErrELBv2NotFound) {
		t.Fatalf("missing lb by name: %v", err)
	}
	if _, err := st.GetELBv2ListenerByARN(account, "arn:missing"); !errors.Is(err, store.ErrELBv2ListenerNotFound) {
		t.Fatalf("missing listener by arn: %v", err)
	}

	lb, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "wave2-alb", "internet-facing", "application")
	if err != nil {
		t.Fatal(err)
	}
	byName, err := st.GetELBv2LoadBalancerByName(account, "wave2-alb")
	if err != nil || byName.ARN != lb.ARN {
		t.Fatalf("by name=%+v err=%v", byName, err)
	}
	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "wave2-tg", "lambda", "HTTP", 80)
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

	rule := store.ELBv2Rule{PathPatterns: []string{"/api*"}, HostHeaders: []string{"app.example.com"}}
	if !store.MatchELBv2RulePathHost(rule, "/api/v1", "app.example.com") {
		t.Fatal("expected path+host match")
	}
	if store.MatchELBv2RulePathHost(rule, "/other", "app.example.com") {
		t.Fatal("path mismatch must fail")
	}
	if store.MatchELBv2RulePathHost(rule, "/api/v1", "other.example.com") {
		t.Fatal("host mismatch must fail")
	}

	if err := st.DeleteELBv2Listener(account, listener.ListenerARN); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteELBv2TargetGroup(account, tg.ARN); err != nil {
		t.Fatal(err)
	}
}

func TestS3VectorsCoverageWave2ListDeleteKey(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := store.EnsureS3VectorsSchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}
	if err := st.EnsureS3VectorsSchema(); err != nil {
		t.Fatal(err)
	}

	empty, err := st.ListS3VectorBuckets(account)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty buckets=%v err=%v", empty, err)
	}
	if err := st.DeleteS3VectorBucket(account, "missing"); !errors.Is(err, store.ErrS3VectorsNotFound) {
		t.Fatalf("delete missing bucket: %v", err)
	}
	if err := st.DeleteS3VectorIndex(account, "b", "i"); !errors.Is(err, store.ErrS3VectorsNotFound) {
		t.Fatalf("delete missing index: %v", err)
	}

	key := store.NewS3VectorKey()
	if key == "" || key == store.NewS3VectorKey() {
		t.Fatalf("NewS3VectorKey not unique/empty: %q", key)
	}

	b, err := st.CreateS3VectorBucket(account, "wave2-vec")
	if err != nil {
		t.Fatal(err)
	}
	buckets, err := st.ListS3VectorBuckets(account)
	if err != nil || len(buckets) != 1 || buckets[0].Name != b.Name {
		t.Fatalf("buckets=%v err=%v", buckets, err)
	}
	emptyIdx, err := st.ListS3VectorIndexes(account, "wave2-vec")
	if err != nil || len(emptyIdx) != 0 {
		t.Fatalf("empty indexes=%v err=%v", emptyIdx, err)
	}
	idx, err := st.CreateS3VectorIndex(account, "wave2-vec", "idx", "cosine", 2)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := st.ListS3VectorIndexes(account, "wave2-vec")
	if err != nil || len(indexes) != 1 || indexes[0].IndexName != idx.IndexName {
		t.Fatalf("indexes=%v err=%v", indexes, err)
	}
	if err := st.PutS3Vectors(account, "wave2-vec", "idx", []store.S3Vector{
		{Key: key, Data: []float64{1, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteS3VectorIndex(account, "wave2-vec", "idx"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteS3VectorBucket(account, "wave2-vec"); err != nil {
		t.Fatal(err)
	}
}

func TestSFNCoverageWave2ListDeleteInvokerWaitPath(t *testing.T) {
	st := openSFNStore(t)
	account := "000000000001"
	if err := store.EnsureSFNSchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}
	if err := st.EnsureSFNSchema(); err != nil {
		t.Fatal(err)
	}

	empty, err := st.ListSFNStateMachines(account)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty sms=%v err=%v", empty, err)
	}
	if err := st.DeleteSFNStateMachine(account, "arn:aws:states:us-east-1:000000000001:stateMachine:missing"); !errors.Is(err, store.ErrSFNStateMachineNotFound) {
		t.Fatalf("delete missing: %v", err)
	}

	called := false
	st.SetSFNTaskInvoker(func(resourceARN, inputJSON string) (string, error) {
		called = true
		return `{"ok":true}`, nil
	})

	def := `{
  "StartAt": "WaitPath",
  "States": {
    "WaitPath": {
      "Type": "Wait",
      "SecondsPath": "$.secs",
      "Next": "Done"
    },
    "Done": { "Type": "Succeed" }
  }
}`
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "wave2-sm", def, "")
	if err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListSFNStateMachines(account)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	exec, err := st.StartSFNExecution(account, "us-east-1", sm.StateMachineARN, "run-wait-path", `{"secs":1}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != "SUCCEEDED" {
		t.Fatalf("status=%s error=%s cause=%s", exec.Status, exec.Error, exec.Cause)
	}

	roleARN, err := st.CreateRole(account, "wave2-sfn-role", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"states.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(roleARN, "put-events", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"events:PutEvents","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	bus, err := st.CreateEventBus(account, "us-east-1", "wave2-sfn-bus")
	if err != nil {
		t.Fatal(err)
	}
	pol := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"states.amazonaws.com"},"Action":"events:PutEvents","Resource":"*"}]}`
	if err := st.PutEventBusPolicy(account, bus.Name, pol); err != nil {
		t.Fatal(err)
	}
	eventsDef := `{
  "StartAt": "Put",
  "States": {
    "Put": {
      "Type": "Task",
      "Resource": "` + bus.ARN + `",
      "End": true
    }
  }
}`
	sm2, err := st.CreateSFNStateMachine(account, "us-east-1", "wave2-events-sm", eventsDef, roleARN)
	if err != nil {
		t.Fatal(err)
	}
	exec2, err := st.StartSFNExecution(account, "us-east-1", sm2.StateMachineARN, "run-events", `{"hello":"world"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exec2.Status != "SUCCEEDED" {
		t.Fatalf("events status=%s error=%s cause=%s", exec2.Status, exec2.Error, exec2.Cause)
	}

	if err := st.DeleteSFNStateMachine(account, sm.StateMachineARN); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSFNStateMachine(account, sm2.StateMachineARN); err != nil {
		t.Fatal(err)
	}
	after, err := st.ListSFNStateMachines(account)
	if err != nil || len(after) != 0 {
		t.Fatalf("after delete=%v err=%v", after, err)
	}
	_ = called
}
