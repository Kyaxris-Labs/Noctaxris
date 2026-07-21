package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestEvaluateAssociatedWAFFailsClosedOnMissingACL(t *testing.T) {
	dir := t.TempDir()
	key, err := LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	account := "000000000001"
	acl, err := st.CreateWAFWebACL(account, "us-east-1", "orphan-lab", "REGIONAL", "", "Allow", nil)
	if err != nil {
		t.Fatal(err)
	}
	resource := "arn:aws:apigateway:us-east-1::/apis/orphan1/stages/$default"
	if err := st.AssociateWAFWebACL(account, acl.ARN, resource); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`DELETE FROM wafv2_web_acls WHERE account_id = ? AND arn = ?`, account, acl.ARN); err != nil {
		t.Fatal(err)
	}

	action, associated, err := st.EvaluateAssociatedWAF(account, []string{resource}, "")
	if !associated {
		t.Fatal("expected association present")
	}
	if !errors.Is(err, ErrWAFNotFound) {
		t.Fatalf("want ErrWAFNotFound, got action=%q err=%v", action, err)
	}
}
