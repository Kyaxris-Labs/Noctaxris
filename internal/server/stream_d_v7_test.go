package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestBedrockInvokeModelREST(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	body := []byte(`{"inputText":"hello"}`)
	path := "/model/" + url.PathEscape("amazon.titan-text-express-v1") + "/invoke"
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "bedrock", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("InvokeModel status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Noctaxris Bedrock stub") {
		t.Fatalf("body=%q", rec.Body.String())
	}

	badBody := []byte(`{"inputText":"x"}`)
	badPath := "/model/unknown.model-v9/invoke"
	badReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+badPath, badBody)
	badReq.Header.Set("Content-Type", "application/json")
	signHeader(t, badReq, badBody, testAccessKey, testSecret, testRegion, "bedrock", now)
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code == http.StatusOK {
		t.Fatalf("expected reject unknown model, got %d %q", badRec.Code, badRec.Body.String())
	}
}

func TestTextractDetectDocumentText(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustJSONTarget(t, handler, "Textract.DetectDocumentText", "textract", map[string]any{
		"Document": map[string]any{"Bytes": "aGVsbG8="},
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("DetectDocumentText status=%d body=%q", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	blocks, _ := out["Blocks"].([]any)
	if len(blocks) < 3 {
		t.Fatalf("blocks=%#v", out)
	}

	bad := mustJSONTarget(t, handler, "Textract.DetectDocumentText", "textract", map[string]any{
		"Document": map[string]any{},
	}, now)
	if bad.Code == http.StatusOK {
		t.Fatalf("expected invalid parameter, got %d", bad.Code)
	}
}

func TestTranscribeStartGetList(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateBucket(testAccountID, "lab"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(testAccountID, "lab", "audio.wav", store.PutObjectMeta{
		Data: []byte("fake-audio"), PlainSize: 10, ContentType: "audio/wav",
	}); err != nil {
		t.Fatal(err)
	}

	start := mustJSONTarget(t, handler, "Transcribe.StartTranscriptionJob", "transcribe", map[string]any{
		"TranscriptionJobName": "srv-job-1",
		"LanguageCode":         "en-US",
		"Media":                map[string]any{"MediaFileUri": "s3://lab/audio.wav"},
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartTranscriptionJob status=%d body=%q", start.Code, start.Body.String())
	}
	if !strings.Contains(start.Body.String(), "COMPLETED") {
		t.Fatalf("body=%q", start.Body.String())
	}

	get := mustJSONTarget(t, handler, "Transcribe.GetTranscriptionJob", "transcribe", map[string]any{
		"TranscriptionJobName": "srv-job-1",
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "TranscriptFileUri") {
		t.Fatalf("GetTranscriptionJob status=%d body=%q", get.Code, get.Body.String())
	}

	list := mustJSONTarget(t, handler, "Transcribe.ListTranscriptionJobs", "transcribe", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "srv-job-1") {
		t.Fatalf("ListTranscriptionJobs status=%d body=%q", list.Code, list.Body.String())
	}
}

func TestEMRRunDescribeListTerminate(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	run := mustJSONTarget(t, handler, "ElasticMapReduce.RunJobFlow", "elasticmapreduce", map[string]any{
		"Name":         "srv-emr",
		"ReleaseLabel": "emr-7.0.0",
		"LogUri":       "s3://logs/",
	}, now)
	if run.Code != http.StatusOK {
		t.Fatalf("RunJobFlow status=%d body=%q", run.Code, run.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(run.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["JobFlowId"].(string)
	if !strings.HasPrefix(id, "j-") {
		t.Fatalf("JobFlowId=%q", id)
	}

	desc := mustJSONTarget(t, handler, "ElasticMapReduce.DescribeCluster", "elasticmapreduce", map[string]any{
		"ClusterId": id,
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "WAITING") {
		t.Fatalf("DescribeCluster status=%d body=%q", desc.Code, desc.Body.String())
	}

	list := mustJSONTarget(t, handler, "ElasticMapReduce.ListClusters", "elasticmapreduce", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), id) {
		t.Fatalf("ListClusters status=%d body=%q", list.Code, list.Body.String())
	}

	term := mustJSONTarget(t, handler, "ElasticMapReduce.TerminateJobFlows", "elasticmapreduce", map[string]any{
		"JobFlowIds": []string{id},
	}, now)
	if term.Code != http.StatusOK {
		t.Fatalf("TerminateJobFlows status=%d body=%q", term.Code, term.Body.String())
	}
}
