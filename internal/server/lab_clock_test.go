package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLabClockDoesNotExtendSTSSessionExpiry(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabForensics = true
	})
	handler := srv.Handler()
	signNow := time.Now().UTC().Truncate(time.Second)
	labFuture := time.Date(2099, 6, 15, 12, 0, 0, 0, time.UTC)

	rec := mustLabForensicsJSON(t, handler, "NoctaxrisLab.SetClock", map[string]any{
		"FixedTime": labFuture.Format(time.RFC3339),
	}, signNow)
	if rec.Code != http.StatusOK {
		t.Fatalf("SetClock status=%d body=%q", rec.Code, rec.Body.String())
	}

	mintedAt := time.Now().UTC()
	gst := stsForm(t, handler, "Action=GetSessionToken&Version=2011-06-15", signNow)
	if gst.Code != http.StatusOK {
		t.Fatalf("GetSessionToken status=%d body=%q", gst.Code, gst.Body.String())
	}
	assertTempCredWallExpiry(t, st, xmlTag(t, gst.Body.String(), "AccessKeyId"),
		xmlTag(t, gst.Body.String(), "SecretAccessKey"),
		xmlTag(t, gst.Body.String(), "SessionToken"),
		xmlTag(t, gst.Body.String(), "Expiration"),
		mintedAt, time.Hour)

	fed := stsForm(t, handler, "Action=GetFederationToken&Version=2011-06-15&Name=clock-fed", signNow)
	if fed.Code != http.StatusOK {
		t.Fatalf("GetFederationToken status=%d body=%q", fed.Code, fed.Body.String())
	}
	assertTempCredWallExpiry(t, st, xmlTag(t, fed.Body.String(), "AccessKeyId"),
		xmlTag(t, fed.Body.String(), "SecretAccessKey"),
		xmlTag(t, fed.Body.String(), "SessionToken"),
		xmlTag(t, fed.Body.String(), "Expiration"),
		time.Now().UTC(), time.Hour)

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(testAccountID, "clock-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	assumeBody := "Action=AssumeRole&Version=2011-06-15&RoleArn=" + url.QueryEscape(roleARN) +
		"&RoleSessionName=clock-sess&DurationSeconds=900"
	assume := stsForm(t, handler, assumeBody, signNow)
	if assume.Code != http.StatusOK {
		t.Fatalf("AssumeRole status=%d body=%q", assume.Code, assume.Body.String())
	}
	assertTempCredWallExpiry(t, st, xmlTag(t, assume.Body.String(), "AccessKeyId"),
		xmlTag(t, assume.Body.String(), "SecretAccessKey"),
		xmlTag(t, assume.Body.String(), "SessionToken"),
		xmlTag(t, assume.Body.String(), "Expiration"),
		time.Now().UTC(), 900*time.Second)
}

func TestLabClockDoesNotExtendIoTCredentialExpiry(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabForensics = true
	})
	handler := srv.Handler()
	signNow := time.Now().UTC().Truncate(time.Second)
	labFuture := time.Date(2099, 6, 15, 12, 0, 0, 0, time.UTC)

	if rec := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{
		"thingName": "clock-thing",
	}, signNow); rec.Code != http.StatusOK {
		t.Fatalf("CreateThing %d %s", rec.Code, rec.Body.String())
	}

	certRec := mustJSONTarget(t, handler, "AWSIotService.CreateKeysAndCertificate", "iot", map[string]any{
		"setAsActive": true,
	}, signNow)
	if certRec.Code != http.StatusOK {
		t.Fatalf("CreateKeysAndCertificate %d %s", certRec.Code, certRec.Body.String())
	}
	var certOut map[string]any
	if err := json.Unmarshal(certRec.Body.Bytes(), &certOut); err != nil {
		t.Fatal(err)
	}
	certARN, _ := certOut["certificateArn"].(string)
	certPEM, _ := certOut["certificatePem"].(string)
	peer := mustParseDeviceCert(t, certPEM)

	if rec := mustJSONTarget(t, handler, "AWSIotService.CreatePolicy", "iot", map[string]any{
		"policyName":     "clock-pol",
		"policyDocument": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["iot:AssumeRoleWithCertificate","iot:*"],"Resource":"*"}]}`,
	}, signNow); rec.Code != http.StatusOK {
		t.Fatalf("CreatePolicy %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustJSONTarget(t, handler, "AWSIotService.AttachPolicy", "iot", map[string]any{
		"policyName": "clock-pol", "target": certARN,
	}, signNow); rec.Code != http.StatusOK {
		t.Fatalf("AttachPolicy %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustJSONTarget(t, handler, "AWSIotService.AttachThingPrincipal", "iot", map[string]any{
		"thingName": "clock-thing", "principal": certARN,
	}, signNow); rec.Code != http.StatusOK {
		t.Fatalf("AttachThingPrincipal %d %s", rec.Code, rec.Body.String())
	}

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"credentials.iot.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(testAccountID, "iot-clock-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	if rec := mustJSONTarget(t, handler, "AWSIotService.CreateRoleAlias", "iot", map[string]any{
		"roleAlias": "clock-creds",
		"roleArn":   roleARN,
	}, signNow); rec.Code != http.StatusOK {
		t.Fatalf("CreateRoleAlias %d %s", rec.Code, rec.Body.String())
	}

	rec := mustLabForensicsJSON(t, handler, "NoctaxrisLab.SetClock", map[string]any{
		"FixedTime": labFuture.Format(time.RFC3339),
	}, signNow)
	if rec.Code != http.StatusOK {
		t.Fatalf("SetClock status=%d body=%q", rec.Code, rec.Body.String())
	}

	okReq := credentialsRequest(t, "clock-creds", "clock-thing", "127.0.0.1", peer)
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("credentials status=%d body=%q", okRec.Code, okRec.Body.String())
	}
	var credOut struct {
		Credentials struct {
			AccessKeyID     string `json:"accessKeyId"`
			SecretAccessKey string `json:"secretAccessKey"`
			SessionToken    string `json:"sessionToken"`
			Expiration      string `json:"expiration"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(okRec.Body.Bytes(), &credOut); err != nil {
		t.Fatal(err)
	}
	assertTempCredWallExpiry(t, st, credOut.Credentials.AccessKeyID,
		credOut.Credentials.SecretAccessKey,
		credOut.Credentials.SessionToken,
		credOut.Credentials.Expiration,
		time.Now().UTC(), time.Hour)
}

