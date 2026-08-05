package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func iamForm(t *testing.T, handler http.Handler, body string, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "iam", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestIAMUserPolicyRoleAccessKeyCredentialReportCoverage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := iamForm(t, handler, "Action=CreateUser&Version=2010-05-08&UserName=cov-user", now)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateUser status=%d body=%q", rec.Code, rec.Body.String())
	}
	dup := iamForm(t, handler, "Action=CreateUser&Version=2010-05-08&UserName=cov-user", now)
	if dup.Code != http.StatusConflict {
		t.Fatalf("CreateUser dup want 409 status=%d body=%q", dup.Code, dup.Body.String())
	}
	empty := iamForm(t, handler, "Action=CreateUser&Version=2010-05-08&UserName=", now)
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("CreateUser empty want 400 status=%d body=%q", empty.Code, empty.Body.String())
	}

	get := iamForm(t, handler, "Action=GetUser&Version=2010-05-08&UserName=cov-user", now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "cov-user") {
		t.Fatalf("GetUser status=%d body=%q", get.Code, get.Body.String())
	}
	missing := iamForm(t, handler, "Action=GetUser&Version=2010-05-08&UserName=nope", now)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("GetUser missing want 404 status=%d body=%q", missing.Code, missing.Body.String())
	}
	list := iamForm(t, handler, "Action=ListUsers&Version=2010-05-08", now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "cov-user") {
		t.Fatalf("ListUsers status=%d body=%q", list.Code, list.Body.String())
	}

	ak := iamForm(t, handler, "Action=CreateAccessKey&Version=2010-05-08&UserName=cov-user", now)
	if ak.Code != http.StatusOK {
		t.Fatalf("CreateAccessKey status=%d body=%q", ak.Code, ak.Body.String())
	}
	accessKeyID := xmlTag(t, ak.Body.String(), "AccessKeyId")
	listAK := iamForm(t, handler, "Action=ListAccessKeys&Version=2010-05-08&UserName=cov-user", now)
	if listAK.Code != http.StatusOK || !strings.Contains(listAK.Body.String(), accessKeyID) {
		t.Fatalf("ListAccessKeys status=%d body=%q", listAK.Code, listAK.Body.String())
	}
	updAK := iamForm(t, handler, "Action=UpdateAccessKey&Version=2010-05-08&UserName=cov-user&AccessKeyId="+
		url.QueryEscape(accessKeyID)+"&Status=Inactive", now)
	if updAK.Code != http.StatusOK {
		t.Fatalf("UpdateAccessKey status=%d body=%q", updAK.Code, updAK.Body.String())
	}
	lastUsed := iamForm(t, handler, "Action=GetAccessKeyLastUsed&Version=2010-05-08&AccessKeyId="+url.QueryEscape(accessKeyID), now)
	if lastUsed.Code != http.StatusOK && lastUsed.Code != http.StatusNotFound {
		t.Fatalf("GetAccessKeyLastUsed status=%d body=%q", lastUsed.Code, lastUsed.Body.String())
	}

	policyDoc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`)
	pol := iamForm(t, handler, "Action=CreatePolicy&Version=2010-05-08&PolicyName=cov-pol&PolicyDocument="+policyDoc, now)
	if pol.Code != http.StatusOK {
		t.Fatalf("CreatePolicy status=%d body=%q", pol.Code, pol.Body.String())
	}
	policyARN := xmlTag(t, pol.Body.String(), "Arn")
	getPol := iamForm(t, handler, "Action=GetPolicy&Version=2010-05-08&PolicyArn="+url.QueryEscape(policyARN), now)
	if getPol.Code != http.StatusOK {
		t.Fatalf("GetPolicy status=%d body=%q", getPol.Code, getPol.Body.String())
	}
	listPol := iamForm(t, handler, "Action=ListPolicies&Version=2010-05-08", now)
	if listPol.Code != http.StatusOK || !strings.Contains(listPol.Body.String(), "cov-pol") {
		t.Fatalf("ListPolicies status=%d body=%q", listPol.Code, listPol.Body.String())
	}
	verDoc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`)
	newVer := iamForm(t, handler, "Action=CreatePolicyVersion&Version=2010-05-08&PolicyArn="+url.QueryEscape(policyARN)+
		"&PolicyDocument="+verDoc+"&SetAsDefault=true", now)
	if newVer.Code != http.StatusOK {
		t.Fatalf("CreatePolicyVersion status=%d body=%q", newVer.Code, newVer.Body.String())
	}
	listVer := iamForm(t, handler, "Action=ListPolicyVersions&Version=2010-05-08&PolicyArn="+url.QueryEscape(policyARN), now)
	if listVer.Code != http.StatusOK {
		t.Fatalf("ListPolicyVersions status=%d body=%q", listVer.Code, listVer.Body.String())
	}
	getVer := iamForm(t, handler, "Action=GetPolicyVersion&Version=2010-05-08&PolicyArn="+url.QueryEscape(policyARN)+"&VersionId=v1", now)
	if getVer.Code != http.StatusOK && getVer.Code != http.StatusNotFound {
		t.Fatalf("GetPolicyVersion status=%d body=%q", getVer.Code, getVer.Body.String())
	}

	attach := iamForm(t, handler, "Action=AttachUserPolicy&Version=2010-05-08&UserName=cov-user&PolicyArn="+url.QueryEscape(policyARN), now)
	if attach.Code != http.StatusOK {
		t.Fatalf("AttachUserPolicy status=%d body=%q", attach.Code, attach.Body.String())
	}
	listAtt := iamForm(t, handler, "Action=ListAttachedUserPolicies&Version=2010-05-08&UserName=cov-user", now)
	if listAtt.Code != http.StatusOK || !strings.Contains(listAtt.Body.String(), policyARN) {
		t.Fatalf("ListAttachedUserPolicies status=%d body=%q", listAtt.Code, listAtt.Body.String())
	}

	inline := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]}`)
	putInline := iamForm(t, handler, "Action=PutUserPolicy&Version=2010-05-08&UserName=cov-user&PolicyName=inline1&PolicyDocument="+inline, now)
	if putInline.Code != http.StatusOK {
		t.Fatalf("PutUserPolicy status=%d body=%q", putInline.Code, putInline.Body.String())
	}
	getInline := iamForm(t, handler, "Action=GetUserPolicy&Version=2010-05-08&UserName=cov-user&PolicyName=inline1", now)
	if getInline.Code != http.StatusOK {
		t.Fatalf("GetUserPolicy status=%d body=%q", getInline.Code, getInline.Body.String())
	}
	listInline := iamForm(t, handler, "Action=ListUserPolicies&Version=2010-05-08&UserName=cov-user", now)
	if listInline.Code != http.StatusOK || !strings.Contains(listInline.Body.String(), "inline1") {
		t.Fatalf("ListUserPolicies status=%d body=%q", listInline.Code, listInline.Body.String())
	}

	trust := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	role := iamForm(t, handler, "Action=CreateRole&Version=2010-05-08&RoleName=cov-role&AssumeRolePolicyDocument="+trust, now)
	if role.Code != http.StatusOK {
		t.Fatalf("CreateRole status=%d body=%q", role.Code, role.Body.String())
	}
	getRole := iamForm(t, handler, "Action=GetRole&Version=2010-05-08&RoleName=cov-role", now)
	if getRole.Code != http.StatusOK {
		t.Fatalf("GetRole status=%d body=%q", getRole.Code, getRole.Body.String())
	}
	listRoles := iamForm(t, handler, "Action=ListRoles&Version=2010-05-08", now)
	if listRoles.Code != http.StatusOK || !strings.Contains(listRoles.Body.String(), "cov-role") {
		t.Fatalf("ListRoles status=%d body=%q", listRoles.Code, listRoles.Body.String())
	}
	attachRole := iamForm(t, handler, "Action=AttachRolePolicy&Version=2010-05-08&RoleName=cov-role&PolicyArn="+url.QueryEscape(policyARN), now)
	if attachRole.Code != http.StatusOK {
		t.Fatalf("AttachRolePolicy status=%d body=%q", attachRole.Code, attachRole.Body.String())
	}
	listRoleAtt := iamForm(t, handler, "Action=ListAttachedRolePolicies&Version=2010-05-08&RoleName=cov-role", now)
	if listRoleAtt.Code != http.StatusOK {
		t.Fatalf("ListAttachedRolePolicies status=%d body=%q", listRoleAtt.Code, listRoleAtt.Body.String())
	}
	putRoleInline := iamForm(t, handler, "Action=PutRolePolicy&Version=2010-05-08&RoleName=cov-role&PolicyName=rin&PolicyDocument="+inline, now)
	if putRoleInline.Code != http.StatusOK {
		t.Fatalf("PutRolePolicy status=%d body=%q", putRoleInline.Code, putRoleInline.Body.String())
	}
	getRoleInline := iamForm(t, handler, "Action=GetRolePolicy&Version=2010-05-08&RoleName=cov-role&PolicyName=rin", now)
	if getRoleInline.Code != http.StatusOK {
		t.Fatalf("GetRolePolicy status=%d body=%q", getRoleInline.Code, getRoleInline.Body.String())
	}
	listRoleInline := iamForm(t, handler, "Action=ListRolePolicies&Version=2010-05-08&RoleName=cov-role", now)
	if listRoleInline.Code != http.StatusOK {
		t.Fatalf("ListRolePolicies status=%d body=%q", listRoleInline.Code, listRoleInline.Body.String())
	}
	updTrust := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	updAssume := iamForm(t, handler, "Action=UpdateAssumeRolePolicy&Version=2010-05-08&RoleName=cov-role&PolicyDocument="+updTrust, now)
	if updAssume.Code != http.StatusOK {
		t.Fatalf("UpdateAssumeRolePolicy status=%d body=%q", updAssume.Code, updAssume.Body.String())
	}

	gen := iamForm(t, handler, "Action=GenerateCredentialReport&Version=2010-05-08", now)
	if gen.Code != http.StatusOK {
		t.Fatalf("GenerateCredentialReport status=%d body=%q", gen.Code, gen.Body.String())
	}
	cred := iamForm(t, handler, "Action=GetCredentialReport&Version=2010-05-08", now)
	if cred.Code != http.StatusOK {
		t.Fatalf("GetCredentialReport status=%d body=%q", cred.Code, cred.Body.String())
	}

	delRoleInline := iamForm(t, handler, "Action=DeleteRolePolicy&Version=2010-05-08&RoleName=cov-role&PolicyName=rin", now)
	if delRoleInline.Code != http.StatusOK {
		t.Fatalf("DeleteRolePolicy status=%d body=%q", delRoleInline.Code, delRoleInline.Body.String())
	}
	detachRole := iamForm(t, handler, "Action=DetachRolePolicy&Version=2010-05-08&RoleName=cov-role&PolicyArn="+url.QueryEscape(policyARN), now)
	if detachRole.Code != http.StatusOK {
		t.Fatalf("DetachRolePolicy status=%d body=%q", detachRole.Code, detachRole.Body.String())
	}
	delRole := iamForm(t, handler, "Action=DeleteRole&Version=2010-05-08&RoleName=cov-role", now)
	if delRole.Code != http.StatusOK {
		t.Fatalf("DeleteRole status=%d body=%q", delRole.Code, delRole.Body.String())
	}
	delInline := iamForm(t, handler, "Action=DeleteUserPolicy&Version=2010-05-08&UserName=cov-user&PolicyName=inline1", now)
	if delInline.Code != http.StatusOK {
		t.Fatalf("DeleteUserPolicy status=%d body=%q", delInline.Code, delInline.Body.String())
	}
	detach := iamForm(t, handler, "Action=DetachUserPolicy&Version=2010-05-08&UserName=cov-user&PolicyArn="+url.QueryEscape(policyARN), now)
	if detach.Code != http.StatusOK {
		t.Fatalf("DetachUserPolicy status=%d body=%q", detach.Code, detach.Body.String())
	}
	delAK := iamForm(t, handler, "Action=DeleteAccessKey&Version=2010-05-08&UserName=cov-user&AccessKeyId="+url.QueryEscape(accessKeyID), now)
	if delAK.Code != http.StatusOK {
		t.Fatalf("DeleteAccessKey status=%d body=%q", delAK.Code, delAK.Body.String())
	}
	delUser := iamForm(t, handler, "Action=DeleteUser&Version=2010-05-08&UserName=cov-user", now)
	if delUser.Code != http.StatusOK {
		t.Fatalf("DeleteUser status=%d body=%q", delUser.Code, delUser.Body.String())
	}
	delUserGone := iamForm(t, handler, "Action=DeleteUser&Version=2010-05-08&UserName=cov-user", now)
	if delUserGone.Code != http.StatusNotFound {
		t.Fatalf("DeleteUser gone want 404 status=%d body=%q", delUserGone.Code, delUserGone.Body.String())
	}
	_ = iamForm(t, handler, "Action=DeletePolicyVersion&Version=2010-05-08&PolicyArn="+url.QueryEscape(policyARN)+"&VersionId=v1", now)
	delPol := iamForm(t, handler, "Action=DeletePolicy&Version=2010-05-08&PolicyArn="+url.QueryEscape(policyARN), now)
	if delPol.Code != http.StatusOK && delPol.Code != http.StatusBadRequest && delPol.Code != http.StatusNotFound {
		t.Fatalf("DeletePolicy status=%d body=%q", delPol.Code, delPol.Body.String())
	}
}

func TestKMSKeyLifecycleCoverage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "TrentService.CreateKey", "kms", map[string]any{
		"Description": "cov",
		"Tags":        []map[string]any{{"TagKey": "env", "TagValue": "lab"}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", create.Code, create.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	meta, _ := created["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)
	if keyID == "" {
		t.Fatalf("missing KeyId: %s", create.Body.String())
	}

	list := mustJSONTarget(t, handler, "TrentService.ListKeys", "kms", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), keyID) {
		t.Fatalf("ListKeys status=%d body=%q", list.Code, list.Body.String())
	}
	desc := mustJSONTarget(t, handler, "TrentService.DescribeKey", "kms", map[string]any{"KeyId": keyID}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeKey status=%d body=%q", desc.Code, desc.Body.String())
	}

	alias := mustJSONTarget(t, handler, "TrentService.CreateAlias", "kms", map[string]any{
		"AliasName": "alias/cov-key", "TargetKeyId": keyID,
	}, now)
	if alias.Code != http.StatusOK {
		t.Fatalf("CreateAlias status=%d body=%q", alias.Code, alias.Body.String())
	}
	listAlias := mustJSONTarget(t, handler, "TrentService.ListAliases", "kms", map[string]any{}, now)
	if listAlias.Code != http.StatusOK || !strings.Contains(listAlias.Body.String(), "alias/cov-key") {
		t.Fatalf("ListAliases status=%d body=%q", listAlias.Code, listAlias.Body.String())
	}
	updAlias := mustJSONTarget(t, handler, "TrentService.UpdateAlias", "kms", map[string]any{
		"AliasName": "alias/cov-key", "TargetKeyId": keyID,
	}, now)
	if updAlias.Code != http.StatusOK {
		t.Fatalf("UpdateAlias status=%d body=%q", updAlias.Code, updAlias.Body.String())
	}

	enc := mustJSONTarget(t, handler, "TrentService.Encrypt", "kms", map[string]any{
		"KeyId": keyID, "Plaintext": base64.StdEncoding.EncodeToString([]byte("hello")),
	}, now)
	if enc.Code != http.StatusOK {
		t.Fatalf("Encrypt status=%d body=%q", enc.Code, enc.Body.String())
	}
	var encOut map[string]any
	_ = json.Unmarshal(enc.Body.Bytes(), &encOut)
	blob, _ := encOut["CiphertextBlob"].(string)
	dec := mustJSONTarget(t, handler, "TrentService.Decrypt", "kms", map[string]any{
		"CiphertextBlob": blob,
	}, now)
	if dec.Code != http.StatusOK {
		t.Fatalf("Decrypt status=%d body=%q", dec.Code, dec.Body.String())
	}
	gdk := mustJSONTarget(t, handler, "TrentService.GenerateDataKey", "kms", map[string]any{
		"KeyId": keyID, "KeySpec": "AES_256",
	}, now)
	if gdk.Code != http.StatusOK {
		t.Fatalf("GenerateDataKey status=%d body=%q", gdk.Code, gdk.Body.String())
	}

	enableRot := mustJSONTarget(t, handler, "TrentService.EnableKeyRotation", "kms", map[string]any{"KeyId": keyID}, now)
	if enableRot.Code != http.StatusOK {
		t.Fatalf("EnableKeyRotation status=%d body=%q", enableRot.Code, enableRot.Body.String())
	}
	rotStatus := mustJSONTarget(t, handler, "TrentService.GetKeyRotationStatus", "kms", map[string]any{"KeyId": keyID}, now)
	if rotStatus.Code != http.StatusOK {
		t.Fatalf("GetKeyRotationStatus status=%d body=%q", rotStatus.Code, rotStatus.Body.String())
	}
	disableRot := mustJSONTarget(t, handler, "TrentService.DisableKeyRotation", "kms", map[string]any{"KeyId": keyID}, now)
	if disableRot.Code != http.StatusOK {
		t.Fatalf("DisableKeyRotation status=%d body=%q", disableRot.Code, disableRot.Body.String())
	}

	getPol := mustJSONTarget(t, handler, "TrentService.GetKeyPolicy", "kms", map[string]any{
		"KeyId": keyID, "PolicyName": "default",
	}, now)
	if getPol.Code != http.StatusOK {
		t.Fatalf("GetKeyPolicy status=%d body=%q", getPol.Code, getPol.Body.String())
	}
	putPol := mustJSONTarget(t, handler, "TrentService.PutKeyPolicy", "kms", map[string]any{
		"KeyId": keyID, "PolicyName": "default",
		"Policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"kms:*","Resource":"*"}]}`,
	}, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}

	tag := mustJSONTarget(t, handler, "TrentService.TagResource", "kms", map[string]any{
		"KeyId": keyID, "Tags": []map[string]any{{"TagKey": "team", "TagValue": "sec"}},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagResource status=%d body=%q", tag.Code, tag.Body.String())
	}
	listTags := mustJSONTarget(t, handler, "TrentService.ListResourceTags", "kms", map[string]any{"KeyId": keyID}, now)
	if listTags.Code != http.StatusOK {
		t.Fatalf("ListResourceTags status=%d body=%q", listTags.Code, listTags.Body.String())
	}
	untag := mustJSONTarget(t, handler, "TrentService.UntagResource", "kms", map[string]any{
		"KeyId": keyID, "TagKeys": []string{"team"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource status=%d body=%q", untag.Code, untag.Body.String())
	}

	grant := mustJSONTarget(t, handler, "TrentService.CreateGrant", "kms", map[string]any{
		"KeyId":             keyID,
		"GranteePrincipal":  "arn:aws:iam::" + testAccountID + ":root",
		"Operations":        []string{"Decrypt", "Encrypt"},
	}, now)
	if grant.Code != http.StatusOK {
		t.Fatalf("CreateGrant status=%d body=%q", grant.Code, grant.Body.String())
	}
	var grantOut map[string]any
	_ = json.Unmarshal(grant.Body.Bytes(), &grantOut)
	grantID, _ := grantOut["GrantId"].(string)
	listGrants := mustJSONTarget(t, handler, "TrentService.ListGrants", "kms", map[string]any{"KeyId": keyID}, now)
	if listGrants.Code != http.StatusOK {
		t.Fatalf("ListGrants status=%d body=%q", listGrants.Code, listGrants.Body.String())
	}
	if grantID != "" {
		retire := mustJSONTarget(t, handler, "TrentService.RetireGrant", "kms", map[string]any{
			"KeyId": keyID, "GrantId": grantID,
		}, now)
		if retire.Code != http.StatusOK {
			revoke := mustJSONTarget(t, handler, "TrentService.RevokeGrant", "kms", map[string]any{
				"KeyId": keyID, "GrantId": grantID,
			}, now)
			if revoke.Code != http.StatusOK {
				t.Fatalf("Retire/RevokeGrant status retire=%d revoke=%d", retire.Code, revoke.Code)
			}
		}
	}

	disable := mustJSONTarget(t, handler, "TrentService.DisableKey", "kms", map[string]any{"KeyId": keyID}, now)
	if disable.Code != http.StatusOK {
		t.Fatalf("DisableKey status=%d body=%q", disable.Code, disable.Body.String())
	}
	enable := mustJSONTarget(t, handler, "TrentService.EnableKey", "kms", map[string]any{"KeyId": keyID}, now)
	if enable.Code != http.StatusOK {
		t.Fatalf("EnableKey status=%d body=%q", enable.Code, enable.Body.String())
	}
	sched := mustJSONTarget(t, handler, "TrentService.ScheduleKeyDeletion", "kms", map[string]any{
		"KeyId": keyID, "PendingWindowInDays": 7,
	}, now)
	if sched.Code != http.StatusOK {
		t.Fatalf("ScheduleKeyDeletion status=%d body=%q", sched.Code, sched.Body.String())
	}
	cancel := mustJSONTarget(t, handler, "TrentService.CancelKeyDeletion", "kms", map[string]any{"KeyId": keyID}, now)
	if cancel.Code != http.StatusOK {
		t.Fatalf("CancelKeyDeletion status=%d body=%q", cancel.Code, cancel.Body.String())
	}

	delAlias := mustJSONTarget(t, handler, "TrentService.DeleteAlias", "kms", map[string]any{
		"AliasName": "alias/cov-key",
	}, now)
	if delAlias.Code != http.StatusOK {
		t.Fatalf("DeleteAlias status=%d body=%q", delAlias.Code, delAlias.Body.String())
	}
	missing := mustJSONTarget(t, handler, "TrentService.DescribeKey", "kms", map[string]any{"KeyId": "missing"}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("DescribeKey missing should fail: %q", missing.Body.String())
	}
}
