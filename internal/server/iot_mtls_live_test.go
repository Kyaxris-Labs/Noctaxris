package server_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/server"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func iotKeysFromCreateResponse(t *testing.T, body []byte) (certPEM, privateKey string) {
	t.Helper()
	var out struct {
		CertificatePem string `json:"certificatePem"`
		KeyPair        struct {
			PrivateKey string `json:"PrivateKey"`
		} `json:"keyPair"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.CertificatePem == "" || out.KeyPair.PrivateKey == "" {
		t.Fatalf("CreateKeysAndCertificate missing certificatePem/keyPair.PrivateKey: %s", body)
	}
	return out.CertificatePem, out.KeyPair.PrivateKey
}

func startIoTDeviceTLSServer(t *testing.T, handler http.Handler, st *store.Store) *httptest.Server {
	t.Helper()
	caPEM, err := st.LabIoTCACertificatePEM()
	if err != nil {
		t.Fatal(err)
	}
	dns, ips := store.DefaultIoTServerSANs()
	material, err := st.EnsureLabIoTServerCertificate(dns, ips)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair([]byte(material.CertPEM), []byte(material.KeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	cfg := server.DeviceClientAuthTLSConfig(cert, caPEM)
	if cfg.ClientAuth != tls.VerifyClientCertIfGiven {
		t.Fatalf("ClientAuth=%v want VerifyClientCertIfGiven", cfg.ClientAuth)
	}
	ts := httptest.NewUnstartedServer(handler)
	_ = ts.Listener.Close()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ts.Listener = ln
	ts.TLS = cfg
	ts.StartTLS()
	t.Cleanup(ts.Close)
	return ts
}

func iotLabCAClient(t *testing.T, st *store.Store, serverName string, device tls.Certificate, withDevice bool) *http.Client {
	t.Helper()
	caPEM, err := st.LabIoTCACertificatePEM()
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(caPEM)) {
		t.Fatal("lab CA PEM")
	}
	tlsCfg := &tls.Config{
		RootCAs:    roots,
		ServerName: serverName,
		MinVersion: tls.VersionTLS12,
	}
	if withDevice {
		tlsCfg.Certificates = []tls.Certificate{device}
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}
}

func provisionIoTDevice(t *testing.T, handler http.Handler, thingName, policyName, policyDoc string) (certPEM, privateKey, certARN string) {
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
	if err := json.Unmarshal(certRec.Body.Bytes(), &certOut); err != nil {
		t.Fatal(err)
	}
	certARN, _ = certOut["certificateArn"].(string)
	if certARN == "" {
		t.Fatalf("missing certificateArn: %s", certRec.Body.String())
	}
	certPEM, privateKey = iotKeysFromCreateResponse(t, certRec.Body.Bytes())
	if rec := mustJSONTarget(t, handler, "AWSIotService.CreatePolicy", "iot", map[string]any{
		"policyName":     policyName,
		"policyDocument": policyDoc,
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
	return certPEM, privateKey, certARN
}

func TestIoTCredentialsLiveTLSDeviceCert(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	const thingName = "tls-cred-thing"
	const alias = "tls-device-creds"
	certPEM, keyPEM, _ := provisionIoTDevice(t, handler, thingName, "tls-cred-pol",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["iot:AssumeRoleWithCertificate","iot:*"],"Resource":"*"}]}`)

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"credentials.iot.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(testAccountID, "iot-tls-creds-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	if rec := mustJSONTarget(t, handler, "AWSIotService.CreateRoleAlias", "iot", map[string]any{
		"roleAlias": alias,
		"roleArn":   roleARN,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreateRoleAlias %d %s", rec.Code, rec.Body.String())
	}

	ts := startIoTDeviceTLSServer(t, handler, st)
	devicePair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatal(err)
	}
	client := iotLabCAClient(t, st, "127.0.0.1", devicePair, true)
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/role-aliases/"+alias+"/credentials", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-amzn-iot-thingname", thingName)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("mTLS credentials GET: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mTLS credentials status=%d body=%q", resp.StatusCode, raw)
	}
	body := string(raw)
	if !strings.Contains(body, `"accessKeyId"`) ||
		!strings.Contains(body, `"secretAccessKey"`) ||
		!strings.Contains(body, `"sessionToken"`) {
		t.Fatalf("credentials shape: %s", body)
	}

	noCert := iotLabCAClient(t, st, "127.0.0.1", tls.Certificate{}, false)
	bare, err := http.NewRequest(http.MethodGet, ts.URL+"/role-aliases/"+alias+"/credentials", nil)
	if err != nil {
		t.Fatal(err)
	}
	bare.Header.Set("x-amzn-iot-thingname", thingName)
	bareResp, err := noCert.Do(bare)
	if err != nil {
		t.Fatalf("TLS GET without client cert: %v", err)
	}
	defer bareResp.Body.Close()
	bareBody, _ := io.ReadAll(bareResp.Body)
	if bareResp.StatusCode != http.StatusForbidden || !strings.Contains(string(bareBody), "MissingAuthenticationToken") {
		t.Fatalf("credentials without client cert want 403 MissingAuthenticationToken: %d %s", bareResp.StatusCode, bareBody)
	}
}

func TestIoTListNamedShadowsLiveMTLS(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	const thingName = "tls-shadow-thing"
	const shadowName = "night"
	certPEM, keyPEM, _ := provisionIoTDevice(t, handler, thingName, "tls-shadow-pol",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["iot:ListNamedShadowsForThing","iot:*"],"Resource":"*"}]}`)

	named := mustJSONTarget(t, handler, "AWSIotDataService.UpdateThingShadow", "iot-data", map[string]any{
		"thingName":  thingName,
		"shadowName": shadowName,
		"payload":    map[string]any{"state": map[string]any{"reported": map[string]any{"n": 1}}},
	}, now)
	if named.Code != http.StatusOK {
		t.Fatalf("named UpdateThingShadow %d %s", named.Code, named.Body.String())
	}

	ts := startIoTDeviceTLSServer(t, handler, st)
	devicePair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatal(err)
	}
	client := iotLabCAClient(t, st, "127.0.0.1", devicePair, true)
	req, err := http.NewRequest(http.MethodGet,
		ts.URL+"/api/things/shadow/ListNamedShadowsForThing/"+thingName, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("mTLS named-shadow list: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"`+shadowName+`"`) {
		t.Fatalf("mTLS named list status=%d body=%q", resp.StatusCode, raw)
	}
}
