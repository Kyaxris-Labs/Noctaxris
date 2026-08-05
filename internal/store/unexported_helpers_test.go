package store

import (
	"errors"
	"strings"
	"testing"
)

// openGateStore opens a store in a temp dir for package-store tests.
func openGateStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	key, err := LoadOrCreateMasterKey(dir + "/master.key")
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

const gateTestAccount = "000000000001"

// TestZerosGateClearCloudTrailLogging covers clearCloudTrailLogging (0.0% before).
func TestZerosGateClearCloudTrailLogging(t *testing.T) {
	st := openGateStore(t)
	if err := st.EnsureCloudTrailSchema(); err != nil {
		t.Fatalf("EnsureCloudTrailSchema: %v", err)
	}
	// No trail exists; the UPDATE simply matches zero rows and must not error.
	if err := st.clearCloudTrailLogging(gateTestAccount, "missing-trail"); err != nil {
		t.Fatalf("clearCloudTrailLogging missing: %v", err)
	}
}

// TestZerosGateDynamoStreamSeqOrdForSequence covers dynamoStreamSeqOrdForSequence (0.0% before).
func TestZerosGateDynamoStreamSeqOrdForSequence(t *testing.T) {
	st := openGateStore(t)
	// Missing sequence number must map to ErrDynamoStreamInvalidShard.
	_, err := st.dynamoStreamSeqOrdForSequence(gateTestAccount, "table", "no-such-seq")
	if !errors.Is(err, ErrDynamoStreamInvalidShard) {
		t.Fatalf("err=%v want ErrDynamoStreamInvalidShard", err)
	}
}

// TestZerosGateMaxKinesisSeqOrdForShard covers maxKinesisSeqOrdForShard (0.0% before).
func TestZerosGateMaxKinesisSeqOrdForShard(t *testing.T) {
	st := openGateStore(t)
	// Empty shard must return -1, nil (no records yet).
	n, err := st.maxKinesisSeqOrdForShard(gateTestAccount, "stream", LabKinesisShardID(0))
	if err != nil {
		t.Fatalf("maxKinesisSeqOrdForShard: %v", err)
	}
	if n != -1 {
		t.Fatalf("n=%d want -1 for empty shard", n)
	}
}

// TestZerosGateTransactionCanceledError covers TransactionCanceledError.Error (0.0% before).
func TestZerosGateTransactionCanceledError(t *testing.T) {
	var nilErr *TransactionCanceledError
	if got := nilErr.Error(); got != "TransactionCanceledException" {
		t.Fatalf("nil Error()=%q", got)
	}
	e := &TransactionCanceledError{Reasons: []CancellationReason{{Code: "None", Message: "m"}}}
	if got := e.Error(); got != "TransactionCanceledException" {
		t.Fatalf("Error()=%q", got)
	}
}

// TestZerosGateModifyCFNKMSAlias covers modifyCFNKMSAlias (0.0% before).
func TestZerosGateModifyCFNKMSAlias(t *testing.T) {
	st := openGateStore(t)

	creator := "arn:aws:iam::" + gateTestAccount + ":root"
	// Missing TargetKeyId must fail closed with ErrCFNBadTemplate.
	key, err := st.CreateKey(gateTestAccount, creator, "")
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	if err := st.CreateAlias(gateTestAccount, "alias/lab", key.KeyID); err != nil {
		t.Fatalf("CreateAlias: %v", err)
	}

	// TargetKeyId required.
	err = st.modifyCFNKMSAlias(gateTestAccount, "alias/lab", map[string]any{"AliasName": "alias/lab"}, map[string]any{"AliasName": "alias/lab"})
	if !errors.Is(err, ErrCFNBadTemplate) {
		t.Fatalf("err=%v want ErrCFNBadTemplate (missing TargetKeyId)", err)
	}

	// Immutable AliasName change must be rejected.
	err = st.modifyCFNKMSAlias(gateTestAccount, "alias/lab", map[string]any{"AliasName": "alias/lab"}, map[string]any{"AliasName": "alias/changed", "TargetKeyId": key.KeyID})
	if err == nil {
		t.Fatal("expected immutable AliasName reject")
	}

	// Success path: alias name falls back to physicalID, TargetKeyId present.
	key2, err := st.CreateKey(gateTestAccount, creator, "")
	if err != nil {
		t.Fatalf("CreateKey2: %v", err)
	}
	if err := st.modifyCFNKMSAlias(gateTestAccount, "alias/lab", map[string]any{"AliasName": "alias/lab"}, map[string]any{"AliasName": "alias/lab", "TargetKeyId": key2.KeyID}); err != nil {
		t.Fatalf("modifyCFNKMSAlias success: %v", err)
	}
}

// TestZerosGateModifyCFNKMSAliasMissingAlias covers the UpdateAlias error path.
func TestZerosGateModifyCFNKMSAliasMissingAlias(t *testing.T) {
	st := openGateStore(t)
	creator := "arn:aws:iam::" + gateTestAccount + ":root"
	key, err := st.CreateKey(gateTestAccount, creator, "")
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	// Alias does not exist; UpdateAlias should fail and map to ErrCFNBadTemplate.
	err = st.modifyCFNKMSAlias(gateTestAccount, "alias/does-not-exist", map[string]any{"AliasName": "alias/does-not-exist"}, map[string]any{"AliasName": "alias/does-not-exist", "TargetKeyId": key.KeyID})
	if !errors.Is(err, ErrCFNBadTemplate) {
		t.Fatalf("err=%v want ErrCFNBadTemplate for missing alias", err)
	}
	if err == nil || !strings.Contains(err.Error(), "UpdateAlias") {
		t.Fatalf("err=%v want UpdateAlias context", err)
	}
}
