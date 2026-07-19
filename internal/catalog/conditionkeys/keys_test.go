package conditionkeys

import "testing"

func TestKnownIncludesGlobalAndRejectsTypo(t *testing.T) {
	if !Known("aws:PrincipalAccount") {
		t.Fatal("expected aws:PrincipalAccount known")
	}
	if !Known("aws:RequestedRegion") {
		t.Fatal("expected aws:RequestedRegion known")
	}
	if Known("aws:NotARealKeyZZZ") {
		t.Fatal("typo must be unknown")
	}
}

func TestKnownMatchesTagTemplates(t *testing.T) {
	if !Known("aws:RequestTag/env") {
		t.Fatal("expected aws:RequestTag/env known via template")
	}
	if !Known("s3:ExistingObjectTag/foo") {
		t.Fatal("expected s3:ExistingObjectTag/foo known via template")
	}
}

func TestKnownServiceKeys(t *testing.T) {
	for _, key := range []string{
		"iam:PassedToService",
		"kms:CallerAccount",
		"s3:prefix",
		"dynamodb:LeadingKeys",
		"lambda:FunctionArn",
		"ssm:Overwrite",
		"ssm:Recursive",
		"secretsmanager:KmsKeyArn",
		"secretsmanager:SecretId",
	} {
		if !Known(key) {
			t.Fatalf("expected %q known", key)
		}
	}
}

func TestKnownSSMAndSecretsTagTemplates(t *testing.T) {
	if !Known("ssm:resourceTag/env") {
		t.Fatal("expected ssm:resourceTag/env known via template")
	}
	if !Known("secretsmanager:ResourceTag/tag-key") {
		t.Fatal("expected secretsmanager:ResourceTag/tag-key known from SAR catalog")
	}
}
