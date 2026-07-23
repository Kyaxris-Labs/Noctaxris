package sdk_test

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Minimal Cognito USER_SRP_AUTH client (amazon-cognito-identity-js / warrant shape).

const (
	sdkSRPNHex = "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AAAC42DAD33170D04507A33A85521ABDF1CBA64" +
		"ECFB850458DBEF0A8AEA71575D060C7DB3970F85A6E1E4C7" +
		"ABF5AE8CDB0933D71E8C94E04A25619DCEE3D2261AD2EE6B" +
		"F12FFA06D98A0864D87602733EC86A64521F2B18177B200C" +
		"BBE117577A615D6C770988C0BAD946E208E24FA074E5AB31" +
		"43DB5BFCE0FD108E4B82D120A93AD2CAFFFFFFFFFFFFFFFF"
)

var (
	sdkSRPN = mustSDKHexBig(sdkSRPNHex)
	sdkSRPG = big.NewInt(2)
	sdkSRPK = mustSDKHexBig(sdkHexHash("00" + sdkSRPNHex + "0" + "2"))
)

type sdkSRPClient struct {
	poolID   string
	username string
	password string
	smallA   *big.Int
	largeA   *big.Int
}

func newSDKSRPClient(poolID, username, password string) (*sdkSRPClient, error) {
	a, err := rand.Int(rand.Reader, sdkSRPN)
	if err != nil {
		return nil, err
	}
	if a.Sign() == 0 {
		a = big.NewInt(1)
	}
	A := new(big.Int).Exp(sdkSRPG, a, sdkSRPN)
	return &sdkSRPClient{
		poolID: poolID, username: username, password: password,
		smallA: a, largeA: A,
	}, nil
}

func (c *sdkSRPClient) srpAHex() string { return strings.ToLower(c.largeA.Text(16)) }

func (c *sdkSRPClient) passwordVerifierResponses(params map[string]string, at time.Time) (map[string]string, error) {
	userID := strings.TrimSpace(params["USER_ID_FOR_SRP"])
	if userID == "" {
		userID = c.username
	}
	saltHex := strings.TrimSpace(params["SALT"])
	srpBHex := strings.TrimSpace(params["SRP_B"])
	secretBlock := strings.TrimSpace(params["SECRET_BLOCK"])
	B, ok := new(big.Int).SetString(srpBHex, 16)
	if !ok || B.Sign() == 0 {
		return nil, fmt.Errorf("invalid SRP_B")
	}
	u := sdkCalculateU(c.largeA, B)
	if u.Sign() == 0 {
		return nil, fmt.Errorf("U cannot be zero")
	}
	x := sdkPrivateX(c.poolID, userID, c.password, saltHex)
	gx := new(big.Int).Exp(sdkSRPG, x, sdkSRPN)
	kv := new(big.Int).Mul(sdkSRPK, gx)
	kv.Mod(kv, sdkSRPN)
	base := new(big.Int).Sub(B, kv)
	base.Mod(base, sdkSRPN)
	exp := new(big.Int).Add(c.smallA, new(big.Int).Mul(u, x))
	S := new(big.Int).Exp(base, exp, sdkSRPN)
	ikm, err := hex.DecodeString(sdkPadHex(S))
	if err != nil {
		return nil, err
	}
	salt, err := hex.DecodeString(sdkPadHex(u))
	if err != nil {
		return nil, err
	}
	hkdf := sdkComputeHKDF(ikm, salt)
	secretBytes, err := base64.StdEncoding.DecodeString(secretBlock)
	if err != nil {
		return nil, err
	}
	ts := sdkSRPTimestamp(at)
	msg := append([]byte(sdkPoolName(c.poolID)), []byte(userID)...)
	msg = append(msg, secretBytes...)
	msg = append(msg, []byte(ts)...)
	mac := hmac.New(sha256.New, hkdf)
	_, _ = mac.Write(msg)
	return map[string]string{
		"USERNAME":                    userID,
		"PASSWORD_CLAIM_SECRET_BLOCK": secretBlock,
		"PASSWORD_CLAIM_SIGNATURE":    base64.StdEncoding.EncodeToString(mac.Sum(nil)),
		"TIMESTAMP":                   ts,
	}, nil
}

func mustSDKHexBig(h string) *big.Int {
	n, ok := new(big.Int).SetString(h, 16)
	if !ok {
		panic("bad hex")
	}
	return n
}

func sdkPoolName(poolID string) string {
	if i := strings.Index(poolID, "_"); i >= 0 && i+1 < len(poolID) {
		return poolID[i+1:]
	}
	return poolID
}

func sdkHashSHA256(buf []byte) string {
	sum := sha256.Sum256(buf)
	h := hex.EncodeToString(sum[:])
	if len(h) >= 64 {
		return h
	}
	return strings.Repeat("0", 64-len(h)) + h
}

func sdkHexHash(hexString string) string {
	b, err := hex.DecodeString(hexString)
	if err != nil {
		return sdkHashSHA256(nil)
	}
	return sdkHashSHA256(b)
}

func sdkPadHex(v any) string {
	var h string
	switch t := v.(type) {
	case string:
		h = strings.ToLower(strings.TrimSpace(t))
	case *big.Int:
		h = strings.ToLower(t.Text(16))
	}
	if len(h)%2 == 1 {
		h = "0" + h
	} else if len(h) > 0 && strings.ContainsRune("89abcdef", rune(h[0])) {
		h = "00" + h
	}
	return h
}

func sdkComputeHKDF(ikm, salt []byte) []byte {
	mac := hmac.New(sha256.New, salt)
	_, _ = mac.Write(ikm)
	prk := mac.Sum(nil)
	info := append([]byte("Caldera Derived Key"), 1)
	mac2 := hmac.New(sha256.New, prk)
	_, _ = mac2.Write(info)
	return mac2.Sum(nil)[:16]
}

func sdkCalculateU(a, b *big.Int) *big.Int {
	return mustSDKHexBig(sdkHexHash(sdkPadHex(a) + sdkPadHex(b)))
}

func sdkPrivateX(poolID, username, password, saltHex string) *big.Int {
	inner := sdkHashSHA256([]byte(sdkPoolName(poolID) + username + ":" + password))
	return mustSDKHexBig(sdkHexHash(sdkPadHex(saltHex) + inner))
}

func sdkSRPTimestamp(t time.Time) string {
	s := t.UTC().Format("Mon Jan 02 15:04:05 UTC 2006")
	if len(s) >= 10 && s[8] == '0' {
		s = s[:8] + s[9:]
	}
	return s
}
