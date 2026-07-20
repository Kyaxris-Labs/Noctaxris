package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrMQBrokerExists   = errors.New("ConflictException")
	ErrMQBrokerNotFound = errors.New("NotFoundException")
	ErrMQBadRequest     = errors.New("BadRequestException")
)

const DefaultMQRegion = "us-east-1"

const mqSchema = `
CREATE TABLE IF NOT EXISTS mq_brokers (
  account_id TEXT NOT NULL,
  broker_id TEXT NOT NULL,
  broker_name TEXT NOT NULL,
  broker_arn TEXT NOT NULL,
  engine_type TEXT NOT NULL,
  engine_version TEXT NOT NULL DEFAULT '',
  deployment_mode TEXT NOT NULL DEFAULT 'SINGLE_INSTANCE',
  broker_state TEXT NOT NULL,
  host_instance_type TEXT NOT NULL DEFAULT 'mq.t3.micro',
  stub_endpoint TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, broker_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_mq_brokers_name ON mq_brokers(account_id, broker_name);
`

// MQBroker is an Amazon MQ broker control-plane row (stub endpoint, no nested broker).
type MQBroker struct {
	BrokerID         string
	BrokerName       string
	BrokerARN        string
	EngineType       string
	EngineVersion    string
	DeploymentMode   string
	BrokerState      string
	HostInstanceType string
	StubEndpoint     string
	CreatedAt        int64
}

// EnsureMQSchema creates MQ tables if missing.
func EnsureMQSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure mq schema: db is nil")
	}
	if _, err := db.Exec(mqSchema); err != nil {
		return fmt.Errorf("ensure mq schema: %w", err)
	}
	return nil
}

// EnsureMQSchema ensures MQ tables on an open store.
func (s *Store) EnsureMQSchema() error {
	return EnsureMQSchema(s.db)
}

// MQBrokerARN builds arn:aws:mq:REGION:ACCOUNT:broker:NAME:ID
func MQBrokerARN(region, accountID, name, id string) string {
	if region == "" {
		region = DefaultMQRegion
	}
	return fmt.Sprintf("arn:aws:mq:%s:%s:broker:%s:%s", region, accountID, name, id)
}

// CreateMQBroker creates a control-plane broker with a documented stub endpoint.
func (s *Store) CreateMQBroker(accountID, region, name, engineType, engineVersion, deploymentMode, instanceType string) (MQBroker, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return MQBroker{}, fmt.Errorf("%w: BrokerName is required", ErrMQBadRequest)
	}
	engineType = strings.ToUpper(strings.TrimSpace(engineType))
	if engineType != "ACTIVEMQ" && engineType != "RABBITMQ" {
		return MQBroker{}, fmt.Errorf("%w: EngineType must be ACTIVEMQ or RABBITMQ", ErrMQBadRequest)
	}
	if deploymentMode == "" {
		deploymentMode = "SINGLE_INSTANCE"
	}
	if instanceType == "" {
		instanceType = "mq.t3.micro"
	}
	if engineVersion == "" {
		if engineType == "ACTIVEMQ" {
			engineVersion = "5.18"
		} else {
			engineVersion = "3.13"
		}
	}
	var existing string
	err := s.db.QueryRow(
		`SELECT broker_id FROM mq_brokers WHERE account_id = ? AND broker_name = ?`,
		accountID, name,
	).Scan(&existing)
	if err == nil {
		return MQBroker{}, ErrMQBrokerExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MQBroker{}, fmt.Errorf("create mq broker: %w", err)
	}
	id := uuid.NewString()
	arn := MQBrokerARN(region, accountID, name, id)
	// Loopback-only stub. No real broker process and no WAN publish.
	stub := fmt.Sprintf("stub://127.0.0.1/mq/%s", id)
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO mq_brokers
		 (account_id, broker_id, broker_name, broker_arn, engine_type, engine_version, deployment_mode, broker_state, host_instance_type, stub_endpoint, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'RUNNING', ?, ?, ?)`,
		accountID, id, name, arn, engineType, engineVersion, deploymentMode, instanceType, stub, now,
	)
	if err != nil {
		return MQBroker{}, fmt.Errorf("create mq broker: insert: %w", err)
	}
	return MQBroker{
		BrokerID: id, BrokerName: name, BrokerARN: arn,
		EngineType: engineType, EngineVersion: engineVersion, DeploymentMode: deploymentMode,
		BrokerState: "RUNNING", HostInstanceType: instanceType, StubEndpoint: stub, CreatedAt: now,
	}, nil
}

// DescribeMQBroker returns a broker by id.
func (s *Store) DescribeMQBroker(accountID, brokerID string) (MQBroker, error) {
	var b MQBroker
	err := s.db.QueryRow(
		`SELECT broker_id, broker_name, broker_arn, engine_type, engine_version, deployment_mode, broker_state, host_instance_type, stub_endpoint, created_at
		 FROM mq_brokers WHERE account_id = ? AND broker_id = ?`,
		accountID, brokerID,
	).Scan(&b.BrokerID, &b.BrokerName, &b.BrokerARN, &b.EngineType, &b.EngineVersion, &b.DeploymentMode, &b.BrokerState, &b.HostInstanceType, &b.StubEndpoint, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MQBroker{}, ErrMQBrokerNotFound
	}
	if err != nil {
		return MQBroker{}, fmt.Errorf("describe mq broker: %w", err)
	}
	return b, nil
}

// ListMQBrokers lists brokers for an account.
func (s *Store) ListMQBrokers(accountID string) ([]MQBroker, error) {
	rows, err := s.db.Query(
		`SELECT broker_id, broker_name, broker_arn, engine_type, engine_version, deployment_mode, broker_state, host_instance_type, stub_endpoint, created_at
		 FROM mq_brokers WHERE account_id = ? ORDER BY broker_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list mq brokers: %w", err)
	}
	defer rows.Close()
	out := []MQBroker{}
	for rows.Next() {
		var b MQBroker
		if err := rows.Scan(&b.BrokerID, &b.BrokerName, &b.BrokerARN, &b.EngineType, &b.EngineVersion, &b.DeploymentMode, &b.BrokerState, &b.HostInstanceType, &b.StubEndpoint, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("list mq brokers: scan: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// DeleteMQBroker deletes a broker by id.
func (s *Store) DeleteMQBroker(accountID, brokerID string) error {
	res, err := s.db.Exec(`DELETE FROM mq_brokers WHERE account_id = ? AND broker_id = ?`, accountID, brokerID)
	if err != nil {
		return fmt.Errorf("delete mq broker: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrMQBrokerNotFound
	}
	return nil
}
