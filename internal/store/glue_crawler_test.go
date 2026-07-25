package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGlueStartCrawlerCreatesTableFromCSV(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "crawl-lab"); err != nil {
		t.Fatal(err)
	}
	csv := "id,name\n1,alice\n2,bob\n"
	if _, err := st.PutObject(account, "crawl-lab", "data/people.csv", store.PutObjectMeta{
		Data: []byte(csv), PlainSize: int64(len(csv)), ContentType: "text/csv",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueDatabase(account, "labdb", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueCrawler(account, store.GlueCrawlerCreate{
		Name:         "csv-crawler",
		DatabaseName: "labdb",
		Targets:      []store.GlueS3Target{{Path: "s3://crawl-lab/data/"}},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.StartGlueCrawler(account, "csv-crawler")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "READY" {
		t.Fatalf("state=%q want READY", got.State)
	}
	tbl, err := st.GetGlueTable(account, "labdb", "people")
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Columns) != 2 || tbl.Columns[0].Name != "id" || tbl.Columns[1].Name != "name" {
		t.Fatalf("columns=%#v", tbl.Columns)
	}
	if tbl.StorageLocation != "s3://crawl-lab/data/" {
		t.Fatalf("location=%q", tbl.StorageLocation)
	}
}

func TestGlueStartCrawlerFailsWhenBucketMissing(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateGlueDatabase(account, "labdb", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueCrawler(account, store.GlueCrawlerCreate{
		Name:         "bad-crawler",
		DatabaseName: "labdb",
		Targets:      []store.GlueS3Target{{Path: "s3://no-such-bucket/prefix/"}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := st.StartGlueCrawler(account, "bad-crawler")
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
	if !errors.Is(err, store.ErrGlueBadRequest) {
		t.Fatalf("err=%v want ErrGlueBadRequest", err)
	}
	cr, err := st.GetGlueCrawler(account, "bad-crawler")
	if err != nil {
		t.Fatal(err)
	}
	if cr.State != "READY" {
		t.Fatalf("state=%q want READY after failed start", cr.State)
	}
}

func TestGlueCrawlerRejectsInvalidTarget(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateGlueDatabase(account, "labdb", ""); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateGlueCrawler(account, store.GlueCrawlerCreate{
		Name:         "bad-path",
		DatabaseName: "labdb",
		Targets:      []store.GlueS3Target{{Path: "not-an-s3-uri"}},
	})
	if err == nil {
		t.Fatal("expected invalid target rejection")
	}
	if !errors.Is(err, store.ErrGlueBadRequest) {
		t.Fatalf("err=%v want ErrGlueBadRequest", err)
	}
}
