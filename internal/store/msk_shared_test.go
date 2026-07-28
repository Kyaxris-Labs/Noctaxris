package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMSKSharedCreateBootstrap(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	c, err := st.CreateMSKCluster(account, "us-east-1", "shared-a", "3.6.0", 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if c.BootstrapBrokers != store.MSKSharedBootstrap {
		t.Fatalf("bootstrap=%q want %q", c.BootstrapBrokers, store.MSKSharedBootstrap)
	}
}

func TestMSKSharedSecondClusterLimit(t *testing.T) {
	st := openTestStore(t)
	accountA := "000000000001"
	accountB := "000000000002"
	if _, err := st.CreateMSKCluster(accountA, "us-east-1", "shared-a", "3.6.0", 1, true); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateMSKCluster(accountB, "us-east-1", "shared-b", "3.6.0", 1, true)
	if !errors.Is(err, store.ErrMSKLimitExceeded) {
		t.Fatalf("second cluster err=%v want %v", err, store.ErrMSKLimitExceeded)
	}
}

func TestMSKSharedFAILEDDoesNotHoldSlot(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	c, err := st.CreateMSKCluster(account, "us-east-1", "shared-fail", "3.6.0", 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetMSKContainerID(account, c.ClusterName, "", store.MSKClusterStateFailed, store.MSKSharedBootstrap); err != nil {
		t.Fatal(err)
	}
	c2, err := st.CreateMSKCluster(account, "us-east-1", "shared-retry", "3.6.0", 1, true)
	if err != nil {
		t.Fatalf("create after FAILED: %v", err)
	}
	if c2.BootstrapBrokers != store.MSKSharedBootstrap {
		t.Fatalf("bootstrap=%q", c2.BootstrapBrokers)
	}
}

func TestMSKSharedRemapRefusePerClusterBootstrap(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateMSKCluster(account, "us-east-1", "legacy-msk", "3.6.0", 1, false); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateMSKCluster(account, "us-east-1", "shared-new", "3.6.0", 1, true)
	if !errors.Is(err, store.ErrMSKSharedRemapRefused) {
		t.Fatalf("shared create err=%v want %v", err, store.ErrMSKSharedRemapRefused)
	}
}

func TestMSKSharedRemapRefusePerClusterContainerID(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateMSKCluster(account, "us-east-1", "legacy-id", "3.6.0", 1, false); err != nil {
		t.Fatal(err)
	}
	if err := st.SetMSKContainerID(account, "legacy-id", "noctaxris-msk-legacy-id", store.MSKClusterStateActive, "noctaxris-msk-legacy-id:9092"); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateMSKCluster(account, "us-east-1", "shared-after", "3.6.0", 1, true)
	if !errors.Is(err, store.ErrMSKSharedRemapRefused) {
		t.Fatalf("remap refuse err=%v want %v", err, store.ErrMSKSharedRemapRefused)
	}
}
