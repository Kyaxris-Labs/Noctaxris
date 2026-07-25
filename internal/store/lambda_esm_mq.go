package store

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	actionMQDescribeBroker = "mq:DescribeBroker"

	// LambdaESMLabMQQueue is the lab default destination queue for MQ ESM polls.
	LambdaESMLabMQQueue = "noctaxris"

	mqLabHostPrefix = "noctaxris-mq-"
)

var (
	// ErrESMMQBrokerNotReady is returned when Create targets a non-RUNNING or stub:// broker.
	ErrESMMQBrokerNotReady = errors.New("ValidationException: Amazon MQ broker must be RUNNING with a nested (non-stub) endpoint")
	// ErrESMMQHostNotAllowed is returned when the broker endpoint host is not an allowlisted nested MQ host.
	ErrESMMQHostNotAllowed = errors.New("InvalidParameterValueException: Amazon MQ endpoint host is not an allowlisted nested data-plane host")
)

// MQESMMessage is one lab MQ message delivered to a Lambda function via ESM.
type MQESMMessage struct {
	MessageID   string
	Data        []byte
	Destination string
	Redelivered bool
}

// MQReceiveFunc receives up to batchSize messages from a validated nested AMQP endpoint.
// Unit tests inject this to avoid DinD/broker. When nil, the poller dials the allowlisted host only.
type MQReceiveFunc func(endpoint string, batchSize int) ([]MQESMMessage, error)

var (
	mqReceiveMu   sync.Mutex
	mqReceiveFunc MQReceiveFunc
)

// SetMQReceiveFunc registers an injectable MQ receive hook for unit tests.
// Pass nil to restore the default allowlisted TCP dial path.
func SetMQReceiveFunc(fn MQReceiveFunc) {
	mqReceiveMu.Lock()
	defer mqReceiveMu.Unlock()
	mqReceiveFunc = fn
}

func getMQReceiveFunc() MQReceiveFunc {
	mqReceiveMu.Lock()
	defer mqReceiveMu.Unlock()
	return mqReceiveFunc
}

// parseMQBrokerARN parses arn:aws:mq:region:account:broker:NAME:ID (MQBrokerARN shape).
func parseMQBrokerARN(arn string) (accountID, brokerName, brokerID string, ok bool) {
	arn = strings.TrimSpace(arn)
	parts := strings.Split(arn, ":")
	// arn aws mq region account broker NAME ID  => 8 parts
	if len(parts) < 8 || parts[0] != "arn" || parts[1] != "aws" || parts[2] != "mq" {
		return "", "", "", false
	}
	if parts[5] != "broker" {
		return "", "", "", false
	}
	accountID = strings.TrimSpace(parts[4])
	brokerName = strings.TrimSpace(parts[6])
	brokerID = strings.TrimSpace(parts[7])
	if accountID == "" || brokerName == "" || brokerID == "" {
		return "", "", "", false
	}
	return accountID, brokerName, brokerID, true
}

func isMQEventSourceARN(arn string) bool {
	_, _, _, ok := parseMQBrokerARN(arn)
	return ok
}

// ValidateNestedMQHost allows only DinD nested MQ hostnames (noctaxris-mq-<id>).
// Loopback, wildcards, and raw IPs are rejected so the ESM poller never dials an
// operator-published or open network endpoint.
func ValidateNestedMQHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("%w: empty host", ErrESMMQHostNotAllowed)
	}
	lower := strings.ToLower(host)
	switch lower {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "*", "host.docker.internal":
		return fmt.Errorf("%w: refusing non-nested mq host %q", ErrESMMQHostNotAllowed, host)
	}
	if strings.Contains(host, "/") || strings.Contains(host, "\\") {
		return fmt.Errorf("%w: invalid nested mq host", ErrESMMQHostNotAllowed)
	}
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("%w: refusing IP mq host %q", ErrESMMQHostNotAllowed, host)
	}
	if !strings.HasPrefix(lower, mqLabHostPrefix) {
		return fmt.Errorf("%w: nested mq host %q is not a data-plane endpoint", ErrESMMQHostNotAllowed, host)
	}
	suffix := strings.TrimPrefix(lower, mqLabHostPrefix)
	if suffix == "" {
		return fmt.Errorf("%w: nested mq host %q is missing a broker suffix", ErrESMMQHostNotAllowed, host)
	}
	return nil
}

