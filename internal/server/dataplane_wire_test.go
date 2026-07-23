package server

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestPromoteNestedDataAfterWaitFailure(t *testing.T) {
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

	srv := &Server{store: st}
	account := "000000000001"
	if _, err := st.CreateElastiCacheCluster(account, "us-east-1", "wait-fail-cache", "redis", "", "", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateDocDBCluster(account, "us-east-1", "wait-fail-docdb", "docdb", "", "labadmin", 0); err != nil {
		t.Fatal(err)
	}

	waitErr := errors.New("compute: data-plane healthy wait: timeout")
	if err := promoteNestedDataAfterWait(srv, account, compute.DataKindElastiCache, "wait-fail-cache", "ctr-ec", "host-ec", waitErr); err == nil {
		t.Fatal("expected wait error returned")
	}
	ec, err := st.DescribeElastiCacheCluster(account, "wait-fail-cache")
	if err != nil || ec.Status != "failed" {
		t.Fatalf("elasticache after wait fail: %+v err=%v", ec, err)
	}

	if err := promoteNestedDataAfterWait(srv, account, compute.DataKindDocDB, "wait-fail-docdb", "ctr-dd", "host-dd", waitErr); err == nil {
		t.Fatal("expected wait error returned")
	}
	dd, err := st.DescribeDocDBCluster(account, "wait-fail-docdb")
	if err != nil || dd.Status != "failed" {
		t.Fatalf("docdb after wait fail: %+v err=%v", dd, err)
	}

	if err := promoteNestedDataAfterWait(srv, account, compute.DataKindElastiCache, "wait-fail-cache", "ctr-ec2", "host-ec2", nil); err != nil {
		t.Fatal(err)
	}
	ecReady, err := st.DescribeElastiCacheCluster(account, "wait-fail-cache")
	if err != nil || ecReady.Status != "available" || ecReady.ContainerID != "ctr-ec2" {
		t.Fatalf("elasticache after wait ok: %+v err=%v", ecReady, err)
	}
}
