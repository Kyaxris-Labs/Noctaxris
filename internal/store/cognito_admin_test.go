package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCognitoAdminUserOpsStoreCoverage(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000001"

	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "admin-store-pool")
	if err != nil {
		t.Fatal(err)
	}

	_, err = st.AdminGetCognitoUser(account, pool.PoolID, "")
	if !errors.Is(err, store.ErrCognitoBadRequest) {
		t.Fatalf("empty username err=%v", err)
	}
	_, err = st.AdminGetCognitoUser(account, "", "alice")
	if !errors.Is(err, store.ErrCognitoBadRequest) {
		t.Fatalf("empty pool err=%v", err)
	}
	_, err = st.ListCognitoUsers(account, "")
	if !errors.Is(err, store.ErrCognitoBadRequest) {
		t.Fatalf("list empty pool err=%v", err)
	}
	_, err = st.ListCognitoUsers(account, "us-east-1_missing")
	if err == nil {
		t.Fatal("list missing pool should fail")
	}

	empty, err := st.ListCognitoUsers(account, pool.PoolID)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list=%v err=%v", empty, err)
	}

	user, err := st.AdminCreateCognitoUser(account, pool.PoolID, "alice", "TempPass1!")
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.AdminGetCognitoUser(account, pool.PoolID, "alice")
	if err != nil || got.Username != user.Username || !got.Enabled {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	_, err = st.AdminGetCognitoUser(account, pool.PoolID, "missing")
	if !errors.Is(err, store.ErrCognitoUserNotFound) {
		t.Fatalf("missing get err=%v", err)
	}

	listed, err := st.ListCognitoUsers(account, pool.PoolID)
	if err != nil || len(listed) != 1 || listed[0].Username != "alice" {
		t.Fatalf("list=%v err=%v", listed, err)
	}

	if err := st.AdminSetCognitoUserPassword(account, pool.PoolID, "alice", "", true); !errors.Is(err, store.ErrCognitoBadRequest) {
		t.Fatalf("empty password err=%v", err)
	}
	if err := st.AdminSetCognitoUserPassword(account, pool.PoolID, "alice", "ForcePass1!", false); err != nil {
		t.Fatal(err)
	}
	forced, err := st.AdminGetCognitoUser(account, pool.PoolID, "alice")
	if err != nil || forced.UserStatus != "FORCE_CHANGE_PASSWORD" {
		t.Fatalf("force status=%+v err=%v", forced, err)
	}
	if err := st.AdminSetCognitoUserPassword(account, pool.PoolID, "alice", "PermPass1!", true); err != nil {
		t.Fatal(err)
	}
	confirmed, err := st.AdminGetCognitoUser(account, pool.PoolID, "alice")
	if err != nil || confirmed.UserStatus != "CONFIRMED" {
		t.Fatalf("confirmed=%+v err=%v", confirmed, err)
	}
	if err := st.AdminSetCognitoUserPassword(account, pool.PoolID, "ghost", "PermPass1!", true); !errors.Is(err, store.ErrCognitoUserNotFound) && err == nil {
		t.Fatalf("missing set password err=%v", err)
	}

	if err := st.AdminDisableCognitoUser(account, "", "alice"); !errors.Is(err, store.ErrCognitoBadRequest) {
		t.Fatalf("disable bad request err=%v", err)
	}
	if err := st.AdminDisableCognitoUser(account, pool.PoolID, "alice"); err != nil {
		t.Fatal(err)
	}
	disabled, err := st.AdminGetCognitoUser(account, pool.PoolID, "alice")
	if err != nil || disabled.Enabled {
		t.Fatalf("disabled=%+v err=%v", disabled, err)
	}
	if err := st.AdminDisableCognitoUser(account, pool.PoolID, "ghost"); !errors.Is(err, store.ErrCognitoUserNotFound) && err == nil {
		t.Fatalf("disable missing err=%v", err)
	}

	if err := st.AdminDeleteCognitoUser(account, pool.PoolID, ""); !errors.Is(err, store.ErrCognitoBadRequest) {
		t.Fatalf("delete empty err=%v", err)
	}
	if err := st.AdminDeleteCognitoUser(account, pool.PoolID, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := st.AdminDeleteCognitoUser(account, pool.PoolID, "alice"); !errors.Is(err, store.ErrCognitoUserNotFound) && err == nil {
		t.Fatalf("delete twice err=%v", err)
	}
	_, err = st.AdminGetCognitoUser(account, pool.PoolID, "alice")
	if !errors.Is(err, store.ErrCognitoUserNotFound) {
		t.Fatalf("get after delete err=%v", err)
	}
}
