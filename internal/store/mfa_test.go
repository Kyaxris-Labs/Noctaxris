package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestVirtualMFADevice(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	seed := []byte("JBSWY3DPEHPK3PXP")

	if _, _, err := st.CreateUser(accountID, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(accountID, "bob"); err != nil {
		t.Fatal(err)
	}

	serial, err := st.CreateVirtualMFADevice(accountID, seed)
	if err != nil {
		t.Fatal(err)
	}
	if serial == "" {
		t.Fatal("empty serial")
	}

	dev, err := st.GetMFADevice(accountID, serial)
	if err != nil {
		t.Fatal(err)
	}
	if dev.Enabled || dev.UserName != "" {
		t.Fatalf("dev=%+v before enable", dev)
	}
	if string(dev.Seed) != string(seed) {
		t.Fatalf("seed = %q, want %q", dev.Seed, seed)
	}

	if err := st.EnableMFADevice(accountID, serial, "alice"); err != nil {
		t.Fatal(err)
	}
	dev, err = st.GetMFADevice(accountID, serial)
	if err != nil {
		t.Fatal(err)
	}
	if !dev.Enabled || dev.UserName != "alice" {
		t.Fatalf("dev=%+v after enable", dev)
	}
	if string(dev.Seed) != string(seed) {
		t.Fatalf("seed after enable = %q", dev.Seed)
	}

	if err := st.EnableMFADevice(accountID, serial, "bob"); !errors.Is(err, store.ErrMFADeviceAlreadyEnabled) {
		t.Fatalf("reassign error = %v, want ErrMFADeviceAlreadyEnabled", err)
	}
	dev, err = st.GetMFADevice(accountID, serial)
	if err != nil {
		t.Fatal(err)
	}
	if !dev.Enabled || dev.UserName != "alice" {
		t.Fatalf("device reassigned unexpectedly: %+v", dev)
	}

	listed, err := st.ListMFADevices(accountID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Serial != serial {
		t.Fatalf("ListMFADevices = %+v", listed)
	}

	if err := st.DeactivateMFADevice(accountID, serial, "alice"); err != nil {
		t.Fatal(err)
	}
	dev, err = st.GetMFADevice(accountID, serial)
	if err != nil {
		t.Fatal(err)
	}
	if dev.Enabled || dev.UserName != "" {
		t.Fatalf("dev=%+v after deactivate", dev)
	}
	listed, err = st.ListMFADevices(accountID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("ListMFADevices after deactivate = %+v", listed)
	}
}