func parseMQAMQPEndpoint(endpoint string) (host string, port int, err error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || strings.HasPrefix(endpoint, "stub://") {
		return "", 0, ErrESMMQBrokerNotReady
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" {
		// host:port form
		h, p, splitErr := net.SplitHostPort(endpoint)
		if splitErr != nil {
			return "", 0, fmt.Errorf("%w: invalid endpoint", ErrESMMQHostNotAllowed)
		}
		portNum, perr := strconv.Atoi(p)
		if perr != nil || portNum <= 0 {
			return "", 0, fmt.Errorf("%w: invalid port", ErrESMMQHostNotAllowed)
		}
		if verr := ValidateNestedMQHost(h); verr != nil {
			return "", 0, verr
		}
		return h, portNum, nil
	}
	if u.Scheme != "amqp" && u.Scheme != "amqps" {
		return "", 0, fmt.Errorf("%w: endpoint scheme must be amqp", ErrESMMQHostNotAllowed)
	}
	host = u.Hostname()
	if host == "" {
		return "", 0, fmt.Errorf("%w: empty host", ErrESMMQHostNotAllowed)
	}
	port = MQNestedPort
	if u.Port() != "" {
		portNum, perr := strconv.Atoi(u.Port())
		if perr != nil || portNum <= 0 {
			return "", 0, fmt.Errorf("%w: invalid port", ErrESMMQHostNotAllowed)
		}
		port = portNum
	}
	if verr := ValidateNestedMQHost(host); verr != nil {
		return "", 0, verr
	}
	return host, port, nil
}

func (s *Store) validateMQEventSourceARN(accountID, arn string) error {
	acct, name, id, ok := parseMQBrokerARN(arn)
	if !ok || acct != accountID {
		return ErrInvalidEventSourceARN
	}
	b, err := s.DescribeMQBroker(accountID, id)
	if err != nil {
		return ErrInvalidEventSourceARN
	}
	if b.BrokerName != name || b.BrokerARN != strings.TrimSpace(arn) {
		return ErrInvalidEventSourceARN
	}
	if b.BrokerState != MQBrokerStateRunning {
		return ErrESMMQBrokerNotReady
	}
	ep := strings.TrimSpace(b.StubEndpoint)
	if ep == "" || strings.HasPrefix(ep, "stub://") {
		return ErrESMMQBrokerNotReady
	}
	if _, _, err := parseMQAMQPEndpoint(ep); err != nil {
		if errors.Is(err, ErrESMMQBrokerNotReady) || errors.Is(err, ErrESMMQHostNotAllowed) {
			return err
		}
		return ErrESMMQBrokerNotReady
	}
	return nil
}

func (s *Store) pollMQEventSourceMappingOnce(m LambdaEventSourceMapping, invoke ESMInvokeFunc) error {
	acct, _, id, ok := parseMQBrokerARN(m.EventSourceARN)
	if !ok || acct != m.AccountID {
		return ErrInvalidEventSourceARN
	}
	b, err := s.DescribeMQBroker(m.AccountID, id)
	if err != nil {
		return err
	}
	if b.BrokerState != MQBrokerStateRunning {
		return ErrESMMQBrokerNotReady
	}
	ep := strings.TrimSpace(b.StubEndpoint)
	if ep == "" || strings.HasPrefix(ep, "stub://") {
		return ErrESMMQBrokerNotReady
	}
	if _, _, err := parseMQAMQPEndpoint(ep); err != nil {
		return err
	}
	batchSize := m.BatchSize
	if batchSize <= 0 {
		batchSize = LambdaESMDefaultBatchSize
	}
	msgs, err := s.receiveMQESMMessages(ep, batchSize)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}
	eventJSON, err := buildMQLambdaEventJSON(m.EventSourceARN, b.EngineType, msgs)
	if err != nil {
		return err
	}
	_, invErr := invoke(m.AccountID, m.FunctionName, esmMappingQualifier(m), eventJSON)
	return invErr
}

