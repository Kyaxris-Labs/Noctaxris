package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGlueDatabaseTableCRUDNegatives(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureGlueSchema(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueDatabase(account, "", ""); !errors.Is(err, store.ErrGlueBadRequest) {
		t.Fatalf("empty name: %v", err)
	}
	db, err := st.CreateGlueDatabase(account, "covdb", "d")
	if err != nil || db.Name != "covdb" {
		t.Fatalf("create=%+v err=%v", db, err)
	}
	if _, err := st.CreateGlueDatabase(account, "covdb", ""); !errors.Is(err, store.ErrGlueAlreadyExists) {
		t.Fatalf("dup: %v", err)
	}
	got, err := st.GetGlueDatabase(account, "covdb")
	if err != nil || got.Description != "d" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	if _, err := st.GetGlueDatabase(account, "missing"); !errors.Is(err, store.ErrGlueNotFound) {
		t.Fatalf("missing db: %v", err)
	}
	list, err := st.GetGlueDatabases(account)
	if err != nil || len(list) < 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}

	tbl, err := st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName: "covdb", Name: "t1",
		Columns:         []store.GlueColumn{{Name: "id", Type: "string"}},
		StorageLocation: "s3://lab/cov/",
	})
	if err != nil || tbl.Name != "t1" {
		t.Fatalf("table=%+v err=%v", tbl, err)
	}
	gotT, err := st.GetGlueTable(account, "covdb", "t1")
	if err != nil || gotT.Name != "t1" {
		t.Fatalf("get table=%+v err=%v", gotT, err)
	}
	tables, err := st.GetGlueTables(account, "covdb")
	if err != nil || len(tables) != 1 {
		t.Fatalf("tables=%v err=%v", tables, err)
	}
	if err := st.DeleteGlueTable(account, "covdb", "t1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetGlueTable(account, "covdb", "t1"); !errors.Is(err, store.ErrGlueNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if err := st.DeleteGlueDatabase(account, "covdb"); err != nil {
		t.Fatal(err)
	}
}

