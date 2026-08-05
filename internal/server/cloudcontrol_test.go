package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCloudControlGetUpdateAndRequestStatus(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "CloudControlApi.CreateResource", "cloudcontrol", map[string]any{
		"TypeName":     "AWS::S3::Bucket",
		"DesiredState": `{"BucketName":"cc-cov-bucket"}`,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateResource status=%d body=%q", create.Code, create.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	pe, _ := created["ProgressEvent"].(map[string]any)
	token, _ := pe["RequestToken"].(string)
	if token == "" {
		// Some payloads flatten ProgressEvent fields to top level.
		token, _ = created["RequestToken"].(string)
	}
	if token == "" {
		t.Fatalf("missing RequestToken in %s", create.Body.String())
	}

	get := mustJSONTarget(t, handler, "CloudControlApi.GetResource", "cloudcontrol", map[string]any{
		"TypeName":   "AWS::S3::Bucket",
		"Identifier": "cc-cov-bucket",
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "cc-cov-bucket") {
		t.Fatalf("GetResource status=%d body=%q", get.Code, get.Body.String())
	}

	getMissing := mustJSONTarget(t, handler, "CloudControlApi.GetResource", "cloudcontrol", map[string]any{
		"TypeName":   "AWS::S3::Bucket",
		"Identifier": "missing-bucket",
	}, now)
	if getMissing.Code != http.StatusBadRequest || !strings.Contains(getMissing.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("GetResource missing want ResourceNotFound status=%d body=%q", getMissing.Code, getMissing.Body.String())
	}

	getUnsupported := mustJSONTarget(t, handler, "CloudControlApi.GetResource", "cloudcontrol", map[string]any{
		"TypeName":   "AWS::EC2::Instance",
		"Identifier": "i-123",
	}, now)
	if getUnsupported.Code != http.StatusBadRequest || !strings.Contains(getUnsupported.Body.String(), "UnsupportedActionException") {
		t.Fatalf("GetResource unsupported want UnsupportedAction status=%d body=%q", getUnsupported.Code, getUnsupported.Body.String())
	}

	status := mustJSONTarget(t, handler, "CloudControlApi.GetResourceRequestStatus", "cloudcontrol", map[string]any{
		"RequestToken": token,
	}, now)
	if status.Code != http.StatusOK {
		t.Fatalf("GetResourceRequestStatus status=%d body=%q", status.Code, status.Body.String())
	}

	statusMissing := mustJSONTarget(t, handler, "CloudControlApi.GetResourceRequestStatus", "cloudcontrol", map[string]any{
		"RequestToken": "missing-token",
	}, now)
	if statusMissing.Code != http.StatusNotFound || !strings.Contains(statusMissing.Body.String(), "RequestTokenNotFoundException") {
		t.Fatalf("GetResourceRequestStatus missing want NotFound status=%d body=%q", statusMissing.Code, statusMissing.Body.String())
	}

	upd := mustJSONTarget(t, handler, "CloudControlApi.UpdateResource", "cloudcontrol", map[string]any{
		"TypeName":   "AWS::S3::Bucket",
		"Identifier": "cc-cov-bucket",
		"PatchDocument": `{"BucketEncryption":{"ServerSideEncryptionConfiguration":[{"ServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}}`,
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateResource status=%d body=%q", upd.Code, upd.Body.String())
	}

	updDesired := mustJSONTarget(t, handler, "CloudControlApi.UpdateResource", "cloudcontrol", map[string]any{
		"TypeName":     "AWS::S3::Bucket",
		"Identifier":   "cc-cov-bucket",
		"DesiredState": `{"BucketEncryption":{"ServerSideEncryptionConfiguration":[{"ServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}}`,
	}, now)
	if updDesired.Code != http.StatusOK {
		t.Fatalf("UpdateResource DesiredState status=%d body=%q", updDesired.Code, updDesired.Body.String())
	}

	updBad := mustJSONTarget(t, handler, "CloudControlApi.UpdateResource", "cloudcontrol", map[string]any{
		"TypeName":      "AWS::S3::Bucket",
		"Identifier":    "cc-cov-bucket",
		"PatchDocument": `{not-json`,
	}, now)
	if updBad.Code != http.StatusBadRequest {
		t.Fatalf("UpdateResource bad patch want 400 status=%d body=%q", updBad.Code, updBad.Body.String())
	}

	updUnsupported := mustJSONTarget(t, handler, "CloudControlApi.UpdateResource", "cloudcontrol", map[string]any{
		"TypeName":      "AWS::EC2::Instance",
		"Identifier":    "i-123",
		"PatchDocument": `[]`,
	}, now)
	if updUnsupported.Code != http.StatusBadRequest || !strings.Contains(updUnsupported.Body.String(), "UnsupportedTypeException") {
		t.Fatalf("UpdateResource unsupported want UnsupportedType status=%d body=%q", updUnsupported.Code, updUnsupported.Body.String())
	}

	unknown := mustJSONTarget(t, handler, "CloudControlApi.CancelResourceRequest", "cloudcontrol", map[string]any{}, now)
	if unknown.Code != http.StatusNotImplemented || !strings.Contains(unknown.Body.String(), "InvalidAction") {
		t.Fatalf("unknown CloudControl action want InvalidAction status=%d body=%q", unknown.Code, unknown.Body.String())
	}
}
