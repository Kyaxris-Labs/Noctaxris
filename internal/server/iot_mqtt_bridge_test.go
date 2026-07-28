package server_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/server"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMQTTBridgeDialAddressDefault(t *testing.T) {
	t.Setenv("NOCTAXRIS_MQTT_BRIDGE_BROKER", "")
	if got := server.MQTTBridgeDialAddress(); got != "noctaxris-engine:1883" {
		t.Fatalf("dial=%q", got)
	}
	t.Setenv("NOCTAXRIS_MQTT_BRIDGE_BROKER", "test-broker:1993")
	if got := server.MQTTBridgeDialAddress(); got != "test-broker:1993" {
		t.Fatalf("override dial=%q", got)
	}
}

func TestShadowMQTTBridgeUpdateMatchesHTTPStore(t *testing.T) {
	st := openBridgeStore(t)
	account := testAccountID
	region := testRegion
	thing, err := st.CreateIoTThing(account, region, "mqtt-bridge-thing", nil)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thing.ThingName, cert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:Publish","Resource":"*"}]}`
	if _, err := st.CreateIoTPolicy(account, region, "shadow-pub", doc); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "shadow-pub", cert.CertificateARN); err != nil {
		t.Fatal(err)
	}

	var publishedTopic string
	var publishedBody []byte
	h := server.NewShadowMQTTHandlerForTest(st, func(topic string, payload []byte) error {
		publishedTopic = topic
		publishedBody = append([]byte(nil), payload...)
		return nil
	})

	updateTopic := "$aws/things/" + thing.ThingName + "/shadow/update"
	payload := []byte(`{"state":{"reported":{"temp":21}}}`)
	if err := h.HandleMessage(cert.CertificateID, updateTopic, payload); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	wantTopic := "$aws/things/" + thing.ThingName + "/shadow/update/accepted"
	if publishedTopic != wantTopic {
		t.Fatalf("published topic=%q want %q", publishedTopic, wantTopic)
	}
	sh, err := st.GetIoTThingShadow(account, region, thing.ThingName, "")
	if err != nil {
		t.Fatal(err)
	}
	var mqttDoc map[string]any
	if err := json.Unmarshal(publishedBody, &mqttDoc); err != nil {
		t.Fatal(err)
	}
	var httpDoc map[string]any
	if err := json.Unmarshal([]byte(sh.PayloadJSON), &httpDoc); err != nil {
		t.Fatal(err)
	}
	if mqttDoc["version"] != httpDoc["version"] {
		t.Fatalf("version mqtt=%v http=%v", mqttDoc["version"], httpDoc["version"])
	}
}

func TestShadowMQTTBridgePolicyDeny(t *testing.T) {
	st := openBridgeStore(t)
	account := testAccountID
	region := testRegion
	thing, err := st.CreateIoTThing(account, region, "mqtt-deny-thing", nil)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thing.ThingName, cert.CertificateARN); err != nil {
		t.Fatal(err)
	}

	var publishedTopic string
	h := server.NewShadowMQTTHandlerForTest(st, func(topic string, payload []byte) error {
		publishedTopic = topic
		return nil
	})
	topic := "$aws/things/" + thing.ThingName + "/shadow/update"
	if err := h.HandleMessage(cert.CertificateID, topic, []byte(`{"state":{"reported":{"x":1}}}`)); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if !strings.Contains(publishedTopic, "/update/rejected") {
		t.Fatalf("want rejected publish, topic=%q", publishedTopic)
	}
}

func openBridgeStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
