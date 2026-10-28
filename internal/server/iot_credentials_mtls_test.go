package server_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestIoTCredentialsProviderThingMismatchForbidden(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if rec := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{
		"thingName": "cred-thing",
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreateThing %d %s", rec.Code, rec.Body.String())
	}

	certRec := mustJSONTarget(t, handler, "AWSIotService.CreateKeysAndCertificate", "iot", map[string]any{
		"setAsActive": true,
	}, now)
	if certRec.Code != http.StatusOK {
		t.Fatalf("CreateKeysAndCertificate %d %s", certRec.Code, certRec.Body.String())
	}
	var certOut map[string]any
	_ = json.Unmarshal(certRec.Body.Bytes(), &certOut)
	certARN, _ := certOut["certificateArn"].(string)
	certPEM, _ := certOut["certificatePem"].(string)
	peer := mustParseDeviceCert(t, certPEM)

	if rec := mustJSONTarget(t, handler, "AWSIotService.CreatePolicy", "iot", map[string]any{
		"policyName":     "cred-pol",
		"policyDocument": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["iot:AssumeRoleWithCertificate","iot:*"],"Resource":"*"}]}`,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreatePolicy %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustJSONTarget(t, handler, "AWSIotService.AttachPolicy", "iot", map[string]any{
		"policyName": "cred-pol", "target": certARN,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("AttachPolicy %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustJSONTarget(t, handler, "AWSIotService.AttachThingPrincipal", "iot", map[string]any{
		"thingName": "cred-thing", "principal": certARN,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("AttachThingPrincipal %d %s", rec.Code, rec.Body.String())
	}

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"credentials.iot.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(testAccountID, "iot-creds-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	if rec := mustJSONTarget(t, handler, "AWSIotService.CreateRoleAlias", "iot", map[string]any{
		"roleAlias": "device-creds",
		"roleArn":   roleARN,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreateRoleAlias %d %s", rec.Code, rec.Body.String())
	}

	mismatch := credentialsRequest(t, "device-creds", "wrong-thing", "127.0.0.1", peer)
	mismatchRec := httptest.NewRecorder()
	handler.ServeHTTP(mismatchRec, mismatch)
	if mismatchRec.Code != http.StatusForbidden {
		t.Fatalf("mismatch status=%d body=%q", mismatchRec.Code, mismatchRec.Body.String())
	}

	okReq := credentialsRequest(t, "device-creds", "cred-thing", "127.0.0.1", peer)
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("match status=%d body=%q", okRec.Code, okRec.Body.String())
	}
	if !strings.Contains(okRec.Body.String(), `"accessKeyId"`) ||
		!strings.Contains(okRec.Body.String(), `"secretAccessKey"`) ||
		!strings.Contains(okRec.Body.String(), `"sessionToken"`) ||
		!strings.Contains(okRec.Body.String(), `"expiration"`) {
		t.Fatalf("credentials shape: %s", okRec.Body.String())
	}

	list := deviceJSONRequest(t, "AWSIotService.ListThings", map[string]any{}, peer)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, list)
	if listRec.Code != http.StatusForbidden {
		t.Fatalf("device ListThings status=%d body=%q", listRec.Code, listRec.Body.String())
	}

	aliases := deviceJSONRequest(t, "AWSIotService.ListRoleAliases", map[string]any{}, peer)
	aliasRec := httptest.NewRecorder()
	handler.ServeHTTP(aliasRec, aliases)
	if aliasRec.Code != http.StatusForbidden {
		t.Fatalf("device ListRoleAliases status=%d body=%q", aliasRec.Code, aliasRec.Body.String())
	}
}

func mustParseDeviceCert(t *testing.T, pemBytes string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(pemBytes))
	if block == nil {
		t.Fatal("certificate pem")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func credentialsRequest(t *testing.T, alias, thingName, sni string, peer *x509.Certificate) *http.Request {
	t.Helper()
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/role-aliases/"+alias+"/credentials", nil)
	req.Header.Set("x-amzn-iot-thingname", thingName)
	req.TLS = &tls.ConnectionState{
		ServerName:       sni,
		PeerCertificates: []*x509.Certificate{peer},
	}
	return req
}

func TestIoTCredentialsLoopbackAllowsPublishedHost(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	const thing = "pub-cred-thing"
	const alias = "pub-device-creds"
	peer := seedIoTCredentialsPeer(t, handler, st, thing, "pub-cred-pol", "iot-pub-creds-role", alias)

	sniReq := credentialsRequest(t, alias, thing, "203.0.113.10", peer)
	sniReq.Host = "203.0.113.10:8443"
	sniRec := httptest.NewRecorder()
	handler.ServeHTTP(sniRec, sniReq)
	if sniRec.Code != http.StatusOK || strings.Contains(sniRec.Body.String(), "TLS SNI must match") {
		t.Fatalf("published SNI status=%d body=%q", sniRec.Code, sniRec.Body.String())
	}
	if !strings.Contains(sniRec.Body.String(), `"accessKeyId"`) {
		t.Fatalf("credentials shape: %s", sniRec.Body.String())
	}

	hostReq := credentialsRequest(t, alias, thing, "", peer)
	hostReq.Host = "203.0.113.10:8443"
	hostRec := httptest.NewRecorder()
	handler.ServeHTTP(hostRec, hostReq)
	if hostRec.Code != http.StatusOK || strings.Contains(hostRec.Body.String(), "TLS SNI must match") {
		t.Fatalf("published Host status=%d body=%q", hostRec.Code, hostRec.Body.String())
	}

	mismatch := credentialsRequest(t, alias, "wrong-thing", "203.0.113.10", peer)
	mismatch.Host = "203.0.113.10:8443"
	mismatchRec := httptest.NewRecorder()
	handler.ServeHTTP(mismatchRec, mismatch)
	if mismatchRec.Code != http.StatusForbidden ||
		!strings.Contains(mismatchRec.Body.String(), "x-amzn-iot-thingname must match the certificate thing.") ||
		strings.Contains(mismatchRec.Body.String(), "TLS SNI must match") {
		t.Fatalf("thing mismatch status=%d body=%q", mismatchRec.Code, mismatchRec.Body.String())
	}
}

func TestIoTCredentialsDNSEndpointRejectsMismatchedSNI(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.IoTEndpointHost = "lab.example.local"
	})
	handler := srv.Handler()
	const thing = "dns-cred-thing"
	const alias = "dns-device-creds"
	peer := seedIoTCredentialsPeer(t, handler, st, thing, "dns-cred-pol", "iot-dns-creds-role", alias)

	bad := credentialsRequest(t, alias, thing, "203.0.113.10", peer)
	bad.Host = "203.0.113.10:8443"
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, bad)
	if badRec.Code != http.StatusForbidden ||
		!strings.Contains(badRec.Body.String(), "TLS SNI must match the CredentialProvider endpointAddress.") {
		t.Fatalf("mismatched SNI status=%d body=%q", badRec.Code, badRec.Body.String())
	}

	noSNI := credentialsRequest(t, alias, thing, "", peer)
	noSNI.Host = "credentials.iot.lab.example.local:8443"
	noRec := httptest.NewRecorder()
	handler.ServeHTTP(noRec, noSNI)
	if noRec.Code != http.StatusForbidden ||
		!strings.Contains(noRec.Body.String(), "TLS SNI must match the CredentialProvider endpointAddress.") {
		t.Fatalf("empty SNI status=%d body=%q", noRec.Code, noRec.Body.String())
	}

	okReq := credentialsRequest(t, alias, thing, "credentials.iot.lab.example.local", peer)
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK || strings.Contains(okRec.Body.String(), "TLS SNI must match") {
		t.Fatalf("matching SNI status=%d body=%q", okRec.Code, okRec.Body.String())
	}
}

func seedIoTCredentialsPeer(t *testing.T, handler http.Handler, st *store.Store, thingName, policyName, roleName, alias string) *x509.Certificate {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	if rec := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{
		"thingName": thingName,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreateThing %d %s", rec.Code, rec.Body.String())
	}
	certRec := mustJSONTarget(t, handler, "AWSIotService.CreateKeysAndCertificate", "iot", map[string]any{
		"setAsActive": true,
	}, now)
	if certRec.Code != http.StatusOK {
		t.Fatalf("CreateKeysAndCertificate %d %s", certRec.Code, certRec.Body.String())
	}
	var certOut map[string]any
	_ = json.Unmarshal(certRec.Body.Bytes(), &certOut)
	certARN, _ := certOut["certificateArn"].(string)
	certPEM, _ := certOut["certificatePem"].(string)
	peer := mustParseDeviceCert(t, certPEM)
	if rec := mustJSONTarget(t, handler, "AWSIotService.CreatePolicy", "iot", map[string]any{
		"policyName":     policyName,
		"policyDocument": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["iot:AssumeRoleWithCertificate","iot:*"],"Resource":"*"}]}`,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreatePolicy %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustJSONTarget(t, handler, "AWSIotService.AttachPolicy", "iot", map[string]any{
		"policyName": policyName, "target": certARN,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("AttachPolicy %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustJSONTarget(t, handler, "AWSIotService.AttachThingPrincipal", "iot", map[string]any{
		"thingName": thingName, "principal": certARN,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("AttachThingPrincipal %d %s", rec.Code, rec.Body.String())
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"credentials.iot.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(testAccountID, roleName, trust)
	if err != nil {
		t.Fatal(err)
	}
	if rec := mustJSONTarget(t, handler, "AWSIotService.CreateRoleAlias", "iot", map[string]any{
		"roleAlias": alias,
		"roleArn":   roleARN,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreateRoleAlias %d %s", rec.Code, rec.Body.String())
	}
	return peer
}

func deviceJSONRequest(t *testing.T, target string, payload map[string]any, peer *x509.Certificate) *http.Request {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{peer}}
	return req
}