func (s *Store) receiveMQESMMessages(endpoint string, batchSize int) ([]MQESMMessage, error) {
	host, port, err := parseMQAMQPEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	if fn := getMQReceiveFunc(); fn != nil {
		return fn(endpoint, batchSize)
	}
	return defaultMQReceiveDial(host, port)
}

// defaultMQReceiveDial dials the allowlisted nested host:port only (no operator hosts).
// Without an injectable MQReceiveFunc / AMQP client, lab lite returns an empty batch after a successful dial.
func defaultMQReceiveDial(host string, port int) ([]MQESMMessage, error) {
	if err := ValidateNestedMQHost(host); err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("mq esm dial %s: %w", addr, err)
	}
	_ = conn.Close()
	return nil, nil
}

func buildMQLambdaEventJSON(eventSourceARN, engineType string, msgs []MQESMMessage) (string, error) {
	engineType = strings.ToUpper(strings.TrimSpace(engineType))
	if engineType == "RABBITMQ" {
		return buildRabbitMQLambdaEventJSON(eventSourceARN, msgs)
	}
	return buildActiveMQLambdaEventJSON(eventSourceARN, msgs)
}

func buildActiveMQLambdaEventJSON(eventSourceARN string, msgs []MQESMMessage) (string, error) {
	out := make([]map[string]any, 0, len(msgs))
	now := time.Now().UTC().UnixMilli()
	for i, msg := range msgs {
		dest := strings.TrimSpace(msg.Destination)
		if dest == "" {
			dest = LambdaESMLabMQQueue
		}
		id := strings.TrimSpace(msg.MessageID)
		if id == "" {
			id = fmt.Sprintf("lab-mq-%d", i+1)
		}
		out = append(out, map[string]any{
			"messageID":    id,
			"messageType":  "jms/text-message",
			"deliveryMode": 1,
			"replyTo":      nil,
			"type":         nil,
			"expiration":   nil,
			"priority":     0,
			"correlationId": nil,
			"redelivered":  msg.Redelivered,
			"destination":  map[string]any{"physicalName": dest},
			"data":         base64.StdEncoding.EncodeToString(msg.Data),
			"timestamp":    now,
			"brokerInTime": now,
			"brokerOutTime": now,
			"properties":   map[string]any{},
		})
	}
	raw, err := json.Marshal(map[string]any{
		"eventSource":    "aws:mq",
		"eventSourceArn": eventSourceARN,
		"messages":       out,
	})
	if err != nil {
		return "", fmt.Errorf("marshal activemq lambda event: %w", err)
	}
	return string(raw), nil
}

func buildRabbitMQLambdaEventJSON(eventSourceARN string, msgs []MQESMMessage) (string, error) {
	key := LambdaESMLabMQQueue + "::/"
	list := make([]map[string]any, 0, len(msgs))
	for _, msg := range msgs {
		list = append(list, map[string]any{
			"basicProperties": map[string]any{
				"contentType":     "text/plain",
				"contentEncoding": nil,
				"headers":         map[string]any{},
				"deliveryMode":    1,
				"priority":        0,
				"correlationId":   nil,
				"replyTo":         nil,
				"expiration":      nil,
				"messageId":       msg.MessageID,
				"timestamp":       nil,
				"type":            nil,
				"userId":          nil,
				"appId":           nil,
				"clusterId":       nil,
				"bodySize":        len(msg.Data),
			},
			"redelivered": msg.Redelivered,
			"data":        base64.StdEncoding.EncodeToString(msg.Data),
		})
	}
	raw, err := json.Marshal(map[string]any{
		"eventSource":        "aws:rmq",
		"eventSourceArn":     eventSourceARN,
		"rmqMessagesByQueue": map[string]any{key: list},
	})
	if err != nil {
		return "", fmt.Errorf("marshal rabbitmq lambda event: %w", err)
	}
	return string(raw), nil
}
