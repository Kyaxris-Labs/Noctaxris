package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOrgsMutateRequiresManagementAccount(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, memberID, err := st.CreateMemberAccount(testAccountID, "member@example.com", "Member")
	if err != nil {
		t.Fatal(err)
	}
	const memberAKID = "AKIAMEMBERROOT001"
	const memberSecret = "secret-member-root"
	if err := st.EnsureRoot(memberID, memberAKID, memberSecret); err != nil {
		t.Fatal(err)
	}

	orgsPost := func(akid, secret, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), akid, secret, testRegion, "organizations", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := orgsPost(memberAKID, memberSecret, "Action=CreateOrganizationalUnit&Version=2016-11-28&ParentId=r-root&Name=DeniedOU")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("member CreateOrganizationalUnit want AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "organizations API requires management account") {
		t.Fatalf("member CreateOrganizationalUnit missing management message body=%q", rec.Body.String())
	}

	rec = orgsPost(memberAKID, memberSecret, "Action=ListAccounts&Version=2016-11-28")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("member ListAccounts want AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}

	// AttachPolicy also gated (mutate) even when a policy id is guessed.
	rec = orgsPost(memberAKID, memberSecret, "Action=AttachPolicy&Version=2016-11-28&PolicyId=p-missing&TargetId=r-root")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("member AttachPolicy want AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = orgsPost(testAccessKey, testSecret, "Action=CreateOrganizationalUnit&Version=2016-11-28&ParentId=r-root&Name=Workloads")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<CreateOrganizationalUnitResponse") {
		t.Fatalf("management CreateOrganizationalUnit status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = orgsPost(testAccessKey, testSecret, "Action=ListAccounts&Version=2016-11-28")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), memberID) {
		t.Fatalf("management ListAccounts status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestCreateAccountRequiresManagementAccount(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, memberID, err := st.CreateMemberAccount(testAccountID, "creator@example.com", "Creator")
	if err != nil {
		t.Fatal(err)
	}
	const memberAKID = "AKIAMEMBERROOT002"
	const memberSecret = "secret-member-root-2"
	if err := st.EnsureRoot(memberID, memberAKID, memberSecret); err != nil {
		t.Fatal(err)
	}

	body := "Action=CreateAccount&Version=2016-11-28&Email=nested%40example.com&AccountName=Nested"
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), memberAKID, memberSecret, testRegion, "organizations", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("member CreateAccount want AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}

	req = mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "organizations", now)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "SUCCEEDED") {
		t.Fatalf("management CreateAccount status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestAssumeRootRequiresManagementAccount(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, memberA, err := st.CreateMemberAccount(testAccountID, "a@example.com", "A")
	if err != nil {
		t.Fatal(err)
	}
	_, memberB, err := st.CreateMemberAccount(testAccountID, "b@example.com", "B")
	if err != nil {
		t.Fatal(err)
	}
	const memberAKID = "AKIAMEMBERROOT003"
	const memberSecret = "secret-member-root-3"
	if err := st.EnsureRoot(memberA, memberAKID, memberSecret); err != nil {
		t.Fatal(err)
	}

	assumeBody := "Action=AssumeRoot&Version=2011-06-15&TargetAccount=" + memberB
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(assumeBody))
	signHeader(t, req, []byte(assumeBody), memberAKID, memberSecret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("member AssumeRoot into sibling want AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}

	req = mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(assumeBody))
	signHeader(t, req, []byte(assumeBody), testAccessKey, testSecret, testRegion, "sts", now)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ASIA") {
		t.Fatalf("management AssumeRoot status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestDescribeCreateAccountStatusOwnership(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createID, memberID, err := st.CreateMemberAccount(testAccountID, "owned@example.com", "Owned")
	if err != nil {
		t.Fatal(err)
	}
	const memberAKID = "AKIAMEMBERROOT004"
	const memberSecret = "secret-member-root-4"
	if err := st.EnsureRoot(memberID, memberAKID, memberSecret); err != nil {
		t.Fatal(err)
	}

	_, otherID, err := st.CreateMemberAccount(testAccountID, "other@example.com", "Other")
	if err != nil {
		t.Fatal(err)
	}
	const otherAKID = "AKIAMEMBERROOT005"
	const otherSecret = "secret-member-root-5"
	if err := st.EnsureRoot(otherID, otherAKID, otherSecret); err != nil {
		t.Fatal(err)
	}

	descBody := "Action=DescribeCreateAccountStatus&Version=2016-11-28&CreateAccountRequestId=" + createID
	orgsPost := func(akid, secret string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(descBody))
		signHeader(t, req, []byte(descBody), akid, secret, testRegion, "organizations", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := orgsPost(otherAKID, otherSecret)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusBadRequest {
		t.Fatalf("non-owner DescribeCreateAccountStatus want deny status=%d body=%q", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), memberID) {
		t.Fatalf("non-owner must not learn account id body=%q", rec.Body.String())
	}

	rec = orgsPost(testAccessKey, testSecret)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), memberID) {
		t.Fatalf("management DescribeCreateAccountStatus status=%d body=%q", rec.Code, rec.Body.String())
	}
}
