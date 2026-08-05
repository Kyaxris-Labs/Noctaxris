package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCFNListStacksChangeSetAndDrift(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	empty := mustCFNForm(t, handler, url.Values{
		"Action":  {"ListStacks"},
		"Version": {"2010-05-15"},
	}, now)
	if empty.Code != http.StatusOK {
		t.Fatalf("ListStacks empty status=%d body=%q", empty.Code, empty.Body.String())
	}

	tpl := `{"Resources":{"B":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-cov-bucket-1"}}}}`
	create := mustCFNForm(t, handler, url.Values{
		"Action":       {"CreateStack"},
		"Version":      {"2010-05-15"},
		"StackName":    {"cov-stack"},
		"TemplateBody": {tpl},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateStack status=%d body=%q", create.Code, create.Body.String())
	}

	list := mustCFNForm(t, handler, url.Values{
		"Action":  {"ListStacks"},
		"Version": {"2010-05-15"},
	}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "cov-stack") {
		t.Fatalf("ListStacks status=%d body=%q", list.Code, list.Body.String())
	}

	tpl2 := `{"Resources":{"B":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-cov-bucket-1"}},"B2":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-cov-bucket-2"}}}}`
	csMissingStack := mustCFNForm(t, handler, url.Values{
		"Action":        {"CreateChangeSet"},
		"Version":       {"2010-05-15"},
		"StackName":     {"no-stack"},
		"ChangeSetName": {"cs1"},
		"TemplateBody":  {tpl2},
	}, now)
	if csMissingStack.Code != http.StatusBadRequest {
		t.Fatalf("CreateChangeSet missing stack want 400 status=%d body=%q", csMissingStack.Code, csMissingStack.Body.String())
	}

	csBadParams := mustCFNForm(t, handler, url.Values{
		"Action":  {"CreateChangeSet"},
		"Version": {"2010-05-15"},
	}, now)
	if csBadParams.Code != http.StatusBadRequest || !strings.Contains(csBadParams.Body.String(), "ValidationError") {
		t.Fatalf("CreateChangeSet missing params want ValidationError status=%d body=%q", csBadParams.Code, csBadParams.Body.String())
	}

	cs := mustCFNForm(t, handler, url.Values{
		"Action":        {"CreateChangeSet"},
		"Version":       {"2010-05-15"},
		"StackName":     {"cov-stack"},
		"ChangeSetName": {"cov-cs"},
		"TemplateBody":  {tpl2},
	}, now)
	if cs.Code != http.StatusOK || (!strings.Contains(cs.Body.String(), "ChangeSetId") && !strings.Contains(cs.Body.String(), "<Id>")) {
		t.Fatalf("CreateChangeSet status=%d body=%q", cs.Code, cs.Body.String())
	}

	descCS := mustCFNForm(t, handler, url.Values{
		"Action":        {"DescribeChangeSet"},
		"Version":       {"2010-05-15"},
		"StackName":     {"cov-stack"},
		"ChangeSetName": {"cov-cs"},
	}, now)
	if descCS.Code != http.StatusOK || !strings.Contains(descCS.Body.String(), "cov-cs") {
		t.Fatalf("DescribeChangeSet status=%d body=%q", descCS.Code, descCS.Body.String())
	}

	descMissing := mustCFNForm(t, handler, url.Values{
		"Action":        {"DescribeChangeSet"},
		"Version":       {"2010-05-15"},
		"StackName":     {"cov-stack"},
		"ChangeSetName": {"missing"},
	}, now)
	if descMissing.Code != http.StatusBadRequest || !strings.Contains(descMissing.Body.String(), "ChangeSetNotFound") {
		t.Fatalf("DescribeChangeSet missing want ChangeSetNotFound status=%d body=%q", descMissing.Code, descMissing.Body.String())
	}

	exec := mustCFNForm(t, handler, url.Values{
		"Action":        {"ExecuteChangeSet"},
		"Version":       {"2010-05-15"},
		"StackName":     {"cov-stack"},
		"ChangeSetName": {"cov-cs"},
	}, now)
	if exec.Code != http.StatusOK {
		t.Fatalf("ExecuteChangeSet status=%d body=%q", exec.Code, exec.Body.String())
	}

	execMissing := mustCFNForm(t, handler, url.Values{
		"Action":        {"ExecuteChangeSet"},
		"Version":       {"2010-05-15"},
		"StackName":     {"cov-stack"},
		"ChangeSetName": {"no-such-cs"},
	}, now)
	if execMissing.Code != http.StatusBadRequest || !strings.Contains(execMissing.Body.String(), "ChangeSetNotFound") {
		t.Fatalf("ExecuteChangeSet missing want ChangeSetNotFound status=%d body=%q", execMissing.Code, execMissing.Body.String())
	}

	drift := mustCFNForm(t, handler, url.Values{
		"Action":    {"DetectStackDrift"},
		"Version":   {"2010-05-15"},
		"StackName": {"cov-stack"},
	}, now)
	if drift.Code != http.StatusOK || !strings.Contains(drift.Body.String(), "StackDriftDetectionId") {
		t.Fatalf("DetectStackDrift status=%d body=%q", drift.Code, drift.Body.String())
	}
	// Extract detection id from XML loosely.
	body := drift.Body.String()
	start := strings.Index(body, "<StackDriftDetectionId>")
	end := strings.Index(body, "</StackDriftDetectionId>")
	if start < 0 || end <= start {
		t.Fatalf("missing StackDriftDetectionId in %q", body)
	}
	detID := body[start+len("<StackDriftDetectionId>") : end]

	status := mustCFNForm(t, handler, url.Values{
		"Action":                 {"DescribeStackDriftDetectionStatus"},
		"Version":                {"2010-05-15"},
		"StackDriftDetectionId": {detID},
	}, now)
	if status.Code != http.StatusOK {
		t.Fatalf("DescribeStackDriftDetectionStatus status=%d body=%q", status.Code, status.Body.String())
	}

	statusMissing := mustCFNForm(t, handler, url.Values{
		"Action":                 {"DescribeStackDriftDetectionStatus"},
		"Version":                {"2010-05-15"},
		"StackDriftDetectionId": {"missing-det"},
	}, now)
	if statusMissing.Code != http.StatusBadRequest {
		t.Fatalf("DescribeStackDriftDetectionStatus missing want 400 status=%d body=%q", statusMissing.Code, statusMissing.Body.String())
	}

	resourceDrifts := mustCFNForm(t, handler, url.Values{
		"Action":    {"DescribeStackResourceDrifts"},
		"Version":   {"2010-05-15"},
		"StackName": {"cov-stack"},
	}, now)
	if resourceDrifts.Code != http.StatusOK {
		t.Fatalf("DescribeStackResourceDrifts status=%d body=%q", resourceDrifts.Code, resourceDrifts.Body.String())
	}

	resourceDriftsMissing := mustCFNForm(t, handler, url.Values{
		"Action":    {"DescribeStackResourceDrifts"},
		"Version":   {"2010-05-15"},
		"StackName": {"no-stack"},
	}, now)
	if resourceDriftsMissing.Code != http.StatusBadRequest {
		t.Fatalf("DescribeStackResourceDrifts missing want 400 status=%d body=%q", resourceDriftsMissing.Code, resourceDriftsMissing.Body.String())
	}

	driftMissing := mustCFNForm(t, handler, url.Values{
		"Action":    {"DetectStackDrift"},
		"Version":   {"2010-05-15"},
		"StackName": {"no-stack"},
	}, now)
	if driftMissing.Code != http.StatusBadRequest {
		t.Fatalf("DetectStackDrift missing want 400 status=%d body=%q", driftMissing.Code, driftMissing.Body.String())
	}
}

func TestCFNUpdateStackHappyAndNegatives(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	tpl := `{"Resources":{"B":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-upd-bucket-1"}}}}`
	create := mustCFNForm(t, handler, url.Values{
		"Action":       {"CreateStack"},
		"Version":      {"2010-05-15"},
		"StackName":    {"upd-stack"},
		"TemplateBody": {tpl},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateStack status=%d body=%q", create.Code, create.Body.String())
	}

	badParams := mustCFNForm(t, handler, url.Values{
		"Action":  {"UpdateStack"},
		"Version": {"2010-05-15"},
	}, now)
	if badParams.Code != http.StatusBadRequest || !strings.Contains(badParams.Body.String(), "ValidationError") {
		t.Fatalf("UpdateStack missing params want ValidationError status=%d body=%q", badParams.Code, badParams.Body.String())
	}

	missing := mustCFNForm(t, handler, url.Values{
		"Action":       {"UpdateStack"},
		"Version":      {"2010-05-15"},
		"StackName":    {"no-stack"},
		"TemplateBody": {tpl},
	}, now)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("UpdateStack missing stack want 400 status=%d body=%q", missing.Code, missing.Body.String())
	}

	tpl2 := `{"Resources":{"B":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-upd-bucket-1"}},"B2":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-upd-bucket-2"}}}}`
	upd := mustCFNForm(t, handler, url.Values{
		"Action":       {"UpdateStack"},
		"Version":      {"2010-05-15"},
		"StackName":    {"upd-stack"},
		"TemplateBody": {tpl2},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateStack status=%d body=%q", upd.Code, upd.Body.String())
	}
}
