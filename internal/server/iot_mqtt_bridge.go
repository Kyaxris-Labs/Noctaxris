package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	defaultMQTTBridgeBroker = "noctaxris-engine:1883"
	envMQTTBridgeBroker     = "NOCTAXRIS_MQTT_BRIDGE_BROKER"
)

// MQTTBridgeDialAddress is the broker endpoint the API bridge dials (Compose network).
func MQTTBridgeDialAddress() string {
	v := strings.TrimSpace(os.Getenv(envMQTTBridgeBroker))
	if v != "" {
		return v
	}
	return defaultMQTTBridgeBroker
}

type shadowTopicOp int

const (
	shadowOpUnknown shadowTopicOp = iota
	shadowOpUpdate
	shadowOpGet
	shadowOpDelete
)

type parsedShadowTopic struct {
	ThingName  string
	ShadowName string
	Op         shadowTopicOp
	Response   string
}

// shadowMQTTHandler maps MQTT shadow topics to the SQLite shadow store with IoT device policy checks.
type shadowMQTTHandler struct {
	store   *store.Store
	publish func(topic string, payload []byte) error
}

func newShadowMQTTHandler(st *store.Store, publish func(topic string, payload []byte) error) *shadowMQTTHandler {
	if publish == nil {
		publish = func(string, []byte) error { return nil }
	}
	return &shadowMQTTHandler{store: st, publish: publish}
}

// HandleMessage processes one MQTT publish for classic or named shadows.
// certificateID is the lab device certificate id (DER SHA-256 hex). Prefer setting the MQTT
// ClientId to certificateId so multi-cert things resolve unambiguously. When empty, the handler
// picks the unique ACTIVE cert on the thing that Allows the shadow action (fail closed if zero
// or multiple Allows).
func (h *shadowMQTTHandler) HandleMessage(certificateID, topic string, payload []byte) error {
	if h == nil || h.store == nil {
		return fmt.Errorf("shadow mqtt handler unavailable")
	}
	parsed, err := parseShadowTopic(topic)
	if err != nil {
		return err
	}
	if parsed.Op == shadowOpUnknown {
		return nil
	}
	thing, err := h.store.DescribeIoTThingByNameGlobal(parsed.ThingName)
	if err != nil {
		return h.publishRejected(shadowRejectTopic(parsed.Response), map[string]any{
			"code":    403,
			"message": "not authorized",
		})
	}
	action, resource := shadowPolicyActionResource(parsed.Op, thing.AccountID, thing.Region, topic)
	dev, err := h.store.ResolveIoTMQTTDeviceForThingShadow(
		thing.AccountID, thing.Region, thing.ThingName, certificateID, action, resource,
	)
	if err != nil {
		return h.publishRejected(shadowRejectTopic(parsed.Response), map[string]any{
			"code":    403,
			"message": "not authorized",
		})
	}
	if !h.store.EvaluateIoTDevicePolicy(dev.AccountID, dev.Region, dev.CertificateID, action, resource) {
		return h.publishRejected(shadowRejectTopic(parsed.Response), map[string]any{
			"code":    403,
			"message": "not authorized",
		})
	}
	switch parsed.Op {
	case shadowOpUpdate:
		return h.handleUpdate(dev, parsed, payload)
	case shadowOpGet:
		return h.handleGet(dev, parsed)
	case shadowOpDelete:
		return h.handleDelete(dev, parsed)
	default:
		return nil
	}
}

// ShadowMQTTHandler exposes shadow MQTT handling for unit tests.
type ShadowMQTTHandler struct {
	inner *shadowMQTTHandler
}

// NewShadowMQTTHandlerForTest builds a handler with a fake publish callback.
func NewShadowMQTTHandlerForTest(st *store.Store, publish func(topic string, payload []byte) error) *ShadowMQTTHandler {
	return &ShadowMQTTHandler{inner: newShadowMQTTHandler(st, publish)}
}

// HandleMessage implements shadow topic handling (see shadowMQTTHandler.HandleMessage).
func (h *ShadowMQTTHandler) HandleMessage(certificateID, topic string, payload []byte) error {
	if h == nil || h.inner == nil {
		return fmt.Errorf("shadow mqtt handler unavailable")
	}
	return h.inner.HandleMessage(certificateID, topic, payload)
}

func (h *shadowMQTTHandler) handleUpdate(dev store.IoTMQTTDeviceContext, parsed parsedShadowTopic, payload []byte) error {
	var doc map[string]any
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &doc); err != nil {
			return h.publishRejected(shadowRejectTopic(parsed.Response), map[string]any{
				"code":    400,
				"message": "invalid JSON document",
			})
		}
	}
	sh, err := h.store.UpdateIoTThingShadow(dev.AccountID, dev.Region, dev.ThingName, parsed.ShadowName, doc)
	if err != nil {
		return h.publishRejected(shadowRejectTopic(parsed.Response), map[string]any{
			"code":    400,
			"message": err.Error(),
		})
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(sh.PayloadJSON), &out); err != nil {
		out = map[string]any{"state": map[string]any{}}
	}
	return h.publishAccepted(parsed.Response, out)
}

