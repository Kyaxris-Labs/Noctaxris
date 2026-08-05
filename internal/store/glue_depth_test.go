package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGlueCoverageWave2DatabaseTableCrawlerJSON(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureGlueSchema(); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureGlueSchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}

	empty, err := st.GetGlueDatabases(account)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty dbs=%v err=%v", empty, err)
	}
	if err := st.DeleteGlueDatabase(account, "missing"); !errors.Is(err, store.ErrGlueNotFound) {
		t.Fatalf("delete missing db: %v", err)
	}
	if err := st.DeleteGlueTable(account, "db", "t"); !errors.Is(err, store.ErrGlueNotFound) {
		t.Fatalf("delete missing table: %v", err)
	}
	if err := st.DeleteGlueCrawler(account, "missing"); !errors.Is(err, store.ErrGlueNotFound) {
		t.Fatalf("delete missing crawler: %v", err)
	}

	db, err := st.CreateGlueDatabase(account, "wave2db", "desc")
	if err != nil || db.Name != "wave2db" {
		t.Fatalf("create db=%+v err=%v", db, err)
	}
	listed, err := st.GetGlueDatabases(account)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list dbs=%v err=%v", listed, err)
	}

	if _, err := st.CreateBucket(account, "glue-wave2"); err != nil {
		t.Fatal(err)
	}
	jsonBody := []byte("\n\n{\"id\":\"1\",\"name\":\"alice\"}\n{\"id\":\"2\"}\n")
	if _, err := st.PutObject(account, "glue-wave2", "orders/items.json", store.PutObjectMeta{
		Data: jsonBody, PlainSize: int64(len(jsonBody)), ContentType: "application/json",
	}); err != nil {
		t.Fatal(err)
	}
	// Pre-create table so crawl hits updateGlueTableFromCrawl path.
	if _, err := st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName: "wave2db", Name: "items",
		Columns:         []store.GlueColumn{{Name: "old", Type: "string"}},
		StorageLocation: "s3://glue-wave2/orders/",
	}); err != nil {
		t.Fatal(err)
	}

	emptyCrawl, err := st.ListGlueCrawlers(account)
	if err != nil || len(emptyCrawl) != 0 {
		t.Fatalf("empty crawlers=%v err=%v", emptyCrawl, err)
	}
	cr, err := st.CreateGlueCrawler(account, store.GlueCrawlerCreate{
		Name: "wave2-crawl", DatabaseName: "wave2db",
		Targets: []store.GlueS3Target{{Path: "s3://glue-wave2/orders/"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	crawlers, err := st.ListGlueCrawlers(account)
	if err != nil || len(crawlers) != 1 || crawlers[0].Name != cr.Name {
		t.Fatalf("crawlers=%v err=%v", crawlers, err)
	}
	if _, err := st.StartGlueCrawler(account, "wave2-crawl"); err != nil {
		t.Fatal(err)
	}
	tbl, err := st.GetGlueTable(account, "wave2db", "items")
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Columns) < 2 {
		t.Fatalf("expected json-inferred columns, got %+v", tbl.Columns)
	}

	if err := st.DeleteGlueCrawler(account, "wave2-crawl"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteGlueCrawler(account, "wave2-crawl"); !errors.Is(err, store.ErrGlueNotFound) {
		t.Fatalf("second delete crawler: %v", err)
	}
	if err := st.DeleteGlueTable(account, "wave2db", "items"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteGlueDatabase(account, "wave2db"); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetGlueDatabases(account)
	if err != nil || len(after) != 0 {
		t.Fatalf("after delete dbs=%v err=%v", after, err)
	}
}
