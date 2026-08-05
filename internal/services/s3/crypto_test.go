package s3_test

import (
	"bytes"
	"testing"

	s3svc "github.com/Kyaxris-Labs/Noctaxris/internal/services/s3"
)

func TestEncryptDecryptAES256GCM(t *testing.T) {
	dek := bytes.Repeat([]byte{0xab}, 32)
	plain := []byte("lab-object-bytes")
	sealed, err := s3svc.EncryptAES256GCM(dek, plain)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := s3svc.DecryptAES256GCM(dek, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(opened, plain) {
		t.Fatalf("roundtrip=%q", opened)
	}
}

func TestEncryptDecryptErrors(t *testing.T) {
	dek := make([]byte, 16)
	if _, err := s3svc.EncryptAES256GCM(dek, []byte("x")); err == nil {
		t.Fatal("expected dek length error on encrypt")
	}
	if _, err := s3svc.DecryptAES256GCM(dek, []byte("short")); err == nil {
		t.Fatal("expected dek length error on decrypt")
	}
	goodDEK := bytes.Repeat([]byte{1}, 32)
	if _, err := s3svc.DecryptAES256GCM(goodDEK, []byte{0}); err == nil {
		t.Fatal("expected ciphertext too short")
	}
	sealed, err := s3svc.EncryptAES256GCM(goodDEK, []byte("tamper"))
	if err != nil {
		t.Fatal(err)
	}
	sealed[len(sealed)-1] ^= 0xff
	if _, err := s3svc.DecryptAES256GCM(goodDEK, sealed); err == nil {
		t.Fatal("expected auth failure on tamper")
	}
}
