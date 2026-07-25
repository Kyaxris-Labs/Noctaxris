package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGlueStartCrawlerCreatesTableFromCSV(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateBucket(testAccountID, "glue-crawl"); err != nil {
		t.Fatal(err)
	}
	csv := "id,name\n1,alice\n"
	if _, err := st.PutObject(testAccountID, "glue-crawl", "data/people.csv", store.PutObjectMeta{
		Data: []byte(csv), PlainSize: int64(len(csv)),
	}); err != nil {
		t.Fatal(err)
	}

	createDB := mustJSONTarget(t, handler, "AWSGlue.CreateDatabase", "glue", map[string]any{
		"DatabaseInput": map[string]any{"Name": "labdb"},
	}, now)
	if createDB.Code != http.StatusOK {
		t.Fatalf("CreateDatabase status=%d body=%q", createDB.Code, createDB.Body.String())
	}

	createCr := mustJSONTarget(t, handler, "AWSGlue.CreateCrawler", "glue", map[string]any{
		"Name":         "lab-crawler",
		"DatabaseName": "labdb",
		"Targets": map[string]any{
			"S3Targets": []map[string]any{{"Path": "s3://glue-crawl/data/"}},
		},
	}, now)
	if createCr.Code != http.StatusOK {
		t.Fatalf("CreateCrawler status=%d body=%q", createCr.Code, createCr.Body.String())
	}

	start := mustJSONTarget(t, handler, "AWSGlue.StartCrawler", "glue", map[string]any{
		"Name": "lab-crawler",
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartCrawler status=%d body=%q", start.Code, start.Body.String())
	}

	get := mustJSONTarget(t, handler, "AWSGlue.GetCrawler", "glue", map[string]any{
		"Name": "lab-crawler",
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetCrawler status=%d body=%q", get.Code, get.Body.String())
	}
	var parsed map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	cr, _ := parsed["Crawler"].(map[string]any)
	if cr["State"] != "READY" {
		t.Fatalf("crawler state=%v", cr["State"])
	}

	tbl, err := st.GetGlueTable(testAccountID, "labdb", "people")
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Columns) != 2 || tbl.Columns[0].Name != "id" {
		t.Fatalf("columns=%#v", tbl.Columns)
	}
}