func (h *shadowMQTTHandler) handleGet(dev store.IoTMQTTDeviceContext, parsed parsedShadowTopic) error {
	sh, err := h.store.GetIoTThingShadow(dev.AccountID, dev.Region, dev.ThingName, parsed.ShadowName)
	if err != nil {
		if errors.Is(err, store.ErrIoTNotFound) {
			return h.publishRejected(shadowRejectTopic(parsed.Response), map[string]any{
				"code":    404,
				"message": "shadow not found",
			})
		}
		return h.publishRejected(shadowRejectTopic(parsed.Response), map[string]any{
			"code":    400,
			"message": err.Error(),
		})
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(sh.PayloadJSON), &out); err != nil {
		out = map[string]any{}
	}
	return h.publishAccepted(parsed.Response, out)
}

func (h *shadowMQTTHandler) handleDelete(dev store.IoTMQTTDeviceContext, parsed parsedShadowTopic) error {
	if err := h.store.DeleteIoTThingShadow(dev.AccountID, dev.Region, dev.ThingName, parsed.ShadowName); err != nil {
		if errors.Is(err, store.ErrIoTNotFound) {
			return h.publishRejected(shadowRejectTopic(parsed.Response), map[string]any{
				"code":    404,
				"message": "shadow not found",
			})
		}
		return h.publishRejected(shadowRejectTopic(parsed.Response), map[string]any{
			"code":    400,
			"message": err.Error(),
		})
	}
	return h.publishAccepted(parsed.Response, map[string]any{})
}

func shadowRejectTopic(accepted string) string {
	return strings.Replace(accepted, "/accepted", "/rejected", 1)
}

func (h *shadowMQTTHandler) publishAccepted(topic string, body map[string]any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return h.publish(topic, raw)
}

func (h *shadowMQTTHandler) publishRejected(topic string, body map[string]any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return h.publish(topic, raw)
}

func parseShadowTopic(topic string) (parsedShadowTopic, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return parsedShadowTopic{}, fmt.Errorf("empty topic")
	}
	parts := strings.Split(topic, "/")
	if len(parts) < 5 || parts[0] != "$aws" || parts[1] != "things" {
		return parsedShadowTopic{Op: shadowOpUnknown}, nil
	}
	thing := parts[2]
	shadowName := ""
	rest := parts[3:]
	if len(rest) >= 2 && rest[0] == "shadow" {
		if rest[1] == "name" && len(rest) >= 4 {
			shadowName = rest[2]
			rest = rest[3:]
		} else {
			rest = rest[1:]
		}
	} else {
		return parsedShadowTopic{Op: shadowOpUnknown}, nil
	}
	if len(rest) != 1 {
		return parsedShadowTopic{Op: shadowOpUnknown}, nil
	}
	op := shadowOpUnknown
	switch rest[0] {
	case "update":
		op = shadowOpUpdate
	case "get":
		op = shadowOpGet
	case "delete":
		op = shadowOpDelete
	default:
		return parsedShadowTopic{Op: shadowOpUnknown}, nil
	}
	base := "$aws/things/" + thing + "/shadow"
	if shadowName != "" {
		base = base + "/name/" + shadowName
	}
	response := base
	switch op {
	case shadowOpUpdate:
		response += "/update/accepted"
	case shadowOpGet:
		response += "/get/accepted"
	case shadowOpDelete:
		response += "/delete/accepted"
	}
	return parsedShadowTopic{
		ThingName:  thing,
		ShadowName: shadowName,
		Op:         op,
		Response:   response,
	}, nil
}

func shadowPolicyActionResource(op shadowTopicOp, accountID, region, topic string) (action, resource string) {
	resource = iotTopicResourceARN(accountID, region, topic)
	switch op {
	case shadowOpUpdate:
		action = "iot:Publish"
	case shadowOpGet:
		action = "iot:Publish"
	case shadowOpDelete:
		action = "iot:Publish"
	default:
		action = "iot:Publish"
	}
	return action, resource
}

func iotTopicResourceARN(accountID, region, topic string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		region = store.DefaultIoTRegion
	}
	return fmt.Sprintf("arn:aws:iot:%s:%s:topic/%s", region, strings.TrimSpace(accountID), strings.TrimPrefix(topic, "/"))
}

// ShadowMQTTSubscribeFilters returns topic filters for the API shadow bridge.
func ShadowMQTTSubscribeFilters() []string {
	return []string{
		"$aws/things/+/shadow/update",
		"$aws/things/+/shadow/get",
		"$aws/things/+/shadow/delete",
		"$aws/things/+/shadow/name/+/update",
		"$aws/things/+/shadow/name/+/get",
		"$aws/things/+/shadow/name/+/delete",
	}
}
