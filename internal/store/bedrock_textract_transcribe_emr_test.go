package store_test

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestBedrockConverseAllowlist(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	body := []byte(`{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`)

	inv, err := st.ConverseBedrockModel(account, "anthropic.claude-3-haiku-20240307-v1:0", body)
	if err != nil {
		t.Fatal(err)
	}
	if inv.InvocationID == "" || !strings.Contains(inv.ResponseBody, "output") {
		t.Fatalf("inv=%#v body=%s", inv, inv.ResponseBody)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(inv.ResponseBody), &parsed); err != nil {
		t.Fatal(err)
	}
	out, _ := parsed["output"].(map[string]any)
	msg, _ := out["message"].(map[string]any)
	if msg["role"] != "assistant" {
		t.Fatalf("message=%#v", msg)
	}

	_, err = st.ConverseBedrockModel(account, "anthropic.claude-3-haiku-20240307-v1:0", []byte(`{}`))
	if !errors.Is(err, store.ErrBedrockValidation) {
		t.Fatalf("want validation, got %v", err)
	}

	_, err = st.ConverseBedrockModel(account, "unknown.model-v1", body)
	if !errors.Is(err, store.ErrBedrockResourceNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}

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

	c, err := st.RunEMRJobFlow(account, "us-east-1", store.EMRRunJobFlowInput{
		Name: "lab-cluster", ReleaseLabel: "emr-7.0.0", LogURI: "s3://logs/",
	})
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

	_, err = st.RunEMRJobFlow(account, "us-east-1", store.EMRRunJobFlowInput{})
	if !errors.Is(err, store.ErrEMRValidation) {
		t.Fatalf("want validation, got %v", err)
	}
}

func TestEMRStepLifecycle(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	c, err := st.RunEMRJobFlow(account, "us-east-1", store.EMRRunJobFlowInput{Name: "steps-cluster", ReleaseLabel: "emr-7.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := st.AddEMRJobFlowSteps(account, c.ClusterID, []store.EMRStepInput{
		{
			Name: "count",
			Jar:  "command-runner.jar",
			Args: []string{"echo", "ok"},
			Properties: map[string]string{
				"mapreduce.job.name": "lab",
			},
		},
	})
	if err != nil || len(ids) != 1 || !strings.HasPrefix(ids[0], "s-") {
		t.Fatalf("add steps: ids=%v err=%v", ids, err)
	}
	step, err := st.DescribeEMRStep(account, c.ClusterID, ids[0])
	if err != nil || step.State != "COMPLETED" || step.Name != "count" {
		t.Fatalf("describe step: %v %#v", err, step)
	}
	list, err := st.ListEMRSteps(account, c.ClusterID, nil, nil)
	if err != nil || len(list) != 1 || list[0].StepID != ids[0] {
		t.Fatalf("list steps: %v %#v", err, list)
	}
	filtered, err := st.ListEMRSteps(account, c.ClusterID, []string{"PENDING"}, nil)
	if err != nil || len(filtered) != 0 {
		t.Fatalf("filter state: %v %#v", err, filtered)
	}
	infos, err := st.CancelEMRSteps(account, c.ClusterID, []string{ids[0], "s-MISSING"})
	if err != nil || len(infos) != 2 {
		t.Fatalf("cancel: %v %#v", err, infos)
	}
	if infos[0].Status != "SUBMITTED" || infos[1].Status != "FAILED" {
		t.Fatalf("cancel infos=%#v", infos)
	}
	cancelled, err := st.DescribeEMRStep(account, c.ClusterID, ids[0])
	if err != nil || cancelled.State != "CANCELLED" {
		t.Fatalf("cancelled step: %v %#v", err, cancelled)
	}
	if err := st.TerminateEMRJobFlows(account, []string{c.ClusterID}); err != nil {
		t.Fatal(err)
	}
	_, err = st.AddEMRJobFlowSteps(account, c.ClusterID, []store.EMRStepInput{{Name: "x"}})
	if !errors.Is(err, store.ErrEMRNotFound) {
		t.Fatalf("want not found on terminated cluster, got %v", err)
	}
}

