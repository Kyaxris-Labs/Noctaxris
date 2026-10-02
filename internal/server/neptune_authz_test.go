package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNeptuneAuthorizeUsesNeptuneActions(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	userName := "neptune-rds-only"
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	// rds:* must not authorize Neptune control plane (catalog ActionNeptune*).
	rdsOnly := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"rds:*","Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(testAccountID, "RDSOnlyNeptune", rdsOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, polARN); err != nil {
		t.Fatal(err)
	}

	body := strings.Join([]string{
		"Action=CreateDBCluster",
		"Version=2014-10-31",
		"DBClusterIdentifier=neptune-authz-1",
		"Engine=neptune",
	}, "&")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), akid, secret, testRegion, "neptune", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden {
		t.Fatalf("rds:* CreateDBCluster status=%d want 403 body=%q", denyRec.Code, denyRec.Body.String())
	}
	if !strings.Contains(denyRec.Body.String(), "neptune:CreateDBCluster") {
		t.Fatalf("deny message should cite neptune action body=%q", denyRec.Body.String())
	}

	neptuneAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"neptune:*","Resource":"*"}]}`
	okARN, err := st.CreateManagedPolicy(testAccountID, "NeptuneAll", neptuneAllow)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, okARN); err != nil {
		t.Fatal(err)
	}
	reqOK := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, reqOK, []byte(body), akid, secret, testRegion, "neptune", now)
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, reqOK)
	if okRec.Code != http.StatusOK {
		t.Fatalf("neptune:* CreateDBCluster status=%d body=%q", okRec.Code, okRec.Body.String())
	}
}
