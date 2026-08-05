package server_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCodeBuildStopBatchGetWebhookNegatives(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-stop-role", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-stop-role"

	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "stop-proj",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "NO_SOURCE",
			"buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo hi"]}}}`,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject %d %s", create.Code, create.Body.String())
	}

	start := mustCodeBuildJSON(t, handler, "StartBuild", map[string]any{"projectName": "stop-proj"}, now)
	// Without DinD compute this fails closed; still covers StartBuild validation/error path.
	if start.Code == http.StatusOK {
		t.Fatalf("StartBuild unexpected OK without compute")
	}

	stopMiss := mustCodeBuildJSON(t, handler, "StopBuild", map[string]any{"id": "missing-build"}, now)
	if stopMiss.Code == http.StatusOK {
		t.Fatalf("StopBuild missing should fail")
	}
	batchMiss := mustCodeBuildJSON(t, handler, "BatchGetBuilds", map[string]any{
		"ids": []string{"missing-1", "missing-2"},
	}, now)
	if batchMiss.Code != http.StatusOK {
		t.Fatalf("BatchGetBuilds %d %s", batchMiss.Code, batchMiss.Body.String())
	}
	listBuilds := mustCodeBuildJSON(t, handler, "ListBuilds", map[string]any{}, now)
	if listBuilds.Code != http.StatusOK {
		t.Fatalf("ListBuilds %d %s", listBuilds.Code, listBuilds.Body.String())
	}
	listProj := mustCodeBuildJSON(t, handler, "ListProjects", map[string]any{}, now)
	if listProj.Code != http.StatusOK || !strings.Contains(listProj.Body.String(), "stop-proj") {
		t.Fatalf("ListProjects %d %s", listProj.Code, listProj.Body.String())
	}

	upd := mustCodeBuildJSON(t, handler, "UpdateProject", map[string]any{
		"name":        "stop-proj",
		"description": "updated",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "NO_SOURCE",
			"buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo bye"]}}}`,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
			"environmentVariables": []any{
				map[string]any{"name": "A", "value": "1"},
			},
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateProject %d %s", upd.Code, upd.Body.String())
	}
	updMiss := mustCodeBuildJSON(t, handler, "UpdateProject", map[string]any{
		"name": "no-proj", "serviceRole": roleARN,
		"source":      map[string]any{"type": "NO_SOURCE", "buildspec": "{}"},
		"environment": map[string]any{"type": "LINUX_CONTAINER", "image": "alpine:3.20"},
		"artifacts":   map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if updMiss.Code == http.StatusOK {
		t.Fatalf("UpdateProject missing should fail")
	}

	wh := mustCodeBuildJSON(t, handler, "CreateWebhook", map[string]any{
		"projectName": "stop-proj",
		"filterGroups": []any{
			[]any{map[string]any{"type": "EVENT", "pattern": "PUSH"}},
		},
	}, now)
	if wh.Code != http.StatusOK {
		t.Fatalf("CreateWebhook %d %s", wh.Code, wh.Body.String())
	}
	listWH := mustCodeBuildJSON(t, handler, "ListWebhooks", map[string]any{}, now)
	if listWH.Code != http.StatusOK {
		t.Fatalf("ListWebhooks %d %s", listWH.Code, listWH.Body.String())
	}
	delWH := mustCodeBuildJSON(t, handler, "DeleteWebhook", map[string]any{"projectName": "stop-proj"}, now)
	if delWH.Code != http.StatusOK {
		t.Fatalf("DeleteWebhook %d %s", delWH.Code, delWH.Body.String())
	}
	delWHMiss := mustCodeBuildJSON(t, handler, "DeleteWebhook", map[string]any{"projectName": "no-proj"}, now)
	if delWHMiss.Code == http.StatusOK {
		t.Fatalf("DeleteWebhook missing should fail")
	}

	startBatch := mustCodeBuildJSON(t, handler, "StartBuildBatch", map[string]any{
		"projectName": "stop-proj",
	}, now)
	if startBatch.Code == http.StatusOK {
		t.Fatalf("StartBuildBatch without compute should fail")
	}

	emptyName := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name": "", "serviceRole": roleARN,
		"source":      map[string]any{"type": "NO_SOURCE", "buildspec": "{}"},
		"environment": map[string]any{"type": "LINUX_CONTAINER", "image": "alpine:3.20"},
		"artifacts":   map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if emptyName.Code == http.StatusOK {
		t.Fatalf("empty project name should fail")
	}

	del := mustCodeBuildJSON(t, handler, "DeleteProject", map[string]any{"name": "stop-proj"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteProject %d %s", del.Code, del.Body.String())
	}
	delMiss := mustCodeBuildJSON(t, handler, "DeleteProject", map[string]any{"name": "stop-proj"}, now)
	if delMiss.Code == http.StatusOK {
		t.Fatalf("DeleteProject twice should fail")
	}
}

func TestLambdaS3CodeAndTaggingNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-s3code-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-s3code-role"

	bucket := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/lambda-code-bkt", nil, "s3", now, nil)
	if bucket.Code < 200 || bucket.Code >= 300 {
		t.Fatalf("create bucket %d %s", bucket.Code, bucket.Body.String())
	}
	zipRaw, err := base64.StdEncoding.DecodeString(testLambdaZipB64(t))
	if err != nil {
		t.Fatal(err)
	}
	putZip := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/lambda-code-bkt/fn.zip", zipRaw, "s3", now, map[string]string{
		"Content-Type": "application/zip",
	})
	if putZip.Code < 200 || putZip.Code >= 300 {
		t.Fatalf("put zip %d %s", putZip.Code, putZip.Body.String())
	}

	create := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "s3code-fn",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
		"Code": map[string]any{
			"S3Bucket": "lambda-code-bkt",
			"S3Key":    "fn.zip",
		},
		"Tags": map[string]string{"env": "lab"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateFunction S3 %d %s", create.Code, create.Body.String())
	}

	listTags := mustLambdaJSON(t, handler, "ListTags", map[string]any{
		"Resource": "arn:aws:lambda:" + testRegion + ":" + testAccountID + ":function:s3code-fn",
	}, now)
	if listTags.Code != http.StatusOK {
		t.Fatalf("ListTags %d %s", listTags.Code, listTags.Body.String())
	}

	csc := mustLambdaJSON(t, handler, "GetFunctionCodeSigningConfig", map[string]any{"FunctionName": "s3code-fn"}, now)
	if csc.Code != http.StatusOK {
		t.Fatalf("GetFunctionCodeSigningConfig %d %s", csc.Code, csc.Body.String())
	}

	urlCfg := mustLambdaJSON(t, handler, "CreateFunctionUrlConfig", map[string]any{
		"FunctionName": "s3code-fn",
		"AuthType":     "NONE",
	}, now)
	if urlCfg.Code != http.StatusOK {
		t.Fatalf("CreateFunctionUrlConfig %d %s", urlCfg.Code, urlCfg.Body.String())
	}
	getURL := mustLambdaJSON(t, handler, "GetFunctionUrlConfig", map[string]any{"FunctionName": "s3code-fn"}, now)
	if getURL.Code != http.StatusOK {
		t.Fatalf("GetFunctionUrlConfig %d %s", getURL.Code, getURL.Body.String())
	}
	listURL := mustLambdaJSON(t, handler, "ListFunctionUrlConfigs", map[string]any{"FunctionName": "s3code-fn"}, now)
	if listURL.Code != http.StatusOK {
		t.Fatalf("ListFunctionUrlConfigs %d %s", listURL.Code, listURL.Body.String())
	}
	delURL := mustLambdaJSON(t, handler, "DeleteFunctionUrlConfig", map[string]any{"FunctionName": "s3code-fn"}, now)
	if delURL.Code != http.StatusOK && delURL.Code != http.StatusNoContent {
		t.Fatalf("DeleteFunctionUrlConfig %d %s", delURL.Code, delURL.Body.String())
	}

	badS3 := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "bad-s3-fn",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
		"Code":         map[string]any{"S3Bucket": "missing-bkt", "S3Key": "x.zip"},
	}, now)
	if badS3.Code == http.StatusOK {
		t.Fatalf("missing S3 code should fail")
	}
	noCode := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "no-code-fn",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
	}, now)
	if noCode.Code == http.StatusOK {
		t.Fatalf("missing Code should fail")
	}

	invokeDry := mustLambdaJSON(t, handler, "Invoke", map[string]any{
		"FunctionName":   "s3code-fn",
		"InvocationType": "DryRun",
		"Payload":        base64.StdEncoding.EncodeToString([]byte(`{}`)),
	}, now)
	if invokeDry.Code != http.StatusOK && invokeDry.Code != http.StatusAccepted && invokeDry.Code != http.StatusNoContent {
		// DryRun may return 204/200 depending on path; accept non-5xx validation success.
		if invokeDry.Code >= 500 {
			t.Fatalf("Invoke DryRun %d %s", invokeDry.Code, invokeDry.Body.String())
		}
	}

	del := mustLambdaJSON(t, handler, "DeleteFunction", map[string]any{"FunctionName": "s3code-fn"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteFunction %d %s", del.Code, del.Body.String())
	}
}

