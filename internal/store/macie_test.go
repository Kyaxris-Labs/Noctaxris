package store_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMacieSessionJobAndFindings(t *testing.T) {
	st := openKMSStore(t)
	acct := "000000000001"

	if _, err := st.GetMacieSession(acct); err == nil {
		t.Fatal("expected not enabled")
	}
	if _, err := st.EnableMacie(acct); err != nil {
		t.Fatal(err)
	}
	sess, err := st.GetMacieSession(acct)
	if err != nil || sess.Status != "ENABLED" {
		t.Fatalf("session=%+v err=%v", sess, err)
	}

	job, err := st.CreateMacieClassificationJob(acct, "us-east-1", "lab-job", "ONE_TIME", map[string]any{
		"bucketDefinitions": []map[string]any{{"accountId": acct, "buckets": []string{"lab"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.JobStatus != "COMPLETE" || job.JobID == "" {
		t.Fatalf("job=%+v", job)
	}
	gotJob, err := st.DescribeMacieClassificationJob(acct, job.JobID)
	if err != nil || gotJob.Name != "lab-job" {
		t.Fatalf("describe=%+v err=%v", gotJob, err)
	}
	jobs, err := st.ListMacieClassificationJobs(acct, 10)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("list=%v err=%v", jobs, err)
	}

	ids, err := st.InjectMacieFindings(acct, "us-east-1", []store.MacieFinding{
		{
			Type:  "SensitiveData:S3Object/Personal",
			Title: "lab finding",
			ResourcesAffected: map[string]any{
				"s3Bucket": map[string]any{"name": "lab"},
				"s3Object": map[string]any{"key": "pii.txt"},
			},
		},
	})
	if err != nil || len(ids) != 1 {
		t.Fatalf("inject ids=%v err=%v", ids, err)
	}
	listed, err := st.ListMacieFindingIDs(acct, 10)
	if err != nil || len(listed) != 1 || listed[0] != ids[0] {
		t.Fatalf("listed=%v want %v", listed, ids)
	}
	got, err := st.GetMacieFindings(acct, ids)
	if err != nil || len(got) != 1 || got[0].Type == "" {
		t.Fatalf("findings=%+v err=%v", got, err)
	}
}

func TestMacieInjectFromS3CannedMatches(t *testing.T) {
	st := openKMSStore(t)
	acct := "000000000001"
	if _, err := st.EnableMacie(acct); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(acct, "macie-lab"); err != nil {
		t.Fatal(err)
	}
	body := []byte("contact 123-45-6789 and key AKIAIOSFODNN7EXAMPLE card 4111-1111-1111-1111\n")
	if _, err := st.PutObject(acct, "macie-lab", "secrets.txt", store.PutObjectMeta{
		Data:        body,
		ContentType: "text/plain",
		PlainSize:   int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}
	ids, err := st.InjectMacieFindingsFromS3Objects(acct, "us-east-1", "job-1", []store.MacieS3ObjectRef{
		{Bucket: "macie-lab", Key: "secrets.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) < 2 {
		t.Fatalf("expected multiple canned findings, got %v", ids)
	}
	findings, err := st.GetMacieFindings(acct, ids)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, f := range findings {
		types = append(types, f.Type)
	}
	joined := strings.Join(types, ",")
	if !strings.Contains(joined, "Personal") || !strings.Contains(joined, "Credentials") {
		t.Fatalf("types=%v", types)
	}
}
