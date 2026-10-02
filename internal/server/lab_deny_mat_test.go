package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Thin Deny + MissingAuthenticationToken coverage for Transfer / Backup / ASG / EC2
// (audit docs/tests gap: UnauthorizedOperation / MAT on these labs).

func TestEC2RunInstancesUnauthorizedOperationAndMAT(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	unsignedBody := strings.Join([]string{
		"Action=RunInstances",
		"Version=2016-11-15",
		"ImageId=ami-alpine",
		"InstanceType=t3.micro",
		"MinCount=1",
		"MaxCount=1",
	}, "&")
	unsignedReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(unsignedBody))
	unsignedReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	unsignedRec := httptest.NewRecorder()
	handler.ServeHTTP(unsignedRec, unsignedReq)
	if unsignedRec.Code != http.StatusForbidden || !strings.Contains(unsignedRec.Body.String(), "MissingAuthenticationToken") {
		t.Fatalf("unsigned RunInstances want 403 MissingAuthenticationToken, got %d %s", unsignedRec.Code, unsignedRec.Body.String())
	}

	_, _, err := st.CreateUser(testAccountID, "ec2-deny")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "ec2-deny")
	if err != nil {
		t.Fatal(err)
	}
	body := unsignedBody
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, req, []byte(body), akid, secret, testRegion, "ec2", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "UnauthorizedOperation") {
		t.Fatalf("noperm RunInstances want 403 UnauthorizedOperation, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestASGCreateLaunchConfigurationAccessDeniedAndMAT(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	unsignedBody := strings.Join([]string{
		"Action=CreateLaunchConfiguration",
		"Version=2011-01-01",
		"LaunchConfigurationName=asg-mat-lc",
		"ImageId=ami-alpine",
		"InstanceType=t3.micro",
	}, "&")
	unsignedReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(unsignedBody))
	unsignedRec := httptest.NewRecorder()
	handler.ServeHTTP(unsignedRec, unsignedReq)
	if unsignedRec.Code != http.StatusForbidden || !strings.Contains(unsignedRec.Body.String(), "MissingAuthenticationToken") {
		t.Fatalf("unsigned CreateLaunchConfiguration want 403 MissingAuthenticationToken, got %d %s", unsignedRec.Code, unsignedRec.Body.String())
	}

	_, _, err := st.CreateUser(testAccountID, "asg-deny")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "asg-deny")
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(unsignedBody))
	signHeader(t, req, []byte(unsignedBody), akid, secret, testRegion, "autoscaling", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("noperm CreateLaunchConfiguration want 403 AccessDenied, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestBackupCreateVaultAccessDeniedAndMAT(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	unsignedReq := mustNewRequest(t, http.MethodPut, "http://127.0.0.1:4566/backup-vaults/mat-vault", []byte("{}"))
	unsignedReq.Header.Set("Content-Type", "application/json")
	unsignedRec := httptest.NewRecorder()
	handler.ServeHTTP(unsignedRec, unsignedReq)
	if unsignedRec.Code != http.StatusForbidden || !strings.Contains(unsignedRec.Body.String(), "MissingAuthenticationToken") {
		t.Fatalf("unsigned CreateBackupVault want 403 MissingAuthenticationToken, got %d %s", unsignedRec.Code, unsignedRec.Body.String())
	}

	_, _, err := st.CreateUser(testAccountID, "backup-deny")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "backup-deny")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("{}")
	req := mustNewRequest(t, http.MethodPut, "http://127.0.0.1:4566/backup-vaults/deny-vault", body)
	req.Header.Set("Content-Type", "application/json")
	signHeader(t, req, body, akid, secret, testRegion, "backup", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDeniedException") {
		t.Fatalf("noperm CreateBackupVault want 403 AccessDeniedException, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestTransferCreateServerAccessDeniedAndMAT(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	raw, err := json.Marshal(map[string]any{"Protocols": []string{"SFTP"}})
	if err != nil {
		t.Fatal(err)
	}
	unsignedReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	unsignedReq.Header.Set("Content-Type", "application/x-amz-json-1.1")
	unsignedReq.Header.Set("X-Amz-Target", "TransferService.CreateServer")
	unsignedRec := httptest.NewRecorder()
	handler.ServeHTTP(unsignedRec, unsignedReq)
	if unsignedRec.Code != http.StatusForbidden || !strings.Contains(unsignedRec.Body.String(), "MissingAuthenticationToken") {
		t.Fatalf("unsigned CreateServer want 403 MissingAuthenticationToken, got %d %s", unsignedRec.Code, unsignedRec.Body.String())
	}

	_, _, err = st.CreateUser(testAccountID, "transfer-deny")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "transfer-deny")
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "TransferService.CreateServer")
	signHeader(t, req, raw, akid, secret, testRegion, "transfer", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDeniedException") {
		t.Fatalf("noperm CreateServer want 403 AccessDeniedException, got %d %s", rec.Code, rec.Body.String())
	}
}
