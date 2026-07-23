package sdk_test

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

func TestKMSEncryptDecryptRoundTrip(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newKMS(t, cfg)
	ctx := context.Background()

	created, err := client.CreateKey(ctx, &kms.CreateKeyInput{
		Description: aws.String(uniquePrefix(t) + "-key"),
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	if created.KeyMetadata == nil || created.KeyMetadata.KeyId == nil {
		t.Fatal("CreateKey missing KeyId")
	}
	keyID := *created.KeyMetadata.KeyId
	t.Cleanup(func() {
		_, _ = client.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{
			KeyId:               aws.String(keyID),
			PendingWindowInDays: aws.Int32(7),
		})
	})

	plain := []byte("noctaxris-kms")
	enc, err := client.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(keyID),
		Plaintext: plain,
	})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(enc.CiphertextBlob) == 0 {
		t.Fatal("Encrypt returned empty ciphertext")
	}

	dec, err := client.Decrypt(ctx, &kms.DecryptInput{CiphertextBlob: enc.CiphertextBlob})
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(dec.Plaintext) != string(plain) {
		t.Fatalf("Decrypt plaintext=%q want=%q (b64=%s)",
			dec.Plaintext, plain, base64.StdEncoding.EncodeToString(dec.Plaintext))
	}
}
