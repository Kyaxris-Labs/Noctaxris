package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openBatchStore(t *testing.T) *store.Store {
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

func TestBatchComputeQueueDefinitionJobLifecycle(t *testing.T) {
	st := openBatchStore(t)
	account := "000000000001"
	region := store.DefaultBatchRegion

	ce, err := st.CreateBatchComputeEnvironment(account, region, store.CreateBatchComputeEnvironmentInput{
		Name:        "lab-ce",
		Type:        "MANAGED",
		ServiceRole: "arn:aws:iam::" + account + ":role/BatchServiceRole",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ce.Status != store.BatchCEStatusValid {
		t.Fatalf("ce=%+v", ce)
	}

	jq, err := st.CreateBatchJobQueue(account, region, store.CreateBatchJobQueueInput{
		Name:     "lab-jq",
		Priority: 1,
		ComputeEnvironmentOrder: []map[string]any{
			{"order": 1, "computeEnvironment": ce.Name},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	jd, err := st.RegisterBatchJobDefinition(account, region, store.RegisterBatchJobDefinitionInput{
		Name:    "lab-jd",
		Type:    "container",
		Image:   "alpine:3.20",
		Command: []string{"echo", "batch-ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if jd.Revision != 1 {
		t.Fatalf("rev=%d", jd.Revision)
	}

	job, gotJD, err := st.SubmitBatchJob(account, region, "job-1", jq.Name, jd.Name)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != store.BatchJobStatusSubmitted || gotJD.ARN != jd.ARN {
		t.Fatalf("job=%+v jd=%+v", job, gotJD)
	}
	if err := st.SetBatchJobRuntime(account, job.JobID, "cid", store.BatchJobStatusSucceeded, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	jobs, err := st.DescribeBatchJobs(account, []string{job.JobID})
	if err != nil || len(jobs) != 1 || jobs[0].Status != store.BatchJobStatusSucceeded {
		t.Fatalf("jobs=%+v err=%v", jobs, err)
	}
}