func TestGroupsCRUDMembersPolicies(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureRoot(account, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(account, "alice"); err != nil {
		t.Fatal(err)
	}
	_, arn, err := st.CreateGroup(account, "devs")
	if err != nil || arn == "" {
		t.Fatalf("group arn=%q err=%v", arn, err)
	}
	if _, _, err := st.CreateGroup(account, "devs"); err == nil {
		t.Fatal("expected dup group")
	}
	g, err := st.GetGroup(account, "devs")
	if err != nil || g.GroupName != "devs" {
		t.Fatalf("get=%+v err=%v", g, err)
	}
	if _, err := st.GetGroup(account, "missing"); err == nil {
		t.Fatal("expected missing group")
	}
	groups, err := st.ListGroups(account)
	if err != nil || len(groups) < 1 {
		t.Fatalf("list=%v err=%v", groups, err)
	}
	if err := st.AddUserToGroup(account, "devs", "alice"); err != nil {
		t.Fatal(err)
	}
	members, err := st.ListGroupMembers(account, "devs")
	if err != nil || len(members) != 1 {
		t.Fatalf("members=%v err=%v", members, err)
	}
	ugs, err := st.ListGroupsForUser(account, "alice")
	if err != nil || len(ugs) != 1 {
		t.Fatalf("user groups=%v err=%v", ugs, err)
	}
	polARN, err := st.CreateManagedPolicy(account, "GroupPol", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachGroupPolicy(account, "devs", polARN); err != nil {
		t.Fatal(err)
	}
	attached, err := st.ListGroupPolicies(account, "devs")
	if err != nil || len(attached) != 1 {
		t.Fatalf("attached=%v err=%v", attached, err)
	}
	if err := st.PutGroupInlinePolicy(account, "devs", "inline", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sqs:*","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	if err := st.DetachGroupPolicy(account, "devs", polARN); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveUserFromGroup(account, "devs", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteGroup(account, "devs"); err != nil {
		t.Fatal(err)
	}
}

func TestServiceDiscoveryCRUDAndDiscover(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureServiceDiscoverySchema(); err != nil {
		t.Fatal(err)
	}
	ns, err := st.CreateSDNamespace(account, "us-east-1", "cov.local", "DNS_PRIVATE", "d", "vpc-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSDNamespace(account, ns.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSDNamespace(account, "missing"); err == nil {
		t.Fatal("expected missing ns")
	}
	list, err := st.ListSDNamespaces(account)
	if err != nil || len(list) < 1 {
		t.Fatalf("ns list=%v err=%v", list, err)
	}
	svc, err := st.CreateSDService(account, "us-east-1", ns.ID, "api", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSDService(account, svc.ID); err != nil {
		t.Fatal(err)
	}
	inst, err := st.RegisterSDInstance(account, svc.ID, "i1", map[string]string{"AWS_INSTANCE_IPV4": "10.0.0.5"})
	if err != nil || inst.InstanceID != "i1" {
		t.Fatalf("inst=%+v err=%v", inst, err)
	}
	found, err := st.DiscoverSDInstances(account, "cov.local", "api", "vpc-1")
	if err != nil || len(found) != 1 {
		t.Fatalf("discover=%v err=%v", found, err)
	}
	if err := st.DeregisterSDInstance(account, svc.ID, "i1"); err != nil {
		t.Fatal(err)
	}
	found, err = st.DiscoverSDInstances(account, "cov.local", "api", "vpc-1")
	if err != nil || len(found) != 0 {
		t.Fatalf("after deregister=%v err=%v", found, err)
	}
}

func TestS3VectorsCRUDQueryNegatives(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureS3VectorsSchema(); err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateS3VectorBucket(account, "vec-cov")
	if err != nil || b.Name != "vec-cov" {
		t.Fatalf("bucket=%+v err=%v", b, err)
	}
	if _, err := st.CreateS3VectorBucket(account, "vec-cov"); err == nil {
		t.Fatal("expected dup bucket")
	}
	if _, err := st.GetS3VectorBucket(account, "missing"); err == nil {
		t.Fatal("expected missing bucket")
	}
	buckets, err := st.ListS3VectorBuckets(account)
	if err != nil || len(buckets) < 1 {
		t.Fatalf("buckets=%v err=%v", buckets, err)
	}
	idx, err := st.CreateS3VectorIndex(account, "vec-cov", "idx", "cosine", 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetS3VectorIndex(account, "vec-cov", idx.IndexName); err != nil {
		t.Fatal(err)
	}
	indexes, err := st.ListS3VectorIndexes(account, "vec-cov")
	if err != nil || len(indexes) != 1 {
		t.Fatalf("indexes=%v err=%v", indexes, err)
	}
	if err := st.PutS3Vectors(account, "vec-cov", "idx", []store.S3Vector{
		{Key: "a", Data: []float64{1, 0, 0}},
		{Key: "b", Data: []float64{0, 1, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	matches, err := st.QueryS3Vectors(account, "vec-cov", "idx", []float64{1, 0, 0}, 1)
	if err != nil || len(matches) != 1 || matches[0].Key != "a" {
		t.Fatalf("matches=%v err=%v", matches, err)
	}
	if _, err := st.QueryS3Vectors(account, "vec-cov", "idx", []float64{1, 0}, 1); err == nil {
		t.Fatal("expected dimension mismatch")
	}
	if err := st.DeleteS3VectorIndex(account, "vec-cov", "idx"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteS3VectorBucket(account, "vec-cov"); err != nil {
		t.Fatal(err)
	}
}

func TestPipesCRUDAndNotFound(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsurePipesSchema(); err != nil {
		t.Fatal(err)
	}
	src, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-src", nil)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-dst", nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreatePipe(account, "us-east-1", "cov-pipe", "d", src.QueueARN, dst.QueueARN, "", "RUNNING")
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetPipe(account, "cov-pipe")
	if err != nil || got.Name != p.Name {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	if _, err := st.DescribePipe(account, "missing"); !errors.Is(err, store.ErrPipeNotFound) {
		t.Fatalf("expected missing pipe, err=%v", err)
	}
	list, err := st.ListPipes(account)
	if err != nil || len(list) < 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.SetPipeSourceCursor(account, "cov-pipe", "c1"); err != nil {
		t.Fatal(err)
	}
	running, err := st.ListRunningPipes()
	if err != nil || len(running) < 1 {
		t.Fatalf("running=%v err=%v", running, err)
	}
	if err := st.DeletePipe(account, "cov-pipe"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetPipe(account, "cov-pipe"); !errors.Is(err, store.ErrPipeNotFound) {
		t.Fatalf("expected deleted, err=%v", err)
	}
}

func TestFirehoseStreamCRUDAndPut(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureFirehoseSchema(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "fh-cov-dest"); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"s3:PutObject","Resource":"*"}]}`
	if err := st.PutBucketPolicy(account, "fh-cov-dest", policy); err != nil {
		t.Fatal(err)
	}
	stream, err := st.CreateFirehoseStream(account, "us-east-1", "cov-fh", "", "S3", "fh-cov-dest", "out/", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetFirehoseStream(account, stream.Name)
	if err != nil || got.Name != "cov-fh" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	if _, err := st.GetFirehoseStream(account, "missing"); err == nil {
		t.Fatal("expected missing stream")
	}
	list, err := st.ListFirehoseStreams(account)
	if err != nil || len(list) < 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	id, err := st.PutFirehoseRecord(account, "cov-fh", []byte(`{"ok":true}`))
	if err != nil || id == "" {
		t.Fatalf("put id=%q err=%v", id, err)
	}
	failed, err := st.PutFirehoseRecordBatch(account, "cov-fh", [][]byte{[]byte("a"), []byte("b")})
	if err != nil || len(failed) != 0 {
		t.Fatalf("batch failed=%v err=%v", failed, err)
	}
	if err := st.DeleteFirehoseStream(account, "cov-fh"); err != nil {
		t.Fatal(err)
	}
}

func TestBatchDescribeFiltersAndMissing(t *testing.T) {
	st := openBatchStore(t)
	account := "000000000001"
	region := store.DefaultBatchRegion
	ce, err := st.CreateBatchComputeEnvironment(account, region, store.CreateBatchComputeEnvironmentInput{
		Name: "cov-ce", Type: "MANAGED", ServiceRole: "arn:aws:iam::" + account + ":role/BatchServiceRole",
	})
	if err != nil {
		t.Fatal(err)
	}
	jq, err := st.CreateBatchJobQueue(account, region, store.CreateBatchJobQueueInput{
		Name: "cov-jq", Priority: 1,
		ComputeEnvironmentOrder: []map[string]any{{"order": 1, "computeEnvironment": ce.Name}},
	})
	if err != nil {
		t.Fatal(err)
	}
	jd, err := st.RegisterBatchJobDefinition(account, region, store.RegisterBatchJobDefinitionInput{
		Name: "cov-jd", Type: "container", Image: "alpine:3.20", Command: []string{"true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ces, err := st.DescribeBatchComputeEnvironments(account, []string{ce.Name, "missing"})
	if err != nil || len(ces) != 1 {
		t.Fatalf("ces=%v err=%v", ces, err)
	}
	jqs, err := st.DescribeBatchJobQueues(account, []string{jq.Name})
	if err != nil || len(jqs) != 1 {
		t.Fatalf("jqs=%v err=%v", jqs, err)
	}
	defs, err := st.DescribeBatchJobDefinitions(account, jd.Name)
	if err != nil || len(defs) < 1 {
		t.Fatalf("defs=%v err=%v", defs, err)
	}
	job, _, err := st.SubmitBatchJob(account, region, "job1", jq.Name, jd.Name)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := st.DescribeBatchJobs(account, []string{job.JobID, "missing"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	if err := st.SetBatchJobRuntime(account, job.JobID, "cid", "SUCCEEDED", "now"); err != nil {
		t.Fatal(err)
	}
}
