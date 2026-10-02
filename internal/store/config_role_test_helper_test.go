package store_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// ensureConfigDeliveryRole creates a role trusted by config.amazonaws.com with s3:PutObject.
func ensureConfigDeliveryRole(t *testing.T, st *store.Store, account, roleName string) string {
	t.Helper()
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"config.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, roleName, trust)
	if err != nil {
		t.Fatal(err)
	}
	allow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:PutObject","sns:Publish"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "config-delivery", allow); err != nil {
		t.Fatal(err)
	}
	return roleARN
}
