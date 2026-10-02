package server_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGlueStartCrawlerRequiresRoleS3Permissions(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateBucket(testAccountID, "glue-role-authz"); err != nil {
		t.Fatal(err)
	}
	csv := "id,name\n1,alice\n"
	if _, err := st.PutObject(testAccountID, "glue-role-authz", "data/people.csv", store.PutObjectMeta{
		Data: []byte(csv), PlainSize: int64(len(csv)),
	}); err != nil {
		t.Fatal(err)
	}
	createDB := mustJSONTarget(t, handler, "AWSGlue.CreateDatabase", "glue", map[string]any{
		"DatabaseInput": map[string]any{"Name": "roleauthzdb"},
	}, now)
	if createDB.Code != http.StatusOK {
		t.Fatalf("CreateDatabase status=%d body=%q", createDB.Code, createDB.Body.String())
	}

	// Role trusts Glue but has no S3 permissions.
	roleARN, err := st.CreateRole(testAccountID, "glue-no-s3", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"glue.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	createCr := mustJSONTarget(t, handler, "AWSGlue.CreateCrawler", "glue", map[string]any{
		"Name":         "role-deny-crawler",
		"Role":         roleARN,
		"DatabaseName": "roleauthzdb",
		"Targets": map[string]any{
			"S3Targets": []map[string]any{{"Path": "s3://glue-role-authz/data/"}},
		},
	}, now)
	if createCr.Code != http.StatusOK {
		t.Fatalf("CreateCrawler status=%d body=%q", createCr.Code, createCr.Body.String())
	}
	denyStart := mustJSONTarget(t, handler, "AWSGlue.StartCrawler", "glue", map[string]any{
		"Name": "role-deny-crawler",
	}, now)
	if denyStart.Code != http.StatusForbidden {
		t.Fatalf("StartCrawler without S3 on Role status=%d want 403 body=%q", denyStart.Code, denyStart.Body.String())
	}

	if err := st.PutInlinePolicy(roleARN, "s3", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket","s3:GetObject"],"Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	okStart := mustJSONTarget(t, handler, "AWSGlue.StartCrawler", "glue", map[string]any{
		"Name": "role-deny-crawler",
	}, now)
	if okStart.Code != http.StatusOK {
		t.Fatalf("StartCrawler with S3 Role status=%d body=%q", okStart.Code, okStart.Body.String())
	}
}

func TestGlueStartCrawlerRequiresRole(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateBucket(testAccountID, "glue-norole"); err != nil {
		t.Fatal(err)
	}
	createDB := mustJSONTarget(t, handler, "AWSGlue.CreateDatabase", "glue", map[string]any{
		"DatabaseInput": map[string]any{"Name": "noroledb"},
	}, now)
	if createDB.Code != http.StatusOK {
		t.Fatalf("CreateDatabase status=%d body=%q", createDB.Code, createDB.Body.String())
	}
	createCr := mustJSONTarget(t, handler, "AWSGlue.CreateCrawler", "glue", map[string]any{
		"Name":         "no-role-crawler",
		"DatabaseName": "noroledb",
		"Targets": map[string]any{
			"S3Targets": []map[string]any{{"Path": "s3://glue-norole/"}},
		},
	}, now)
	if createCr.Code != http.StatusOK {
		t.Fatalf("CreateCrawler status=%d body=%q", createCr.Code, createCr.Body.String())
	}
	start := mustJSONTarget(t, handler, "AWSGlue.StartCrawler", "glue", map[string]any{
		"Name": "no-role-crawler",
	}, now)
	if start.Code != http.StatusForbidden {
		t.Fatalf("StartCrawler without Role status=%d want 403 body=%q", start.Code, start.Body.String())
	}
}
