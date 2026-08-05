package store

import (
	"path/filepath"
	"testing"
)

func TestDeliveryServiceResourceTagPrefix(t *testing.T) {
	cases := []struct {
		arn  string
		want string
	}{
		{"", ""},
		{"arn", ""},
		{"arn:aws:ecr:us-east-1:1:repository/x", "ecr:ResourceTag/"},
		{"arn:aws:ssm:us-east-1:1:parameter/x", "ssm:resourceTag/"},
		{"arn:aws:secretsmanager:us-east-1:1:secret:x", "secretsmanager:ResourceTag/"},
		{"arn:aws:iam::1:role/x", "iam:ResourceTag/"},
		{"arn:aws:ecs:us-east-1:1:service/x", "ecs:ResourceTag/"},
		{"arn:aws:s3:::bucket", ""},
	}
	for _, tc := range cases {
		if got := deliveryServiceResourceTagPrefix(tc.arn); got != tc.want {
			t.Fatalf("arn=%q got %q want %q", tc.arn, got, tc.want)
		}
	}
}

func TestDeliveryRoleSessionConditionKeys(t *testing.T) {
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
	if err := st.EnsureRoot(account, "AKIATESTROOT01", "secret-root-value-32chars!!!!!!!!"); err != nil {
		t.Fatal(err)
	}

	// Boundary: empty region defaults; empty role omits PrincipalArn
	keys := st.deliveryRoleSessionConditionKeys(account, "", "arn:aws:s3:::lab", "")
	if keys["aws:PrincipalAccount"] != account || keys["aws:PrincipalType"] != "AssumedRole" {
		t.Fatalf("keys=%v", keys)
	}
	if keys["aws:RequestedRegion"] == "" {
		t.Fatal("expected default region")
	}
	if _, ok := keys["aws:PrincipalArn"]; ok {
		t.Fatalf("unexpected PrincipalArn: %v", keys)
	}

	roleKeys := st.deliveryRoleSessionConditionKeys(account, "DeliveryRole", "*", "eu-west-1")
	if roleKeys["aws:PrincipalArn"] != "arn:aws:iam::"+account+":role/DeliveryRole" {
		t.Fatalf("PrincipalArn=%v", roleKeys)
	}
	if roleKeys["aws:RequestedRegion"] != "eu-west-1" {
		t.Fatalf("region=%v", roleKeys)
	}

	// Negative/security: mergeDeliveryResourceTagKeys no-ops on empty/* and unknown ARN
	m := map[string]string{}
	st.mergeDeliveryResourceTagKeys(m, account, "")
	st.mergeDeliveryResourceTagKeys(m, account, "*")
	st.mergeDeliveryResourceTagKeys(nil, account, "arn:aws:s3:::x")
	if len(m) != 0 {
		t.Fatalf("expected no tags merged, got %v", m)
	}
}
