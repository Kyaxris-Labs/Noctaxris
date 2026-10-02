package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGlueStartCrawlerRequiresRoleS3Permissions(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "glue-role-authz"); err != nil {
		t.Fatal(err)
	}
	csv := "id,name\n1,alice\n"
	if _, err := st.PutObject(account, "glue-role-authz", "data/people.csv", store.PutObjectMeta{
		Data: []byte(csv), PlainSize: int64(len(csv)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueDatabase(account, "roleauthzdb", ""); err != nil {
		t.Fatal(err)
	}

	roleARN, err := st.CreateRole(account, "glue-no-s3", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"glue.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueCrawler(account, store.GlueCrawlerCreate{
		Name: "role-deny-crawler", Role: roleARN, DatabaseName: "roleauthzdb",
		Targets: []store.GlueS3Target{{Path: "s3://glue-role-authz/data/"}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err = st.StartGlueCrawler(account, "role-deny-crawler")
	if err == nil || !errors.Is(err, store.ErrGlueBadRequest) || !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("StartGlueCrawler without S3 on Role err=%v", err)
	}

	if err := st.PutInlinePolicy(roleARN, "s3", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket","s3:GetObject"],"Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.StartGlueCrawler(account, "role-deny-crawler"); err != nil {
		t.Fatalf("StartGlueCrawler with S3 Role: %v", err)
	}
}

func TestGlueStartCrawlerRequiresRole(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "glue-norole"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueDatabase(account, "noroledb", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueCrawler(account, store.GlueCrawlerCreate{
		Name: "no-role-crawler", DatabaseName: "noroledb",
		Targets: []store.GlueS3Target{{Path: "s3://glue-norole/"}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := st.StartGlueCrawler(account, "no-role-crawler")
	if err == nil || !errors.Is(err, store.ErrGlueBadRequest) || !strings.Contains(err.Error(), "Role is required") {
		t.Fatalf("StartGlueCrawler without Role err=%v", err)
	}
}
