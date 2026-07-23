package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestDynamoDBDescribeContinuousBackups(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "pitr-lab",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	descRec := mustDynamoJSON(t, handler, "DescribeContinuousBackups", map[string]any{
		"TableName": "pitr-lab",
	}, now)
	if descRec.Code != http.StatusOK {
		t.Fatalf("DescribeContinuousBackups status=%d body=%q", descRec.Code, descRec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(descRec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	desc, _ := out["ContinuousBackupsDescription"].(map[string]any)
	if desc["ContinuousBackupsStatus"] != "ENABLED" {
		t.Fatalf("ContinuousBackupsStatus=%v", desc["ContinuousBackupsStatus"])
	}
	pitr, _ := desc["PointInTimeRecoveryDescription"].(map[string]any)
	if pitr["PointInTimeRecoveryStatus"] != "DISABLED" {
		t.Fatalf("PointInTimeRecoveryStatus=%v", pitr["PointInTimeRecoveryStatus"])
	}

	missing := mustDynamoJSON(t, handler, "DescribeContinuousBackups", map[string]any{
		"TableName": "no-such-table",
	}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("expected missing table to fail, body=%q", missing.Body.String())
	}
}