func TestEMRGroupsFleetsTagsSecurityConfig(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	c, err := st.RunEMRJobFlow(account, "us-east-1", store.EMRRunJobFlowInput{
		Name: "ig-cluster",
		Tags: map[string]string{"env": "lab"},
		Groups: []store.EMRInstanceGroupInput{
			{Name: "master", InstanceGroupType: "MASTER", InstanceType: "m5.xlarge", InstanceCount: 1},
			{Name: "core", InstanceGroupType: "CORE", InstanceType: "m5.xlarge", InstanceCount: 2, Market: "ON_DEMAND"},
		},
		Fleets: []store.EMRInstanceFleetInput{
			{Name: "master-fleet", InstanceFleetType: "MASTER", TargetOnDemandCapacity: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	groups, err := st.ListEMRInstanceGroups(account, c.ClusterID)
	if err != nil || len(groups) != 2 {
		t.Fatalf("groups: %v %#v", err, groups)
	}
	byType := map[string]store.EMRInstanceGroup{}
	for _, g := range groups {
		if g.State != "RUNNING" || !strings.HasPrefix(g.ID, "ig-") {
			t.Fatalf("groups detail=%#v", groups)
		}
		byType[g.InstanceGroupType] = g
	}
	if byType["MASTER"].RequestedInstanceCount != 1 || byType["CORE"].RequestedInstanceCount != 2 {
		t.Fatalf("groups detail=%#v", groups)
	}
	fleets, err := st.ListEMRInstanceFleets(account, c.ClusterID)
	if err != nil || len(fleets) != 1 || fleets[0].ProvisionedOnDemandCapacity != 1 {
		t.Fatalf("fleets: %v %#v", err, fleets)
	}
	if !strings.HasPrefix(fleets[0].ID, "if-") {
		t.Fatalf("fleet id=%q", fleets[0].ID)
	}

	got, err := st.DescribeEMRCluster(account, c.ClusterID)
	if err != nil || got.Tags["env"] != "lab" {
		t.Fatalf("tags on create: %v %#v", err, got.Tags)
	}
	if err := st.AddEMRTags(account, c.ClusterID, map[string]string{"owner": "qa"}); err != nil {
		t.Fatal(err)
	}
	got, err = st.DescribeEMRCluster(account, c.ClusterID)
	if err != nil || got.Tags["owner"] != "qa" || got.Tags["env"] != "lab" {
		t.Fatalf("after add tags: %v %#v", err, got.Tags)
	}
	if err := st.RemoveEMRTags(account, c.ClusterID, []string{"env"}); err != nil {
		t.Fatal(err)
	}
	got, err = st.DescribeEMRCluster(account, c.ClusterID)
	if err != nil || got.Tags["env"] != "" || got.Tags["owner"] != "qa" {
		t.Fatalf("after remove tags: %v %#v", err, got.Tags)
	}

	sc, err := st.CreateEMRSecurityConfiguration(account, "lab-sec", `{"EncryptionConfiguration":{}}`)
	if err != nil || sc.Name != "lab-sec" {
		t.Fatalf("create sc: %v %#v", err, sc)
	}
	desc, err := st.DescribeEMRSecurityConfiguration(account, "lab-sec")
	if err != nil || desc.SecurityConfiguration != `{"EncryptionConfiguration":{}}` {
		t.Fatalf("describe sc: %v %#v", err, desc)
	}
	list, err := st.ListEMRSecurityConfigurations(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list sc: %v %#v", err, list)
	}
	_, err = st.CreateEMRSecurityConfiguration(account, "lab-sec", `{}`)
	if !errors.Is(err, store.ErrEMRNotFound) {
		t.Fatalf("want duplicate not found, got %v", err)
	}
	if err := st.DeleteEMRSecurityConfiguration(account, "lab-sec"); err != nil {
		t.Fatal(err)
	}
	_, err = st.DescribeEMRSecurityConfiguration(account, "lab-sec")
	if !errors.Is(err, store.ErrEMRNotFound) {
		t.Fatalf("want missing after delete, got %v", err)
	}
}
