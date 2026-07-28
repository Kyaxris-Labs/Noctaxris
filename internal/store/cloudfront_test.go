package store_test

import (
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
