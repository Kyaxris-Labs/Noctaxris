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

func TestBedrockConverseREST(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	body := []byte(`{"messages":[{"role":"user","content":[{"text":"hello"}]}]}`)
	model := "anthropic.claude-3-haiku-20240307-v1:0"
	path := "/model/" + url.PathEscape(model) + "/converse"
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "bedrock", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Converse status=%d body=%q", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	output, _ := out["output"].(map[string]any)
	message, _ := output["message"].(map[string]any)
	if message["role"] != "assistant" {
		t.Fatalf("output=%#v", out)
	}
	if out["stopReason"] != "end_turn" {
		t.Fatalf("stopReason=%v", out["stopReason"])
	}

	emptyBody := []byte(`{"messages":[]}`)
	emptyReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+path, emptyBody)
	emptyReq.Header.Set("Content-Type", "application/json")
	signHeader(t, emptyReq, emptyBody, testAccessKey, testSecret, testRegion, "bedrock", now)
	emptyRec := httptest.NewRecorder()
	handler.ServeHTTP(emptyRec, emptyReq)
	if emptyRec.Code != http.StatusBadRequest {
		t.Fatalf("empty messages status=%d body=%q", emptyRec.Code, emptyRec.Body.String())
	}

	streamPath := "/model/" + url.PathEscape(model) + "/converse-stream"
	streamReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+streamPath, body)
	streamReq.Header.Set("Content-Type", "application/json")
	signHeader(t, streamReq, body, testAccessKey, testSecret, testRegion, "bedrock", now)
	streamRec := httptest.NewRecorder()
	handler.ServeHTTP(streamRec, streamReq)
	if streamRec.Code != http.StatusNotImplemented {
		t.Fatalf("converse-stream status=%d body=%q", streamRec.Code, streamRec.Body.String())
	}
}

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
		"Tags": []map[string]any{
			{"Key": "team", "Value": "lab"},
		},
		"Instances": map[string]any{
			"InstanceGroups": []map[string]any{
				{"Name": "master", "InstanceRole": "MASTER", "InstanceType": "m5.xlarge", "InstanceCount": 1},
			},
			"InstanceFleets": []map[string]any{
				{"Name": "core-fleet", "InstanceFleetType": "CORE", "TargetOnDemandCapacity": 2},
			},
		},
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
	if !strings.Contains(desc.Body.String(), "team") {
		t.Fatalf("DescribeCluster missing tags body=%q", desc.Body.String())
	}

	groups := mustJSONTarget(t, handler, "ElasticMapReduce.ListInstanceGroups", "elasticmapreduce", map[string]any{
		"ClusterId": id,
	}, now)
	if groups.Code != http.StatusOK || !strings.Contains(groups.Body.String(), "MASTER") {
		t.Fatalf("ListInstanceGroups status=%d body=%q", groups.Code, groups.Body.String())
	}

	fleets := mustJSONTarget(t, handler, "ElasticMapReduce.ListInstanceFleets", "elasticmapreduce", map[string]any{
		"ClusterId": id,
	}, now)
	if fleets.Code != http.StatusOK || !strings.Contains(fleets.Body.String(), "CORE") {
		t.Fatalf("ListInstanceFleets status=%d body=%q", fleets.Code, fleets.Body.String())
	}

	list := mustJSONTarget(t, handler, "ElasticMapReduce.ListClusters", "elasticmapreduce", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), id) {
		t.Fatalf("ListClusters status=%d body=%q", list.Code, list.Body.String())
	}

	add := mustJSONTarget(t, handler, "ElasticMapReduce.AddJobFlowSteps", "elasticmapreduce", map[string]any{
		"JobFlowId": id,
		"Steps": []map[string]any{
			{
				"Name":            "smoke",
				"ActionOnFailure": "CONTINUE",
				"HadoopJarStep": map[string]any{
					"Jar":  "command-runner.jar",
					"Args": []string{"echo", "ok"},
				},
			},
		},
	}, now)
	if add.Code != http.StatusOK {
		t.Fatalf("AddJobFlowSteps status=%d body=%q", add.Code, add.Body.String())
	}
	var added map[string]any
	if err := json.Unmarshal(add.Body.Bytes(), &added); err != nil {
		t.Fatal(err)
	}
	stepIDs, _ := added["StepIds"].([]any)
	if len(stepIDs) != 1 {
		t.Fatalf("StepIds=%v body=%q", stepIDs, add.Body.String())
	}
	stepID, _ := stepIDs[0].(string)
	if !strings.HasPrefix(stepID, "s-") {
		t.Fatalf("StepId=%q", stepID)
	}

	descStep := mustJSONTarget(t, handler, "ElasticMapReduce.DescribeStep", "elasticmapreduce", map[string]any{
		"ClusterId": id,
		"StepId":    stepID,
	}, now)
	if descStep.Code != http.StatusOK || !strings.Contains(descStep.Body.String(), "COMPLETED") {
		t.Fatalf("DescribeStep status=%d body=%q", descStep.Code, descStep.Body.String())
	}

	listSteps := mustJSONTarget(t, handler, "ElasticMapReduce.ListSteps", "elasticmapreduce", map[string]any{
		"ClusterId": id,
	}, now)
	if listSteps.Code != http.StatusOK || !strings.Contains(listSteps.Body.String(), stepID) {
		t.Fatalf("ListSteps status=%d body=%q", listSteps.Code, listSteps.Body.String())
	}

	cancel := mustJSONTarget(t, handler, "ElasticMapReduce.CancelSteps", "elasticmapreduce", map[string]any{
		"ClusterId": id,
		"StepIds":   []string{stepID},
	}, now)
	if cancel.Code != http.StatusOK || !strings.Contains(cancel.Body.String(), "SUBMITTED") {
		t.Fatalf("CancelSteps status=%d body=%q", cancel.Code, cancel.Body.String())
	}
	descCancelled := mustJSONTarget(t, handler, "ElasticMapReduce.DescribeStep", "elasticmapreduce", map[string]any{
		"ClusterId": id,
		"StepId":    stepID,
	}, now)
	if descCancelled.Code != http.StatusOK || !strings.Contains(descCancelled.Body.String(), "CANCELLED") {
		t.Fatalf("DescribeStep after cancel status=%d body=%q", descCancelled.Code, descCancelled.Body.String())
	}

	addTag := mustJSONTarget(t, handler, "ElasticMapReduce.AddTags", "elasticmapreduce", map[string]any{
		"ResourceId": id,
		"Tags":       []map[string]any{{"Key": "owner", "Value": "qa"}},
	}, now)
	if addTag.Code != http.StatusOK {
		t.Fatalf("AddTags status=%d body=%q", addTag.Code, addTag.Body.String())
	}
	descTagged := mustJSONTarget(t, handler, "ElasticMapReduce.DescribeCluster", "elasticmapreduce", map[string]any{
		"ClusterId": id,
	}, now)
	if descTagged.Code != http.StatusOK || !strings.Contains(descTagged.Body.String(), "owner") {
		t.Fatalf("DescribeCluster after AddTags status=%d body=%q", descTagged.Code, descTagged.Body.String())
	}
	rmTag := mustJSONTarget(t, handler, "ElasticMapReduce.RemoveTags", "elasticmapreduce", map[string]any{
		"ResourceId": id,
		"TagKeys":    []string{"team"},
	}, now)
	if rmTag.Code != http.StatusOK {
		t.Fatalf("RemoveTags status=%d body=%q", rmTag.Code, rmTag.Body.String())
	}

	createSC := mustJSONTarget(t, handler, "ElasticMapReduce.CreateSecurityConfiguration", "elasticmapreduce", map[string]any{
		"Name":                  "srv-sec",
		"SecurityConfiguration": `{"EncryptionConfiguration":{}}`,
	}, now)
	if createSC.Code != http.StatusOK || !strings.Contains(createSC.Body.String(), "srv-sec") {
		t.Fatalf("CreateSecurityConfiguration status=%d body=%q", createSC.Code, createSC.Body.String())
	}
	descSC := mustJSONTarget(t, handler, "ElasticMapReduce.DescribeSecurityConfiguration", "elasticmapreduce", map[string]any{
		"Name": "srv-sec",
	}, now)
	if descSC.Code != http.StatusOK || !strings.Contains(descSC.Body.String(), "EncryptionConfiguration") {
		t.Fatalf("DescribeSecurityConfiguration status=%d body=%q", descSC.Code, descSC.Body.String())
	}
	listSC := mustJSONTarget(t, handler, "ElasticMapReduce.ListSecurityConfigurations", "elasticmapreduce", map[string]any{}, now)
	if listSC.Code != http.StatusOK || !strings.Contains(listSC.Body.String(), "srv-sec") {
		t.Fatalf("ListSecurityConfigurations status=%d body=%q", listSC.Code, listSC.Body.String())
	}
	delSC := mustJSONTarget(t, handler, "ElasticMapReduce.DeleteSecurityConfiguration", "elasticmapreduce", map[string]any{
		"Name": "srv-sec",
	}, now)
	if delSC.Code != http.StatusOK {
		t.Fatalf("DeleteSecurityConfiguration status=%d body=%q", delSC.Code, delSC.Body.String())
	}

	term := mustJSONTarget(t, handler, "ElasticMapReduce.TerminateJobFlows", "elasticmapreduce", map[string]any{
		"JobFlowIds": []string{id},
	}, now)
	if term.Code != http.StatusOK {
		t.Fatalf("TerminateJobFlows status=%d body=%q", term.Code, term.Body.String())
	}
}
