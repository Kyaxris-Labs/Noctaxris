package store_test

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestBedrockInvokeAllowlist(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	inv, err := st.InvokeBedrockModel(account, "amazon.titan-text-express-v1", "application/json", []byte(`{"inputText":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if inv.InvocationID == "" || !strings.Contains(inv.ResponseBody, "Noctaxris Bedrock stub") {
		t.Fatalf("inv=%#v", inv)
	}
	got, err := st.GetBedrockInvocation(account, inv.InvocationID)
	if err != nil || got.ModelID != "amazon.titan-text-express-v1" {
		t.Fatalf("get: %v %#v", err, got)
	}

	_, err = st.InvokeBedrockModel(account, "unknown.model-v1", "application/json", []byte(`{}`))
	if !errors.Is(err, store.ErrBedrockResourceNotFound) {
		t.Fatalf("want not found, got %v", err)
	}

	_, err = st.InvokeBedrockModel(account, "", "application/json", nil)
	if !errors.Is(err, store.ErrBedrockValidation) {
		t.Fatalf("want validation, got %v", err)
	}
}

func TestTextractDetectDocumentText(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	det, err := st.DetectDocumentTextStub(account, true, "", "")
	if err != nil {
		t.Fatal(err)
	}
	var blocks []map[string]any
	if err := json.Unmarshal([]byte(det.BlocksJSON), &blocks); err != nil || len(blocks) < 3 {
		t.Fatalf("blocks: %v %s", err, det.BlocksJSON)
	}
	if blocks[0]["BlockType"] != "PAGE" {
		t.Fatalf("first block=%#v", blocks[0])
	}

	_, err = st.DetectDocumentTextStub(account, false, "", "")
	if !errors.Is(err, store.ErrTextractInvalidParameter) {
		t.Fatalf("want invalid param, got %v", err)
	}

	_, err = st.DetectDocumentTextStub(account, false, "missing-bucket", "doc.png")
	if !errors.Is(err, store.ErrTextractInvalidS3Object) {
		t.Fatalf("want invalid s3, got %v", err)
	}

	if _, err := st.CreateBucket(account, "textract-lab"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(account, "textract-lab", "doc.png", store.PutObjectMeta{
		Data: []byte("fake-png"), PlainSize: 8, ContentType: "image/png",
	}); err != nil {
		t.Fatal(err)
	}
	det2, err := st.AnalyzeDocumentStub(account, false, "textract-lab", "doc.png", []string{"TABLES", "FORMS"})
	if err != nil || det2.FeatureTypes != "TABLES,FORMS" {
		t.Fatalf("analyze: %v %#v", err, det2)
	}
}

func TestTranscribeJobLifecycle(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	if _, err := st.CreateBucket(account, "bucket"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(account, "bucket", "audio.wav", store.PutObjectMeta{
		Data: []byte("fake-audio"), PlainSize: 10, ContentType: "audio/wav",
	}); err != nil {
		t.Fatal(err)
	}

	job, err := st.StartTranscriptionJobStub(account, "us-east-1", "job-1", "s3://bucket/audio.wav", "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if job.JobStatus != "COMPLETED" || job.TranscriptURI == "" {
		t.Fatalf("job=%#v", job)
	}
	path := strings.TrimPrefix(job.TranscriptURI, "file://")
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), "Noctaxris stub transcript") {
		t.Fatalf("transcript file: %v %s", err, raw)
	}

	got, err := st.GetTranscriptionJob(account, "job-1")
	if err != nil || got.MediaURI != "s3://bucket/audio.wav" {
		t.Fatalf("get: %v %#v", err, got)
	}
	list, err := st.ListTranscriptionJobs(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %#v", err, list)
	}

	_, err = st.StartTranscriptionJobStub(account, "us-east-1", "job-1", "s3://bucket/audio.wav", "en-US")
	if !errors.Is(err, store.ErrTranscribeConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	_, err = st.StartTranscriptionJobStub(account, "us-east-1", "", "s3://b/a", "en-US")
	if !errors.Is(err, store.ErrTranscribeBadRequest) {
		t.Fatalf("want bad request, got %v", err)
	}
	_, err = st.StartTranscriptionJobStub(account, "us-east-1", "job-https", "https://evil.example/a.wav", "en-US")
	if !errors.Is(err, store.ErrTranscribeBadRequest) {
		t.Fatalf("want bad request for non-s3 uri, got %v", err)
	}
	_, err = st.StartTranscriptionJobStub(account, "us-east-1", "job-missing", "s3://bucket/missing.wav", "en-US")
	if !errors.Is(err, store.ErrTranscribeBadRequest) {
		t.Fatalf("want bad request for missing object, got %v", err)
	}
}

func TestEMRClusterLifecycle(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	c, err := st.RunEMRJobFlow(account, "us-east-1", "lab-cluster", "emr-7.0.0", "s3://logs/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.ClusterID, "j-") || c.Status != "WAITING" {
		t.Fatalf("cluster=%#v", c)
	}
	got, err := st.DescribeEMRCluster(account, c.ClusterID)
	if err != nil || got.ClusterName != "lab-cluster" {
		t.Fatalf("describe: %v %#v", err, got)
	}
	list, err := st.ListEMRClusters(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %#v", err, list)
	}
	if err := st.TerminateEMRJobFlows(account, []string{c.ClusterID}); err != nil {
		t.Fatal(err)
	}
	term, err := st.DescribeEMRCluster(account, c.ClusterID)
	if err != nil || term.Status != "TERMINATED" {
		t.Fatalf("terminated: %v %#v", err, term)
	}

	_, err = st.RunEMRJobFlow(account, "us-east-1", "", "", "")
	if !errors.Is(err, store.ErrEMRValidation) {
		t.Fatalf("want validation, got %v", err)
	}
}
