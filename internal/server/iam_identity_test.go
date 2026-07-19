package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIAMGroupsBoundariesInstanceProfilesIdP(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "iam", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := post("Action=CreateGroup&Version=2010-05-08&GroupName=Admins")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<CreateGroupResponse") {
		t.Fatalf("CreateGroup status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = post("Action=CreateUser&Version=2010-05-08&UserName=alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateUser status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = post("Action=AddUserToGroup&Version=2010-05-08&GroupName=Admins&UserName=alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("AddUserToGroup status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = post("Action=GetGroup&Version=2010-05-08&GroupName=Admins")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<UserName>alice</UserName>") {
		t.Fatalf("GetGroup status=%d body=%q", rec.Code, rec.Body.String())
	}

	policyDoc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:ListUsers","Resource":"*"}]}`)
	rec = post("Action=CreatePolicy&Version=2010-05-08&PolicyName=GroupListUsers&PolicyDocument=" + policyDoc)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreatePolicy status=%d body=%q", rec.Code, rec.Body.String())
	}
	policyARN := xmlTag(t, rec.Body.String(), "Arn")

	rec = post("Action=AttachGroupPolicy&Version=2010-05-08&GroupName=Admins&PolicyArn=" + url.QueryEscape(policyARN))
	if rec.Code != http.StatusOK {
		t.Fatalf("AttachGroupPolicy status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = post("Action=ListAttachedGroupPolicies&Version=2010-05-08&GroupName=Admins")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), policyARN) {
		t.Fatalf("ListAttachedGroupPolicies status=%d body=%q", rec.Code, rec.Body.String())
	}

	inlineDoc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:GetUser","Resource":"*"}]}`)
	rec = post("Action=PutGroupPolicy&Version=2010-05-08&GroupName=Admins&PolicyName=inline1&PolicyDocument=" + inlineDoc)
	if rec.Code != http.StatusOK {
		t.Fatalf("PutGroupPolicy status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=GetGroupPolicy&Version=2010-05-08&GroupName=Admins&PolicyName=inline1")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "GetGroupPolicyResult") {
		t.Fatalf("GetGroupPolicy status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=ListGroupPolicies&Version=2010-05-08&GroupName=Admins")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "inline1") {
		t.Fatalf("ListGroupPolicies status=%d body=%q", rec.Code, rec.Body.String())
	}

	// Permissions boundary
	boundDoc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:GetUser","Resource":"*"}]}`)
	rec = post("Action=CreatePolicy&Version=2010-05-08&PolicyName=UserBoundary&PolicyDocument=" + boundDoc)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreatePolicy boundary status=%d body=%q", rec.Code, rec.Body.String())
	}
	boundARN := xmlTag(t, rec.Body.String(), "Arn")
	rec = post("Action=PutUserPermissionsBoundary&Version=2010-05-08&UserName=alice&PermissionsBoundary=" + url.QueryEscape(boundARN))
	if rec.Code != http.StatusOK {
		t.Fatalf("PutUserPermissionsBoundary status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=GetUserPermissionsBoundary&Version=2010-05-08&UserName=alice")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), boundARN) {
		t.Fatalf("GetUserPermissionsBoundary status=%d body=%q", rec.Code, rec.Body.String())
	}

	// Instance profile + role
	trust := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	rec = post("Action=CreateRole&Version=2010-05-08&RoleName=ec2Role&AssumeRolePolicyDocument=" + trust)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateRole status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=CreateInstanceProfile&Version=2010-05-08&InstanceProfileName=ec2Profile")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ec2Profile") {
		t.Fatalf("CreateInstanceProfile status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=AddRoleToInstanceProfile&Version=2010-05-08&InstanceProfileName=ec2Profile&RoleName=ec2Role")
	if rec.Code != http.StatusOK {
		t.Fatalf("AddRoleToInstanceProfile status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=GetInstanceProfile&Version=2010-05-08&InstanceProfileName=ec2Profile")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ec2Role") {
		t.Fatalf("GetInstanceProfile status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=ListInstanceProfiles&Version=2010-05-08")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ec2Profile") {
		t.Fatalf("ListInstanceProfiles status=%d body=%q", rec.Code, rec.Body.String())
	}

	// Role permissions boundary
	rec = post("Action=PutRolePermissionsBoundary&Version=2010-05-08&RoleName=ec2Role&PermissionsBoundary=" + url.QueryEscape(boundARN))
	if rec.Code != http.StatusOK {
		t.Fatalf("PutRolePermissionsBoundary status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=GetRolePermissionsBoundary&Version=2010-05-08&RoleName=ec2Role")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), boundARN) {
		t.Fatalf("GetRolePermissionsBoundary status=%d body=%q", rec.Code, rec.Body.String())
	}

	// OIDC / SAML IdP
	rec = post("Action=CreateOpenIDConnectProvider&Version=2010-05-08&Url=https://accounts.google.com&ClientIDList.member.1=my-client")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "oidc-provider") {
		t.Fatalf("CreateOpenIDConnectProvider status=%d body=%q", rec.Code, rec.Body.String())
	}
	oidcARN := xmlTag(t, rec.Body.String(), "OpenIDConnectProviderArn")
	rec = post("Action=GetOpenIDConnectProvider&Version=2010-05-08&OpenIDConnectProviderArn=" + url.QueryEscape(oidcARN))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "accounts.google.com") {
		t.Fatalf("GetOpenIDConnectProvider status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=ListOpenIDConnectProviders&Version=2010-05-08")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), oidcARN) {
		t.Fatalf("ListOpenIDConnectProviders status=%d body=%q", rec.Code, rec.Body.String())
	}

	meta := url.QueryEscape("<EntityDescriptor>lab</EntityDescriptor>")
	rec = post("Action=CreateSAMLProvider&Version=2010-05-08&Name=MyIdP&SAMLMetadataDocument=" + meta)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "saml-provider") {
		t.Fatalf("CreateSAMLProvider status=%d body=%q", rec.Code, rec.Body.String())
	}
	samlARN := xmlTag(t, rec.Body.String(), "SAMLProviderArn")
	rec = post("Action=GetSAMLProvider&Version=2010-05-08&SAMLProviderArn=" + url.QueryEscape(samlARN))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "EntityDescriptor") {
		t.Fatalf("GetSAMLProvider status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=ListSAMLProviders&Version=2010-05-08")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), samlARN) {
		t.Fatalf("ListSAMLProviders status=%d body=%q", rec.Code, rec.Body.String())
	}

	// Cleanup detach / delete paths
	rec = post("Action=DetachGroupPolicy&Version=2010-05-08&GroupName=Admins&PolicyArn=" + url.QueryEscape(policyARN))
	if rec.Code != http.StatusOK {
		t.Fatalf("DetachGroupPolicy status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=DeleteGroupPolicy&Version=2010-05-08&GroupName=Admins&PolicyName=inline1")
	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteGroupPolicy status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=RemoveUserFromGroup&Version=2010-05-08&GroupName=Admins&UserName=alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("RemoveUserFromGroup status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=RemoveRoleFromInstanceProfile&Version=2010-05-08&InstanceProfileName=ec2Profile&RoleName=ec2Role")
	if rec.Code != http.StatusOK {
		t.Fatalf("RemoveRoleFromInstanceProfile status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=DeleteInstanceProfile&Version=2010-05-08&InstanceProfileName=ec2Profile")
	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteInstanceProfile status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=DeleteUserPermissionsBoundary&Version=2010-05-08&UserName=alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteUserPermissionsBoundary status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=DeleteRolePermissionsBoundary&Version=2010-05-08&RoleName=ec2Role")
	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteRolePermissionsBoundary status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=DeleteOpenIDConnectProvider&Version=2010-05-08&OpenIDConnectProviderArn=" + url.QueryEscape(oidcARN))
	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteOpenIDConnectProvider status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=DeleteSAMLProvider&Version=2010-05-08&SAMLProviderArn=" + url.QueryEscape(samlARN))
	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteSAMLProvider status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=DeleteGroup&Version=2010-05-08&GroupName=Admins")
	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteGroup status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=ListGroups&Version=2010-05-08")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "Admins") {
		t.Fatalf("ListGroups after delete status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestAddRoleToInstanceProfileErrorMapping(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "iam", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	trust := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	rec := post("Action=CreateRole&Version=2010-05-08&RoleName=mapRole&AssumeRolePolicyDocument=" + trust)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateRole status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=CreateRole&Version=2010-05-08&RoleName=mapRole2&AssumeRolePolicyDocument=" + trust)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateRole2 status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=CreateInstanceProfile&Version=2010-05-08&InstanceProfileName=mapProfile")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateInstanceProfile status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = post("Action=AddRoleToInstanceProfile&Version=2010-05-08&InstanceProfileName=missingProfile&RoleName=mapRole")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "NoSuchEntity") {
		t.Fatalf("missing profile want 404 NoSuchEntity got status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = post("Action=AddRoleToInstanceProfile&Version=2010-05-08&InstanceProfileName=mapProfile&RoleName=missingRole")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "NoSuchEntity") {
		t.Fatalf("missing role want 404 NoSuchEntity got status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = post("Action=AddRoleToInstanceProfile&Version=2010-05-08&InstanceProfileName=mapProfile&RoleName=mapRole")
	if rec.Code != http.StatusOK {
		t.Fatalf("first AddRoleToInstanceProfile status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=AddRoleToInstanceProfile&Version=2010-05-08&InstanceProfileName=mapProfile&RoleName=mapRole2")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "LimitExceeded") {
		t.Fatalf("second role want 400 LimitExceeded got status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestIAMGroupPolicyAuthorizesUser(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	iamPost := func(akid, secret, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), akid, secret, testRegion, "iam", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := iamPost(testAccessKey, testSecret, "Action=CreateUser&Version=2010-05-08&UserName=bob")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateUser: %d %s", rec.Code, rec.Body.String())
	}
	rec = iamPost(testAccessKey, testSecret, "Action=CreateAccessKey&Version=2010-05-08&UserName=bob")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateAccessKey: %d %s", rec.Code, rec.Body.String())
	}
	bobKey := xmlTag(t, rec.Body.String(), "AccessKeyId")
	bobSecret := xmlTag(t, rec.Body.String(), "SecretAccessKey")

	// Bob has no identity policies: ListUsers must be denied.
	rec = iamPost(bobKey, bobSecret, "Action=ListUsers&Version=2010-05-08")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bob ListUsers before group want 403 got %d body=%q", rec.Code, rec.Body.String())
	}

	rec = iamPost(testAccessKey, testSecret, "Action=CreateGroup&Version=2010-05-08&GroupName=Readers")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateGroup: %d %s", rec.Code, rec.Body.String())
	}
	rec = iamPost(testAccessKey, testSecret, "Action=AddUserToGroup&Version=2010-05-08&GroupName=Readers&UserName=bob")
	if rec.Code != http.StatusOK {
		t.Fatalf("AddUserToGroup: %d %s", rec.Code, rec.Body.String())
	}

	doc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:ListUsers","Resource":"*"}]}`)
	rec = iamPost(testAccessKey, testSecret, "Action=CreatePolicy&Version=2010-05-08&PolicyName=ReadersList&PolicyDocument="+doc)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreatePolicy: %d %s", rec.Code, rec.Body.String())
	}
	arn := xmlTag(t, rec.Body.String(), "Arn")
	rec = iamPost(testAccessKey, testSecret, fmt.Sprintf(
		"Action=AttachGroupPolicy&Version=2010-05-08&GroupName=Readers&PolicyArn=%s", url.QueryEscape(arn),
	))
	if rec.Code != http.StatusOK {
		t.Fatalf("AttachGroupPolicy: %d %s", rec.Code, rec.Body.String())
	}

	// Group attached policy flows through IdentityPolicyDocsForUser + EvaluateFull.
	rec = iamPost(bobKey, bobSecret, "Action=ListUsers&Version=2010-05-08")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<ListUsersResponse") {
		t.Fatalf("bob ListUsers after group attach want 200 got %d body=%q", rec.Code, rec.Body.String())
	}
}