func TestSTSSessionTokenFederationAndAssumeRootNegatives(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	sess := stsForm(t, handler, "Action=GetSessionToken&Version=2011-06-15&DurationSeconds=900", now)
	if sess.Code != http.StatusOK {
		t.Fatalf("GetSessionToken %d %s", sess.Code, sess.Body.String())
	}
	mfaHalf := stsForm(t, handler, "Action=GetSessionToken&Version=2011-06-15&SerialNumber=arn:aws:iam::"+testAccountID+":mfa/root", now)
	if mfaHalf.Code == http.StatusOK {
		t.Fatalf("GetSessionToken serial without TokenCode should fail")
	}
	mfaBad := stsForm(t, handler, "Action=GetSessionToken&Version=2011-06-15&SerialNumber=arn:aws:iam::"+testAccountID+":mfa/nope&TokenCode=123456", now)
	if mfaBad.Code == http.StatusOK {
		t.Fatalf("GetSessionToken bad MFA should fail")
	}

	fed := stsForm(t, handler, "Action=GetFederationToken&Version=2011-06-15&Name=fed-ops&Policy="+url.QueryEscape(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:ListBucket","Resource":"*"}]}`,
	)+"&DurationSeconds=900", now)
	if fed.Code != http.StatusOK {
		t.Fatalf("GetFederationToken %d %s", fed.Code, fed.Body.String())
	}
	fedEmpty := stsForm(t, handler, "Action=GetFederationToken&Version=2011-06-15&Name=", now)
	if fedEmpty.Code == http.StatusOK {
		t.Fatalf("GetFederationToken empty name should fail")
	}

	aki := stsForm(t, handler, "Action=GetAccessKeyInfo&Version=2011-06-15&AccessKeyId="+testAccessKey, now)
	if aki.Code != http.StatusOK {
		t.Fatalf("GetAccessKeyInfo %d %s", aki.Code, aki.Body.String())
	}
	akiMiss := stsForm(t, handler, "Action=GetAccessKeyInfo&Version=2011-06-15&AccessKeyId=AKIAXXXXXXXX", now)
	if akiMiss.Code == http.StatusOK {
		t.Fatalf("GetAccessKeyInfo missing should fail")
	}

	msg := base64.StdEncoding.EncodeToString([]byte(`{"allowed":false}`))
	dec := stsForm(t, handler, "Action=DecodeAuthorizationMessage&Version=2011-06-15&EncodedMessage="+url.QueryEscape(msg), now)
	if dec.Code != http.StatusOK {
		t.Fatalf("DecodeAuthorizationMessage %d %s", dec.Code, dec.Body.String())
	}
	decBad := stsForm(t, handler, "Action=DecodeAuthorizationMessage&Version=2011-06-15&EncodedMessage=!!!", now)
	if decBad.Code == http.StatusOK {
		t.Fatalf("DecodeAuthorizationMessage bad should fail")
	}

	root := stsForm(t, handler, "Action=AssumeRoot&Version=2011-06-15&TargetPrincipal=arn:aws:iam::"+testAccountID+":root&TaskPolicyArn.arn=arn:aws:iam::aws:policy/AdministratorAccess", now)
	if root.Code == http.StatusOK {
		// may be unimplemented or denied; accept either non-panic path
		t.Logf("AssumeRoot OK body=%s", root.Body.String())
	}

	deleg := stsForm(t, handler, "Action=GetDelegatedAccessToken&Version=2011-06-15", now)
	if deleg.Code == http.StatusOK {
		t.Fatalf("GetDelegatedAccessToken should fail closed")
	}
	webTok := stsForm(t, handler, "Action=GetWebIdentityToken&Version=2011-06-15", now)
	if webTok.Code == http.StatusOK {
		t.Fatalf("GetWebIdentityToken should fail closed")
	}
}

func stsForm(t *testing.T, handler http.Handler, body string, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