func assertTempCredWallExpiry(
	t *testing.T,
	st *store.Store,
	accessKeyID, secret, sessionToken, expiration string,
	mintedAt time.Time,
	dur time.Duration,
) {
	t.Helper()
	want := mintedAt.UTC().Add(dur)
	gotXML, err := time.Parse(time.RFC3339, expiration)
	if err != nil {
		t.Fatalf("Expiration parse %q: %v", expiration, err)
	}
	if gotXML.Year() >= 2090 {
		t.Fatalf("lab clock leaked into Expiration %s", expiration)
	}
	if gotXML.Before(want.Add(-5*time.Second)) || gotXML.After(want.Add(8*time.Second)) {
		t.Fatalf("Expiration=%s want ~%s", gotXML.Format(time.RFC3339), want.Format(time.RFC3339))
	}

	ak, err := st.LookupAccessKeyRecord(accessKeyID)
	if err != nil {
		t.Fatal(err)
	}
	if ak.ExpiresAt.Year() >= 2090 {
		t.Fatalf("lab clock leaked into store ExpiresAt %s", ak.ExpiresAt)
	}
	if ak.ExpiresAt.Before(want.Add(-5*time.Second)) || ak.ExpiresAt.After(want.Add(8*time.Second)) {
		t.Fatalf("store ExpiresAt=%s want ~%s", ak.ExpiresAt.Format(time.RFC3339), want.Format(time.RFC3339))
	}

	liveBody := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	liveReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", liveBody)
	signHeader(t, liveReq, liveBody, accessKeyID, secret, testRegion, "sts", time.Now().UTC().Truncate(time.Second))
	liveReq.Header.Set("X-Amz-Security-Token", sessionToken)
	if _, err := authn.Verify(liveReq, liveBody, time.Now().UTC(), 15*time.Minute, testKeyLookup(st)); err != nil {
		t.Fatalf("unexpired session Verify: %v", err)
	}

	expiredNow := ak.ExpiresAt.Add(time.Second)
	expBody := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	expReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", expBody)
	signHeader(t, expReq, expBody, accessKeyID, secret, testRegion, "sts", expiredNow)
	expReq.Header.Set("X-Amz-Security-Token", sessionToken)
	_, err = authn.Verify(expReq, expBody, expiredNow, 15*time.Minute, testKeyLookup(st))
	if authn.Code(err) != authn.CodeInvalidClientTokenId {
		t.Fatalf("expired session Code=%q err=%v want InvalidClientTokenId", authn.Code(err), err)
	}
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired session err=%v want expired", err)
	}
}

func testKeyLookup(st *store.Store) authn.KeyLookup {
	return func(id string) (authn.ResolvedKey, error) {
		ak, err := st.LookupAccessKeyRecord(id)
		if err != nil {
			return authn.ResolvedKey{}, err
		}
		return authn.ResolvedKey{
			AccountID:    ak.AccountID,
			Secret:       ak.Secret,
			Status:       ak.Status,
			SessionToken: ak.SessionToken,
			ExpiresAt:    ak.ExpiresAt,
		}, nil
	}
}
