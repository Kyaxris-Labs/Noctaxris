package kms

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

const (
	// SigningAlgorithmPSSSHA256 is the lab primary RSA signing algorithm.
	SigningAlgorithmPSSSHA256 = "RSASSA_PSS_SHA_256"
	// SigningAlgorithmPKCS1SHA256 is also accepted for Sign/Verify.
	SigningAlgorithmPKCS1SHA256 = "RSASSA_PKCS1_V1_5_SHA_256"

	MessageTypeRAW    = "RAW"
	MessageTypeDIGEST = "DIGEST"
)

// ErrUnsupportedSigningAlgorithm is returned for SigningAlgorithm values outside the lab set.
var ErrUnsupportedSigningAlgorithm = errors.New("unsupported signing algorithm")

// ErrInvalidMessageType is returned when MessageType is not RAW or DIGEST.
var ErrInvalidMessageType = errors.New("invalid message type")

// ErrInvalidDigestLength is returned when MessageType=DIGEST and Message is not 32 bytes.
var ErrInvalidDigestLength = errors.New("digest must be 32 bytes for SHA-256")

// NormalizeSigningAlgorithm returns the algorithm to use. Empty defaults to RSASSA_PSS_SHA_256.
func NormalizeSigningAlgorithm(alg string) (string, error) {
	alg = strings.TrimSpace(alg)
	if alg == "" {
		return SigningAlgorithmPSSSHA256, nil
	}
	switch alg {
	case SigningAlgorithmPSSSHA256, SigningAlgorithmPKCS1SHA256:
		return alg, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedSigningAlgorithm, alg)
	}
}

// NormalizeMessageType returns RAW when empty.
func NormalizeMessageType(mt string) (string, error) {
	mt = strings.TrimSpace(mt)
	if mt == "" {
		return MessageTypeRAW, nil
	}
	switch mt {
	case MessageTypeRAW, MessageTypeDIGEST:
		return mt, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidMessageType, mt)
	}
}

func digestForSign(message []byte, messageType string) ([]byte, error) {
	switch messageType {
	case MessageTypeRAW:
		sum := sha256.Sum256(message)
		return sum[:], nil
	case MessageTypeDIGEST:
		if len(message) != sha256.Size {
			return nil, ErrInvalidDigestLength
		}
		out := make([]byte, len(message))
		copy(out, message)
		return out, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidMessageType, messageType)
	}
}

// SignRSA signs message with the private key using the given SigningAlgorithm and MessageType.
func SignRSA(priv *rsa.PrivateKey, message []byte, signingAlgorithm, messageType string) ([]byte, error) {
	alg, err := NormalizeSigningAlgorithm(signingAlgorithm)
	if err != nil {
		return nil, err
	}
	mt, err := NormalizeMessageType(messageType)
	if err != nil {
		return nil, err
	}
	hashed, err := digestForSign(message, mt)
	if err != nil {
		return nil, err
	}
	switch alg {
	case SigningAlgorithmPSSSHA256:
		return rsa.SignPSS(rand.Reader, priv, crypto.SHA256, hashed, &rsa.PSSOptions{
			SaltLength: rsa.PSSSaltLengthEqualsHash,
			Hash:       crypto.SHA256,
		})
	case SigningAlgorithmPKCS1SHA256:
		return rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, hashed)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedSigningAlgorithm, alg)
	}
}

// VerifyRSA verifies signature over message with the public key.
func VerifyRSA(pub *rsa.PublicKey, message, signature []byte, signingAlgorithm, messageType string) (bool, error) {
	alg, err := NormalizeSigningAlgorithm(signingAlgorithm)
	if err != nil {
		return false, err
	}
	mt, err := NormalizeMessageType(messageType)
	if err != nil {
		return false, err
	}
	hashed, err := digestForSign(message, mt)
	if err != nil {
		return false, err
	}
	switch alg {
	case SigningAlgorithmPSSSHA256:
		verr := rsa.VerifyPSS(pub, crypto.SHA256, hashed, signature, &rsa.PSSOptions{
			SaltLength: rsa.PSSSaltLengthEqualsHash,
			Hash:       crypto.SHA256,
		})
		return verr == nil, nil
	case SigningAlgorithmPKCS1SHA256:
		verr := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hashed, signature)
		return verr == nil, nil
	default:
		return false, fmt.Errorf("%w: %s", ErrUnsupportedSigningAlgorithm, alg)
	}
}

// PublicKeyPEMSPKI returns a PEM-encoded SubjectPublicKeyInfo block for pub.
func PublicKeyPEMSPKI(pub *rsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("marshal spki: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

// LabSigningAlgorithms lists SigningAlgorithm values supported for RSA_2048 SIGN_VERIFY keys.
func LabSigningAlgorithms() []string {
	return []string{SigningAlgorithmPSSSHA256, SigningAlgorithmPKCS1SHA256}
}
