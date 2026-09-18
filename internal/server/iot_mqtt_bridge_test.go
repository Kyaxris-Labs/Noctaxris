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

func TestShadowMQTTBridgeEmptyCertificateIDRejected(t *testing.T) {
	st := openBridgeStore(t)
	account := testAccountID
	region := testRegion
	thing, err := st.CreateIoTThing(account, region, "mqtt-empty-cert", nil)
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
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["iot:Connect","iot:Publish"],"Resource":"*"}]}`
	if _, err := st.CreateIoTPolicy(account, region, "shadow-connect", doc); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "shadow-connect", cert.CertificateARN); err != nil {
		t.Fatal(err)
	}

	var publishedTopic string
	h := server.NewShadowMQTTHandlerForTest(st, func(topic string, payload []byte) error {
		publishedTopic = topic
		return nil
	})
	topic := "$aws/things/" + thing.ThingName + "/shadow/update"
	if err := h.HandleMessage("", topic, []byte(`{"state":{"reported":{"x":1}}}`)); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if !strings.Contains(publishedTopic, "/update/rejected") {
		t.Fatalf("empty certificateId want rejected, topic=%q", publishedTopic)
	}
	if _, err := st.GetIoTThingShadow(account, region, thing.ThingName, ""); err == nil {
		t.Fatal("empty certificateId must not write the topic thing shadow")
	}
}

func TestShadowMQTTBridgeDeviceACannotUpdateThingB(t *testing.T) {
	st := openBridgeStore(t)
	account := testAccountID
	region := testRegion
	thingA, err := st.CreateIoTThing(account, region, "mqtt-thing-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	thingB, err := st.CreateIoTThing(account, region, "mqtt-thing-b", nil)
	if err != nil {
		t.Fatal(err)
	}
	certA, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	certB, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["iot:Connect","iot:Publish"],"Resource":"*"}]}`
	if _, err := st.CreateIoTPolicy(account, region, "all-iot", doc); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "all-iot", certA.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "all-iot", certB.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thingA.ThingName, certA.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thingB.ThingName, certB.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateIoTThingShadow(account, region, thingB.ThingName, "", map[string]any{
		"state": map[string]any{"reported": map[string]any{"keep": true}},
	}); err != nil {
		t.Fatal(err)
	}

	var publishedTopic string
	h := server.NewShadowMQTTHandlerForTest(st, func(topic string, payload []byte) error {
		publishedTopic = topic
		return nil
	})
	topicB := "$aws/things/" + thingB.ThingName + "/shadow/update"
	payload := []byte(`{"state":{"reported":{"pwn":true}}}`)
	if err := h.HandleMessage(certA.CertificateID, topicB, payload); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if !strings.Contains(publishedTopic, "/update/rejected") {
		t.Fatalf("device A on thing B want rejected, topic=%q", publishedTopic)
	}
	sh, err := st.GetIoTThingShadow(account, region, thingB.ThingName, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sh.PayloadJSON, "pwn") {
		t.Fatalf("thing B shadow mutated by device A: %s", sh.PayloadJSON)
	}
}

func TestShadowMQTTHandleBrokerMessageUsesConnectCert(t *testing.T) {
	st := openBridgeStore(t)
	account := testAccountID
	region := testRegion
	thing, err := st.CreateIoTThing(account, region, "mqtt-broker-thing", nil)
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
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["iot:Connect","iot:Publish"],"Resource":"*"}]}`
	if _, err := st.CreateIoTPolicy(account, region, "connect-pub", doc); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "connect-pub", cert.CertificateARN); err != nil {
		t.Fatal(err)
	}

	var publishedTopic string
	h := server.NewShadowMQTTHandlerForTest(st, func(topic string, payload []byte) error {
		publishedTopic = topic
		return nil
	})
	topic := "$aws/things/" + thing.ThingName + "/shadow/update"
	if err := h.HandleBrokerMessage(topic, []byte(`{"state":{"reported":{"ok":1}}}`)); err != nil {
		t.Fatalf("HandleBrokerMessage: %v", err)
	}
	if !strings.Contains(publishedTopic, "/update/accepted") {
		t.Fatalf("want accepted, topic=%q", publishedTopic)
	}
}

func TestShadowMQTTHandleBrokerMessageRejectsWithoutConnect(t *testing.T) {
	st := openBridgeStore(t)
	account := testAccountID
	region := testRegion
	thing, err := st.CreateIoTThing(account, region, "mqtt-broker-noconnect", nil)
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
	if _, err := st.CreateIoTPolicy(account, region, "pub-only", doc); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "pub-only", cert.CertificateARN); err != nil {
		t.Fatal(err)
	}

	var publishedTopic string
	h := server.NewShadowMQTTHandlerForTest(st, func(topic string, payload []byte) error {
		publishedTopic = topic
		return nil
	})
	topic := "$aws/things/" + thing.ThingName + "/shadow/update"
	if err := h.HandleBrokerMessage(topic, []byte(`{"state":{"reported":{"ok":1}}}`)); err != nil {
		t.Fatalf("HandleBrokerMessage: %v", err)
	}
	if !strings.Contains(publishedTopic, "/update/rejected") {
		t.Fatalf("want rejected without Connect, topic=%q", publishedTopic)
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

func TestPersistMQTTRetainedWritesStore(t *testing.T) {
	st := openBridgeStore(t)
	account := testAccountID
	region := testRegion
	thing, err := st.CreateIoTThing(account, region, "retain-thing", nil)
	if err != nil {
		t.Fatal(err)
	}
	topic := thing.ThingName + "/status"
	if err := server.PersistMQTTRetainedForTest(st, topic, []byte("hot"), 1); err != nil {
		t.Fatal(err)
	}
	got, err := st.ListIoTRetainedMessages(account, region)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Topic != topic || string(got[0].Payload) != "hot" || got[0].QoS != 1 {
		t.Fatalf("retained=%v", got)
	}
	if err := server.PersistMQTTRetainedForTest(st, topic, nil, 0); err != nil {
		t.Fatal(err)
	}
	got, err = st.ListIoTRetainedMessages(account, region)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("cleared list=%v", got)
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
