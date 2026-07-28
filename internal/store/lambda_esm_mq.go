package store

import (
	"context"
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

	amqp10 "github.com/Azure/go-amqp"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	actionMQDescribeBroker = "mq:DescribeBroker"

	// LambdaESMLabMQQueue is the lab default destination queue for MQ ESM polls.
	LambdaESMLabMQQueue = "noctaxris"

	// MQLabAMQPUser / MQLabAMQPPassword are nested RabbitMQ credentials (DinD image env).
	// Endpoint URLs from MQNestedAMQPEndpoint omit userinfo; Rabbit ESM Dial uses these.
	MQLabAMQPUser     = "noctaxris"
	MQLabAMQPPassword = "noctaxris-mq-lab"

	mqLabHostPrefix = "noctaxris-mq-"

	mqESMDialTimeout = 2 * time.Second

	// activeMQAMQP10ReceiveWait is the per-message wait when the lab queue is empty.
	activeMQAMQP10ReceiveWait = 250 * time.Millisecond

	// activeMQLabQueueAddress is the ActiveMQ classic AMQP 1.0 source for the lab queue.
	activeMQLabQueueAddress = "queue://" + LambdaESMLabMQQueue
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
// Unit tests inject this to avoid DinD/broker. When nil, the poller uses engine-specific receive.
type MQReceiveFunc func(endpoint string, batchSize int) ([]MQESMMessage, error)

// MQDialFunc is an optional TCP dial override for unit tests (ActiveMQ AMQP 1.0 path).
// Production ActiveMQ path uses net.DialTimeout to the allowlisted nested host:port only,
// then negotiates AMQP 1.0 over that connection.
type MQDialFunc func(network, address string, timeout time.Duration) (net.Conn, error)

var (
	mqReceiveMu   sync.Mutex
	mqReceiveFunc MQReceiveFunc
	mqDialFunc    MQDialFunc
)

// SetMQReceiveFunc registers an injectable MQ receive hook for unit tests.
// Pass nil to restore the default engine-specific receive path.
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

// SetMQDialFunc registers an injectable TCP dial for unit tests of the ActiveMQ default receive path.
// Pass nil to restore net.DialTimeout. The dial address is still built only after ValidateNestedMQHost.
func SetMQDialFunc(fn MQDialFunc) {
	mqReceiveMu.Lock()
	defer mqReceiveMu.Unlock()
	mqDialFunc = fn
}

func getMQDialFunc() MQDialFunc {
	mqReceiveMu.Lock()
	defer mqReceiveMu.Unlock()
	return mqDialFunc
}

// MQDefaultReceiveEmptyReason explains empty default (non-injected) batches.
// RabbitMQ uses AMQP 0-9-1 basic.get (empty only when the queue has no messages).
// ActiveMQ uses AMQP 1.0 receive on queue://noctaxris (empty when no message arrives in wait).
func MQDefaultReceiveEmptyReason(engineType string) string {
	switch strings.ToUpper(strings.TrimSpace(engineType)) {
	case "RABBITMQ":
		return "RabbitMQ: empty batch when basic.get finds no messages on queue " + LambdaESMLabMQQueue
	case "ACTIVEMQ":
		return "ActiveMQ: empty batch when AMQP 1.0 receive finds no messages on " + activeMQLabQueueAddress
	default:
		return "empty batch after allowlisted dial with no messages"
	}
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
	msgs, err := s.receiveMQESMMessages(ep, b.EngineType, batchSize)
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

func (s *Store) receiveMQESMMessages(endpoint, engineType string, batchSize int) ([]MQESMMessage, error) {
	host, port, err := parseMQAMQPEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	if fn := getMQReceiveFunc(); fn != nil {
		return fn(endpoint, batchSize)
	}
	return defaultMQReceiveDial(host, port, engineType, batchSize)
}

// defaultMQReceiveDial receives from an allowlisted nested host only (no operator/WAN hosts).
//
//   - RABBITMQ: AMQP 0-9-1 Dial + basic.get on LambdaESMLabMQQueue (lab user MQLabAMQPUser).
//   - ACTIVEMQ: AMQP 1.0 Dial + receive on queue://noctaxris (nested classic connector on 5672).
//
// MQReceiveFunc injection takes precedence for unit tests.
func defaultMQReceiveDial(host string, port int, engineType string, batchSize int) ([]MQESMMessage, error) {
	if err := ValidateNestedMQHost(host); err != nil {
		return nil, err
	}
	switch strings.ToUpper(strings.TrimSpace(engineType)) {
	case "RABBITMQ":
		return rabbitMQBasicGet(host, port, batchSize)
	default:
		return activeMQAMQP10Receive(host, port, batchSize)
	}
}

// rabbitMQBasicGet dials AMQP 0-9-1 on the allowlisted host and Get+Acks up to batchSize
// messages from LambdaESMLabMQQueue. Dial/auth/protocol errors fail closed (returned, not silent empty).
func rabbitMQBasicGet(host string, port int, batchSize int) ([]MQESMMessage, error) {
	if err := ValidateNestedMQHost(host); err != nil {
		return nil, err
	}
	if batchSize <= 0 {
		batchSize = LambdaESMDefaultBatchSize
	}
	u := &url.URL{
		Scheme: "amqp",
		User:   url.UserPassword(MQLabAMQPUser, MQLabAMQPPassword),
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   "/",
	}
	cfg := amqp.Config{
		Dial:      amqp.DefaultDial(mqESMDialTimeout),
		Heartbeat: 5 * time.Second,
		Locale:    "en_US",
	}
	conn, err := amqp.DialConfig(u.String(), cfg)
	if err != nil {
		return nil, fmt.Errorf("mq esm rabbit dial %s: %w", net.JoinHostPort(host, strconv.Itoa(port)), err)
	}
	defer func() { _ = conn.Close() }()

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("mq esm rabbit channel: %w", err)
	}
	defer func() { _ = ch.Close() }()

	// Ensure lab queue exists so empty polls return [] (not NOT_FOUND). Durable, not exclusive.
	if _, err := ch.QueueDeclare(LambdaESMLabMQQueue, true, false, false, false, nil); err != nil {
		return nil, fmt.Errorf("mq esm rabbit queue declare: %w", err)
	}

	out := make([]MQESMMessage, 0, batchSize)
	for i := 0; i < batchSize; i++ {
		d, ok, gerr := ch.Get(LambdaESMLabMQQueue, false)
		if gerr != nil {
			return nil, fmt.Errorf("mq esm rabbit basic.get: %w", gerr)
		}
		if !ok {
			break
		}
		if aerr := d.Ack(false); aerr != nil {
			return nil, fmt.Errorf("mq esm rabbit ack: %w", aerr)
		}
		id := strings.TrimSpace(d.MessageId)
		if id == "" {
			id = fmt.Sprintf("lab-rmq-%d", i+1)
		}
		body := d.Body
		if body == nil {
			body = []byte{}
		}
		out = append(out, MQESMMessage{
			MessageID:   id,
			Data:        body,
			Destination: LambdaESMLabMQQueue,
			Redelivered: d.Redelivered,
		})
	}
	return out, nil
}

// activeMQAMQP10Receive dials the allowlisted host and receives up to batchSize messages
// via AMQP 1.0 from activeMQLabQueueAddress. Dial/protocol errors fail closed.
// An empty queue within the short receive wait returns an empty batch (not an error).
func activeMQAMQP10Receive(host string, port int, batchSize int) ([]MQESMMessage, error) {
	if err := ValidateNestedMQHost(host); err != nil {
		return nil, err
	}
	if batchSize <= 0 {
		batchSize = LambdaESMDefaultBatchSize
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dial := getMQDialFunc()
	var (
		netConn net.Conn
		err     error
	)
	if dial != nil {
		netConn, err = dial("tcp", addr, mqESMDialTimeout)
	} else {
		netConn, err = net.DialTimeout("tcp", addr, mqESMDialTimeout)
	}
	if err != nil {
		return nil, fmt.Errorf("mq esm activemq dial %s: %w", addr, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), mqESMDialTimeout+activeMQAMQP10ReceiveWait*time.Duration(batchSize+1))
	defer cancel()

	conn, err := amqp10.NewConn(ctx, netConn, &amqp10.ConnOptions{
		HostName:    host,
		SASLType:    amqp10.SASLTypeAnonymous(),
		IdleTimeout: -1,
	})
	if err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("mq esm activemq amqp1.0 conn %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	session, err := conn.NewSession(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mq esm activemq amqp1.0 session: %w", err)
	}
	defer func() { _ = session.Close(ctx) }()

	receiver, err := session.NewReceiver(ctx, activeMQLabQueueAddress, &amqp10.ReceiverOptions{
		Credit: int32(batchSize),
	})
	if err != nil {
		return nil, fmt.Errorf("mq esm activemq amqp1.0 receiver %s: %w", activeMQLabQueueAddress, err)
	}
	defer func() { _ = receiver.Close(ctx) }()

	out := make([]MQESMMessage, 0, batchSize)
	for i := 0; i < batchSize; i++ {
		recvCtx, recvCancel := context.WithTimeout(ctx, activeMQAMQP10ReceiveWait)
		msg, rerr := receiver.Receive(recvCtx, nil)
		recvCancel()
		if rerr != nil {
			if errors.Is(rerr, context.DeadlineExceeded) || errors.Is(rerr, context.Canceled) {
				break
			}
			// Empty / no credit left often surfaces as link errors after wait; treat as empty.
			if len(out) == 0 && isActiveMQEmptyReceive(rerr) {
				break
			}
			if len(out) > 0 && isActiveMQEmptyReceive(rerr) {
				break
			}
			return nil, fmt.Errorf("mq esm activemq amqp1.0 receive: %w", rerr)
		}
		if aerr := receiver.AcceptMessage(ctx, msg); aerr != nil {
			return nil, fmt.Errorf("mq esm activemq amqp1.0 accept: %w", aerr)
		}
		id := ""
		if msg.Properties != nil {
			switch mid := msg.Properties.MessageID.(type) {
			case string:
				id = strings.TrimSpace(mid)
			case []byte:
				id = strings.TrimSpace(string(mid))
			case uint64:
				id = strconv.FormatUint(mid, 10)
			}
		}
		if id == "" {
			id = fmt.Sprintf("lab-amq-%d", i+1)
		}
		body := msg.GetData()
		if body == nil {
			body = []byte{}
		}
		out = append(out, MQESMMessage{
			MessageID:   id,
			Data:        append([]byte(nil), body...),
			Destination: LambdaESMLabMQQueue,
			Redelivered: false,
		})
	}
	return out, nil
}

func isActiveMQEmptyReceive(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "timeout") ||
		strings.Contains(s, "deadline") ||
		strings.Contains(s, "no messages") ||
		strings.Contains(s, "link closed") ||
		strings.Contains(s, "connection closed")
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
			"messageID":     id,
			"messageType":   "jms/text-message",
			"deliveryMode":  1,
			"replyTo":       nil,
			"type":          nil,
			"expiration":    nil,
			"priority":      0,
			"correlationId": nil,
			"redelivered":   msg.Redelivered,
			"destination":   map[string]any{"physicalName": dest},
			"data":          base64.StdEncoding.EncodeToString(msg.Data),
			"timestamp":     now,
			"brokerInTime":  now,
			"brokerOutTime": now,
			"properties":    map[string]any{},
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
