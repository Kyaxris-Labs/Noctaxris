package secretsmanager_test

import (
	"encoding/json"
	"testing"

	smsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/secretsmanager"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSecretsManagerJSON(t *testing.T) {
	sec := store.Secret{
		Name: "lab/secret", ARN: "arn:aws:secretsmanager:us-east-1:1:secret:lab/secret",
		SecretString: "value", VersionID: "v1", KmsKeyID: "arn:aws:kms:us-east-1:1:key/k",
		Description: "lab", CreatedDate: "2024-01-01T00:00:00Z", LastChangedDate: "2024-01-02T00:00:00Z",
		SecretBinary: []byte{1, 2, 3},
	}
	if _, err := smsvc.CreateSecretJSON(sec); err != nil {
		t.Fatal(err)
	}
	if _, err := smsvc.GetSecretValueJSON(sec); err != nil {
		t.Fatal(err)
	}
	if _, err := smsvc.PutSecretValueJSON(sec); err != nil {
		t.Fatal(err)
	}
	del, err := smsvc.DeleteSecretJSON(sec, "2024-06-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	var delOut map[string]any
	if err := json.Unmarshal(del, &delOut); err != nil {
		t.Fatal(err)
	}
	desc, err := smsvc.DescribeSecretJSON(sec)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(desc) {
		t.Fatalf("describe invalid json")
	}
	sec.DeletedDate = "2024-06-01T00:00:00Z"
	sec.DeletionDate = "2024-07-01T00:00:00Z"
	sec.RotationEnabled = true
	sec.RotationLambdaARN = "arn:aws:lambda:us-east-1:1:function:rot"
	sec.RotationRoleARN = "arn:aws:iam::1:role/rot"
	sec.NextRotationDate = "2024-08-01T00:00:00Z"
	sec.LastRotatedDate = "2024-07-15T00:00:00Z"
	if _, err := smsvc.DescribeSecretJSON(sec); err != nil {
		t.Fatal(err)
	}
	if _, err := smsvc.RotateSecretJSON(sec); err != nil {
		t.Fatal(err)
	}
	if _, err := smsvc.UpdateSecretVersionStageJSON(sec); err != nil {
		t.Fatal(err)
	}
	if _, err := smsvc.RestoreSecretJSON(sec); err != nil {
		t.Fatal(err)
	}
	if _, err := smsvc.ListSecretsJSON([]store.Secret{sec}); err != nil {
		t.Fatal(err)
	}
	sec.ResourcePolicy = `{"Version":"2012-10-17"}`
	if _, err := smsvc.PutResourcePolicyJSON(sec); err != nil {
		t.Fatal(err)
	}
	if _, err := smsvc.GetResourcePolicyJSON(sec, sec.ResourcePolicy); err != nil {
		t.Fatal(err)
	}
	if _, err := smsvc.DeleteResourcePolicyJSON(sec); err != nil {
		t.Fatal(err)
	}
	if _, err := smsvc.EmptyOKJSON(); err != nil {
		t.Fatal(err)
	}
	if _, err := smsvc.ListTagsForResourceJSON([]store.ResourceTag{{Key: "env", Value: "lab"}}); err != nil {
		t.Fatal(err)
	}
}
