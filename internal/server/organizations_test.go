package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestOrganizationsDepthHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "organizations", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// Create a member account so ListAccounts returns more than management.
	rec := post("Action=CreateAccount&Version=2016-11-28&Email=member%40example.com&AccountName=Member")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateAccount status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = post("Action=ListAccounts&Version=2016-11-28")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<ListAccountsResponse") {
		t.Fatalf("ListAccounts status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<Id>000000000001</Id>") || !strings.Contains(rec.Body.String(), "<Id>000000000002</Id>") {
		t.Fatalf("ListAccounts missing accounts body=%q", rec.Body.String())
	}

	rec = post("Action=CreateOrganizationalUnit&Version=2016-11-28&ParentId=r-root&Name=Workloads")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<CreateOrganizationalUnitResponse") {
		t.Fatalf("CreateOrganizationalUnit status=%d body=%q", rec.Code, rec.Body.String())
	}
	ouID := xmlTag(t, rec.Body.String(), "Id")
	if !strings.HasPrefix(ouID, "ou-") {
		t.Fatalf("ou id=%q", ouID)
	}

	rec = post("Action=ListOrganizationalUnitsForParent&Version=2016-11-28&ParentId=r-root")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), ouID) || !strings.Contains(rec.Body.String(), "Workloads") {
		t.Fatalf("ListOrganizationalUnitsForParent status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = post("Action=EnablePolicyType&Version=2016-11-28&RootId=r-root&PolicyType=SERVICE_CONTROL_POLICY")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "SERVICE_CONTROL_POLICY") {
		t.Fatalf("EnablePolicyType SCP status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=EnablePolicyType&Version=2016-11-28&RootId=r-root&PolicyType=RESOURCE_CONTROL_POLICY")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "RESOURCE_CONTROL_POLICY") {
		t.Fatalf("EnablePolicyType RCP status=%d body=%q", rec.Code, rec.Body.String())
	}

	scpDoc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`)
	rec = post("Action=CreatePolicy&Version=2016-11-28&Name=FullAWS&Type=SERVICE_CONTROL_POLICY&Content=" + scpDoc)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<CreatePolicyResponse") {
		t.Fatalf("CreatePolicy SCP status=%d body=%q", rec.Code, rec.Body.String())
	}
	scpID := xmlTag(t, rec.Body.String(), "Id")
	if !strings.HasPrefix(scpID, "p-") {
		t.Fatalf("scp id=%q", scpID)
	}

	rcpDoc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`)
	rec = post("Action=CreatePolicy&Version=2016-11-28&Name=S3Only&Type=RESOURCE_CONTROL_POLICY&Content=" + rcpDoc)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreatePolicy RCP status=%d body=%q", rec.Code, rec.Body.String())
	}
	rcpID := xmlTag(t, rec.Body.String(), "Id")

	rec = post("Action=DescribePolicy&Version=2016-11-28&PolicyId=" + scpID)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "SERVICE_CONTROL_POLICY") || !strings.Contains(rec.Body.String(), "FullAWS") {
		t.Fatalf("DescribePolicy status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = post("Action=AttachPolicy&Version=2016-11-28&PolicyId=" + scpID + "&TargetId=r-root")
	if rec.Code != http.StatusOK {
		t.Fatalf("AttachPolicy root status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=AttachPolicy&Version=2016-11-28&PolicyId=" + scpID + "&TargetId=000000000002")
	if rec.Code != http.StatusOK {
		t.Fatalf("AttachPolicy account status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=AttachPolicy&Version=2016-11-28&PolicyId=" + rcpID + "&TargetId=000000000002")
	if rec.Code != http.StatusOK {
		t.Fatalf("AttachPolicy RCP status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = post("Action=DetachPolicy&Version=2016-11-28&PolicyId=" + scpID + "&TargetId=000000000002")
	if rec.Code != http.StatusOK {
		t.Fatalf("DetachPolicy status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = post("Action=MoveAccount&Version=2016-11-28&AccountId=000000000002&SourceParentId=r-root&DestinationParentId=" + ouID)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<MoveAccountResponse") {
		t.Fatalf("MoveAccount status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = post("Action=MoveAccount&Version=2016-11-28&AccountId=000000000002&SourceParentId=r-root&DestinationParentId=" + ouID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("MoveAccount bad source want 400 status=%d body=%q", rec.Code, rec.Body.String())
	}

	// IAM CreatePolicy must still work with iam signing (not stolen by orgs dispatcher).
	iamBody := "Action=CreatePolicy&Version=2010-05-08&PolicyName=StillIAM&PolicyDocument=" + scpDoc
	iamReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(iamBody))
	signHeader(t, iamReq, []byte(iamBody), testAccessKey, testSecret, testRegion, "iam", now)
	iamRec := httptest.NewRecorder()
	handler.ServeHTTP(iamRec, iamReq)
	if iamRec.Code != http.StatusOK || !strings.Contains(iamRec.Body.String(), "StillIAM") {
		t.Fatalf("IAM CreatePolicy status=%d body=%q", iamRec.Code, iamRec.Body.String())
	}
}

func TestOrgsCreatePolicyRequiresEnable(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	doc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`)
	body := "Action=CreatePolicy&Version=2016-11-28&Name=TooSoon&Type=SERVICE_CONTROL_POLICY&Content=" + doc
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "organizations", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "PolicyTypeNotEnabledException") {
		t.Fatalf("expected PolicyTypeNotEnabledException status=%d body=%q", rec.Code, rec.Body.String())
	}
}
