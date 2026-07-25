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
