package store_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCreateCloudFrontDistributionDeployedWithDomain(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	account := "000000000001"
	if _, err := st.CreateBucket(account, "cf-origin-bucket"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	d, err := st.CreateCloudFrontDistribution(account, "lab", "caller-1", true, []store.CloudFrontOrigin{{
		ID: "o1", DomainName: "cf-origin-bucket", OriginType: "s3",
	}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.Status != store.CloudFrontStatusDeployed {
		t.Fatalf("status=%q want Deployed", d.Status)
	}
	if d.DomainName == "" || !strings.Contains(d.DomainName, "cloudfront.noctaxris.local") {
		t.Fatalf("DomainName=%q", d.DomainName)
	}
	wantPrefix := "d" + strings.ToLower(d.ID) + "."
	if !strings.HasPrefix(d.DomainName, wantPrefix) {
		t.Fatalf("DomainName=%q want prefix %q", d.DomainName, wantPrefix)
	}
	if d.ETag == "" {
		t.Fatal("expected ETag on create")
	}
}

func TestSelectCloudFrontOriginPathPattern(t *testing.T) {
	origins := []store.CloudFrontOrigin{
		{ID: "default", DomainName: "a", OriginType: "s3"},
		{ID: "api", DomainName: "b", OriginType: "s3"},
	}
	behaviors := []store.CloudFrontCacheBehavior{
		{PathPattern: "/api/*", TargetOriginId: "api"},
		{PathPattern: "*", TargetOriginId: "default"},
	}
	got, err := store.SelectCloudFrontOrigin(origins, behaviors, "api/v1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "api" {
		t.Fatalf("got origin %q want api", got.ID)
	}
	got, err = store.SelectCloudFrontOrigin(origins, behaviors, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "default" {
		t.Fatalf("got origin %q want default", got.ID)
	}
	if !store.MatchCloudFrontPathPattern("/images/x", "/images/*") {
		t.Fatal("expected /images/* to match /images/x")
	}
	if store.MatchCloudFrontPathPattern("/img/x", "/images/*") {
		t.Fatal("expected /images/* not to match /img/x")
	}
}

func TestCloudFrontUpdateDistributionRequiresIfMatch(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	account := "000000000001"
	if _, err := st.CreateBucket(account, "cf-upd-bucket"); err != nil {
		t.Fatal(err)
	}
	d, err := st.CreateCloudFrontDistributionWithBehaviors(account, "lab", "upd-caller", true,
		[]store.CloudFrontOrigin{{ID: "o1", DomainName: "cf-upd-bucket", OriginType: "s3"}},
		[]store.CloudFrontCacheBehavior{{PathPattern: "*", TargetOriginId: "o1"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	enabled := false
	_, err = st.UpdateCloudFrontDistribution(account, d.ID, store.UpdateCloudFrontDistributionInput{
		IfMatch: "wrong-etag",
		Enabled: &enabled,
	})
	if !errors.Is(err, store.ErrCloudFrontPrecondition) {
		t.Fatalf("want ErrCloudFrontPrecondition, got %v", err)
	}
	_, err = st.UpdateCloudFrontDistribution(account, d.ID, store.UpdateCloudFrontDistributionInput{
		Enabled: &enabled,
	})
	if !errors.Is(err, store.ErrCloudFrontPrecondition) {
		t.Fatalf("want ErrCloudFrontPrecondition for missing IfMatch, got %v", err)
	}

	if _, err := st.CreateBucket(account, "cf-upd-bucket-2"); err != nil {
		t.Fatal(err)
	}
	updated, err := st.UpdateCloudFrontDistribution(account, d.ID, store.UpdateCloudFrontDistributionInput{
		IfMatch:    d.ETag,
		Enabled:    &enabled,
		Comment:    ptr("updated"),
		HasOrigins: true,
		Origins: []store.CloudFrontOrigin{
			{ID: "o2", DomainName: "cf-upd-bucket-2", OriginType: "s3"},
		},
		HasBehaviors: true,
		Behaviors: []store.CloudFrontCacheBehavior{
			{PathPattern: "/static/*", TargetOriginId: "o2"},
			{PathPattern: "*", TargetOriginId: "o2"},
		},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Enabled {
		t.Fatal("expected Enabled=false")
	}
	if updated.Comment != "updated" {
		t.Fatalf("comment=%q", updated.Comment)
	}
	if updated.ETag == "" || updated.ETag == d.ETag {
		t.Fatalf("etag should change: old=%q new=%q", d.ETag, updated.ETag)
	}
	var origins []store.CloudFrontOrigin
	if err := json.Unmarshal([]byte(updated.OriginsJSON), &origins); err != nil || len(origins) != 1 || origins[0].ID != "o2" {
		t.Fatalf("origins=%s err=%v", updated.OriginsJSON, err)
	}
	var behaviors []store.CloudFrontCacheBehavior
	if err := json.Unmarshal([]byte(updated.BehaviorsJSON), &behaviors); err != nil || len(behaviors) != 2 {
		t.Fatalf("behaviors=%s err=%v", updated.BehaviorsJSON, err)
	}
}

func TestCloudFrontInvalidationTheatre(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	account := "000000000001"
	if _, err := st.CreateBucket(account, "cf-inv-bucket"); err != nil {
		t.Fatal(err)
	}
	d, err := st.CreateCloudFrontDistribution(account, "lab", "inv-caller-dist", true, []store.CloudFrontOrigin{{
		ID: "o1", DomainName: "cf-inv-bucket", OriginType: "s3",
	}})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := st.CreateCloudFrontInvalidation(account, d.ID, "inv-ref-1", []string{"/index.html", "/images/*"})
	if err != nil {
		t.Fatalf("CreateInvalidation: %v", err)
	}
	if inv.Status != store.CloudFrontInvalidationStatusCompleted {
		t.Fatalf("status=%q want Completed", inv.Status)
	}
	if !strings.HasPrefix(inv.ID, "I") {
		t.Fatalf("id=%q", inv.ID)
	}
	got, err := st.GetCloudFrontInvalidation(account, d.ID, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != inv.ID || got.CallerReference != "inv-ref-1" {
		t.Fatalf("got=%+v", got)
	}
	var paths []string
	if err := json.Unmarshal([]byte(got.PathsJSON), &paths); err != nil || len(paths) != 2 {
		t.Fatalf("paths=%s err=%v", got.PathsJSON, err)
	}
	list, err := st.ListCloudFrontInvalidations(account, d.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	same, err := st.CreateCloudFrontInvalidation(account, d.ID, "inv-ref-1", []string{"/index.html", "/images/*"})
	if err != nil || same.ID != inv.ID {
		t.Fatalf("idempotent create: %+v err=%v", same, err)
	}
	_, err = st.CreateCloudFrontInvalidation(account, d.ID, "inv-ref-1", []string{"/other"})
	if !errors.Is(err, store.ErrCloudFrontBadRequest) {
		t.Fatalf("want bad request for conflicting caller, got %v", err)
	}
	if err := st.DeleteCloudFrontDistribution(account, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetCloudFrontInvalidation(account, d.ID, inv.ID); !errors.Is(err, store.ErrCloudFrontNotFound) {
		t.Fatalf("want NoSuchDistribution after delete, got %v", err)
	}
}

func ptr[T any](v T) *T { return &v }
