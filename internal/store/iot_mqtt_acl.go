package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	labMQTTDynsecFile       = "dynamic-security.json"
	labMQTTDynsecPluginPath = "/usr/lib/mosquitto_dynamic_security.so"

	// LabMQTTBridgeUsername is the Mosquitto username for the API bridge
	// (TLS CN when use_identity_as_username is true).
	LabMQTTBridgeUsername = "noctaxris-mqtt-bridge"
	labMQTTBridgeClientID = "noctaxris-mqtt-bridge"
)

// SetMQTTBrokerAuthHook registers a callback after IoT principal/policy/cert changes
// so live Mosquitto ACL and dynsec can be rewritten (Shared MQTT only).
func (s *Store) SetMQTTBrokerAuthHook(fn func()) {
	if s == nil {
		return
	}
	s.mqttBrokerAuthMu.Lock()
	s.mqttBrokerAuthHook = fn
	s.mqttBrokerAuthMu.Unlock()
}

func (s *Store) notifyMQTTBrokerAuth() {
	if s == nil {
		return
	}
	s.mqttBrokerAuthMu.Lock()
	fn := s.mqttBrokerAuthHook
	s.mqttBrokerAuthMu.Unlock()
	if fn != nil {
		fn()
	}
}

func writeMQTTAuthStamp(dir, aclBody string, dynsecBody []byte) (string, error) {
	sum := sha256.Sum256([]byte(aclBody + "\n" + string(dynsecBody)))
	hexSum := hex.EncodeToString(sum[:8])
	matches, _ := filepath.Glob(filepath.Join(dir, "auth-*.stamp"))
	for _, p := range matches {
		_ = os.Remove(p)
	}
	stampPath := filepath.Join(dir, "auth-"+hexSum+".stamp")
	if err := os.WriteFile(stampPath, []byte(hexSum+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write mosquitto auth stamp: %w", err)
	}
	return stampPath, nil
}

func (s *Store) labMQTTAuthFiles() (aclBody string, dynsecBody []byte, err error) {
	devices, err := s.listIoTMQTTAttachedDevices()
	if err != nil {
		return "", nil, err
	}
	activeIDs, err := s.listActiveIoTCertificateIDs()
	if err != nil {
		return "", nil, err
	}
	aclBody = labMQTTACLBody(devices)
	dynsecBody, err = labMQTTDynsecBody(devices, activeIDs, s.AllowMQTTConnect)
	if err != nil {
		return "", nil, err
	}
	return aclBody, dynsecBody, nil
}

func (s *Store) listIoTMQTTAttachedDevices() ([]IoTMQTTDeviceContext, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT c.account_id, c.region, c.certificate_id, tp.thing_name
		 FROM iot_certificates c
		 JOIN iot_thing_principals tp
		   ON tp.account_id = c.account_id AND tp.region = c.region AND tp.principal = c.certificate_arn
		 WHERE c.status = 'ACTIVE'
		 ORDER BY tp.thing_name, c.certificate_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list mqtt attached devices: %w", err)
	}
	defer rows.Close()
	var out []IoTMQTTDeviceContext
	for rows.Next() {
		var d IoTMQTTDeviceContext
		if err := rows.Scan(&d.AccountID, &d.Region, &d.CertificateID, &d.ThingName); err != nil {
			return nil, fmt.Errorf("list mqtt attached devices scan: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) listActiveIoTCertificateIDs() ([]string, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT certificate_id FROM iot_certificates WHERE status = 'ACTIVE' ORDER BY certificate_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list active iot certificates: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list active iot certificates scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func mqttACLIdentitySafe(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, "#+\n\r\t /") {
		return false
	}
	return true
}

func mqttDuplicateCertificateIDs(devices []IoTMQTTDeviceContext) map[string]struct{} {
	n := map[string]int{}
	for _, d := range devices {
		id := strings.TrimSpace(d.CertificateID)
		if id == "" {
			continue
		}
		n[id]++
	}
	out := map[string]struct{}{}
	for id, c := range n {
		if c > 1 {
			out[id] = struct{}{}
		}
	}
	return out
}

func labMQTTACLBody(devices []IoTMQTTDeviceContext) string {
	dups := mqttDuplicateCertificateIDs(devices)
	var b strings.Builder
	b.WriteString("# Lab MQTT: the API bridge may read/write any topic. Device usernames are TLS CNs (certificateId).\n")
	b.WriteString("# Device lines are scoped to the attached thing. There is no pattern readwrite # for devices.\n")
	b.WriteString("user " + LabMQTTBridgeUsername + "\n")
	b.WriteString("topic readwrite #\n")
	b.WriteString("topic read $SYS/#\n")
	seen := map[string]struct{}{}
	for _, d := range devices {
		if !mqttACLIdentitySafe(d.CertificateID) || !mqttACLIdentitySafe(d.ThingName) {
			continue
		}
		if _, dup := dups[d.CertificateID]; dup {
			continue
		}
		if _, ok := seen[d.CertificateID]; ok {
			continue
		}
		seen[d.CertificateID] = struct{}{}
		b.WriteString("\nuser " + d.CertificateID + "\n")
		b.WriteString("topic readwrite $aws/things/" + d.ThingName + "/shadow/#\n")
		b.WriteString("topic readwrite $aws/things/" + d.ThingName + "/#\n")
		b.WriteString("topic readwrite " + d.ThingName + "/#\n")
		b.WriteString("topic readwrite " + d.ThingName + "\n")
	}
	return b.String()
}

type mqttDynsecFile struct {
	DefaultACLAccess mqttDynsecDefaultACL `json:"defaultACLAccess"`
	Clients          []mqttDynsecClient   `json:"clients"`
	Groups           []struct{}           `json:"groups"`
	Roles            []mqttDynsecRole     `json:"roles"`
}

type mqttDynsecDefaultACL struct {
	PublishClientSend    bool `json:"publishClientSend"`
	PublishClientReceive bool `json:"publishClientReceive"`
	Subscribe            bool `json:"subscribe"`
	Unsubscribe          bool `json:"unsubscribe"`
}

type mqttDynsecClient struct {
	Username string              `json:"username"`
	TextName string              `json:"textname,omitempty"`
	ClientID string              `json:"clientid,omitempty"`
	Disabled bool                `json:"disabled,omitempty"`
	Roles    []mqttDynsecRoleRef `json:"roles"`
}

type mqttDynsecRoleRef struct {
	RoleName string `json:"rolename"`
}

type mqttDynsecRole struct {
	RoleName string          `json:"rolename"`
	ACLs     []mqttDynsecACL `json:"acls"`
}

type mqttDynsecACL struct {
	ACLType  string `json:"acltype"`
	Topic    string `json:"topic"`
	Priority int    `json:"priority"`
	Allow    bool   `json:"allow"`
}

func labMQTTDynsecBody(devices []IoTMQTTDeviceContext, activeCertIDs []string, allowConnect func(certificateID, clientID string) bool) ([]byte, error) {
	if allowConnect == nil {
		allowConnect = func(string, string) bool { return false }
	}
	roles := []mqttDynsecRole{
		{
			RoleName: "noctaxris-bridge",
			ACLs:     mqttDynsecAllowTopics("#"),
		},
	}
	roleByThing := map[string]struct{}{}
	clients := []mqttDynsecClient{
		{
			Username: LabMQTTBridgeUsername,
			TextName: "API MQTT shadow bridge",
			ClientID: labMQTTBridgeClientID,
			Roles:    []mqttDynsecRoleRef{{RoleName: "noctaxris-bridge"}},
		},
	}
	dups := mqttDuplicateCertificateIDs(devices)
	seenUser := map[string]struct{}{LabMQTTBridgeUsername: {}}
	for _, d := range devices {
		if !mqttACLIdentitySafe(d.CertificateID) || !mqttACLIdentitySafe(d.ThingName) {
			continue
		}
		if _, ok := seenUser[d.CertificateID]; ok {
			continue
		}
		if _, dup := dups[d.CertificateID]; dup {
			clients = append(clients, mqttDynsecClient{
				Username: d.CertificateID,
				Disabled: true,
				Roles:    []mqttDynsecRoleRef{},
			})
			seenUser[d.CertificateID] = struct{}{}
			continue
		}
		roleName := "thing-" + d.ThingName
		if _, ok := roleByThing[d.ThingName]; !ok {
			roles = append(roles, mqttDynsecRole{
				RoleName: roleName,
				ACLs: mqttDynsecAllowTopics(
					"$aws/things/"+d.ThingName+"/shadow/#",
					"$aws/things/"+d.ThingName+"/#",
					d.ThingName+"/#",
					d.ThingName,
				),
			})
			roleByThing[d.ThingName] = struct{}{}
		}
		cl := mqttDynsecClient{
			Username: d.CertificateID,
			ClientID: d.ThingName,
			Roles:    []mqttDynsecRoleRef{{RoleName: roleName}},
		}
		if !allowConnect(d.CertificateID, d.ThingName) {
			cl.Disabled = true
		}
		clients = append(clients, cl)
		seenUser[d.CertificateID] = struct{}{}
	}
	for _, id := range activeCertIDs {
		if _, ok := seenUser[id]; ok {
			continue
		}
		if !mqttACLIdentitySafe(id) {
			continue
		}
		clients = append(clients, mqttDynsecClient{
			Username: id,
			Disabled: true,
			Roles:    []mqttDynsecRoleRef{},
		})
		seenUser[id] = struct{}{}
	}
	body := mqttDynsecFile{
		DefaultACLAccess: mqttDynsecDefaultACL{},
		Clients:          clients,
		Groups:           []struct{}{},
		Roles:            roles,
	}
	raw, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal mosquitto dynsec: %w", err)
	}
	raw = append(raw, '\n')
	return raw, nil
}

func mqttDynsecAllowTopics(topics ...string) []mqttDynsecACL {
	var out []mqttDynsecACL
	for _, topic := range topics {
		for _, aclType := range []string{
			"publishClientSend",
			"publishClientReceive",
			"subscribePattern",
			"unsubscribePattern",
		} {
			out = append(out, mqttDynsecACL{
				ACLType:  aclType,
				Topic:    topic,
				Priority: 0,
				Allow:    true,
			})
		}
	}
	return out
}
