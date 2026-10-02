package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAuthorizeAthenaDuckS3ReadsRequiresGetObject(t *testing.T) {
	srv, st := newAthenaDuckAuthzTestServer(t)
	account := testAccountID
	seedAthenaDuckAuthzTable(t, st, account, "duck-authz", "labdb", "people", "s3://duck-authz/data/", "data/t1.csv")

	userName := "athena-no-s3"
	if _, _, err := st.CreateUser(account, userName); err != nil {
		t.Fatal(err)
	}
	akid, _, err := st.CreateUserAccessKey(account, userName)
	if err != nil {
		t.Fatal(err)
	}
	athenaOnly := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"athena:StartQueryExecution","Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(account, "AthenaStartOnly", athenaOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(account, userName, polARN); err != nil {
		t.Fatal(err)
	}
	v := &authn.Verified{
		AccountID:   account,
		AccessKeyID: akid,
		Region:      "us-east-1",
		Service:     "athena",
		Principal:   identity.UserPrincipal(account, userName, akid),
	}
	in := store.AthenaStartInput{
		QueryString: "SELECT * FROM labdb.people",
		Database:    "labdb",
	}
	err = srv.authorizeAthenaDuckS3Reads(v, account, in)
	if err == nil || !errors.Is(err, store.ErrAthenaAccessDenied) {
		t.Fatalf("deny without s3:GetObject: err=%v", err)
	}

	getAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
	getARN, err := st.CreateManagedPolicy(account, "S3GetObject", getAllow)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(account, userName, getARN); err != nil {
		t.Fatal(err)
	}
	if err := srv.authorizeAthenaDuckS3Reads(v, account, in); err != nil {
		t.Fatalf("allow with s3:GetObject: %v", err)
	}
}

func TestAuthorizeAthenaDuckS3ReadsResolvesDatabaseFromQuery(t *testing.T) {
	srv, st := newAthenaDuckAuthzTestServer(t)
	account := testAccountID
	seedAthenaDuckAuthzTable(t, st, account, "duck-authz-q", "qdb", "rows", "s3://duck-authz-q/p/", "p/a.csv")

	userName := "athena-query-db"
	if _, _, err := st.CreateUser(account, userName); err != nil {
		t.Fatal(err)
	}
	akid, _, err := st.CreateUserAccessKey(account, userName)
	if err != nil {
		t.Fatal(err)
	}
	athenaOnly := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"athena:StartQueryExecution","Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(account, "AthenaStartOnlyQ", athenaOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(account, userName, polARN); err != nil {
		t.Fatal(err)
	}
	v := &authn.Verified{
		AccountID:   account,
		AccessKeyID: akid,
		Region:      "us-east-1",
		Service:     "athena",
		Principal:   identity.UserPrincipal(account, userName, akid),
	}
	// Empty QueryExecutionContext.Database; db comes from SELECT db.table.
	in := store.AthenaStartInput{QueryString: "SELECT * FROM qdb.rows"}
	err = srv.authorizeAthenaDuckS3Reads(v, account, in)
	if err == nil || !errors.Is(err, store.ErrAthenaAccessDenied) {
		t.Fatalf("must authorize scanned keys when Database empty: err=%v", err)
	}
}

func TestTryStartAthenaDuckDeniesWithoutGetObject(t *testing.T) {
	duck := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","rows":[{"id":"1"}]}`))
	}))
	t.Cleanup(duck.Close)

	t.Setenv(compute.EnvAthenaEngine, "duckdb")
	t.Setenv(compute.EnvDuckDBURL, duck.URL)

	srv, st := newAthenaDuckAuthzTestServer(t)
	account := testAccountID
	seedAthenaDuckAuthzTable(t, st, account, "duck-try", "tdb", "t1", "s3://duck-try/data/", "data/x.csv")

	userName := "duck-start-only"
	if _, _, err := st.CreateUser(account, userName); err != nil {
		t.Fatal(err)
	}
	akid, _, err := st.CreateUserAccessKey(account, userName)
	if err != nil {
		t.Fatal(err)
	}
	athenaOnly := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"athena:StartQueryExecution","Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(account, "AthenaStartOnlyTry", athenaOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(account, userName, polARN); err != nil {
		t.Fatal(err)
	}
	v := &authn.Verified{
		AccountID:   account,
		AccessKeyID: akid,
		Region:      "us-east-1",
		Service:     "athena",
		Principal:   identity.UserPrincipal(account, userName, akid),
	}
	_, used, err := srv.tryStartAthenaDuck(v, account, store.AthenaStartInput{
		QueryString: "SELECT * FROM tdb.t1",
		Database:    "tdb",
	})
	if !used {
		t.Fatal("expected Duck path used")
	}
	if err == nil || !errors.Is(err, store.ErrAthenaAccessDenied) {
		t.Fatalf("tryStart without GetObject: err=%v", err)
	}
	_ = catalog.ActionS3GetObject
}

func newAthenaDuckAuthzTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if err := st.EnsureRoot(testAccountID, testAccessKey, testSecret); err != nil {
		t.Fatal(err)
	}
	aud, err := audit.NewWriter(filepath.Join(dir, "cloudtrail"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = aud.Close() })
	cfg := config.Config{
		ListenAddr: "127.0.0.1:0",
		DataRoot:   dir,
		AccountID:  testAccountID,
	}
	return New(cfg, st, aud), st
}

func seedAthenaDuckAuthzTable(t *testing.T, st *store.Store, account, bucket, db, table, location, key string) {
	t.Helper()
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	csv := "id,name\n1,alice\n"
	if _, err := st.PutObject(account, bucket, key, store.PutObjectMeta{
		Data: []byte(csv), PlainSize: int64(len(csv)), ContentType: "text/csv",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueDatabase(account, db, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName:    db,
		Name:            table,
		StorageLocation: location,
		Columns: []store.GlueColumn{
			{Name: "id", Type: "string"},
			{Name: "name", Type: "string"},
		},
	}); err != nil {
		t.Fatal(err)
	}
}
