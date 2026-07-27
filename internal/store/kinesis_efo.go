package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrKinesisConsumerExists   = errors.New("ResourceInUseException")
	ErrKinesisConsumerNotFound = errors.New("ResourceNotFoundException")
)

const maxLabKinesisConsumersPerStream = 5

// KinesisConsumer is a registered enhanced fan-out consumer.
type KinesisConsumer struct {
	ConsumerName             string
	ConsumerARN              string
	ConsumerStatus           string
	ConsumerCreationTimestamp int64 // epoch millis
	StreamARN                string
	StreamName               string
}

const kinesisConsumerSchema = `
CREATE TABLE IF NOT EXISTS kinesis_consumers (
  account_id TEXT NOT NULL,
  stream_name TEXT NOT NULL,
  consumer_name TEXT NOT NULL,
  consumer_arn TEXT NOT NULL,
  consumer_status TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, stream_name, consumer_name)
);
`

// EnsureKinesisConsumerSchema creates consumer tables if missing.
func EnsureKinesisConsumerSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure kinesis consumer schema: db is nil")
	}
	if _, err := db.Exec(kinesisConsumerSchema); err != nil {
		return fmt.Errorf("ensure kinesis consumer schema: %w", err)
	}
	return nil
}

func (s *Store) EnsureKinesisConsumerSchema() error {
	return EnsureKinesisConsumerSchema(s.db)
}

// KinesisConsumerARN builds arn:aws:kinesis:REGION:ACCOUNT:stream/NAME/consumer/CNAME:TIMESTAMP
func KinesisConsumerARN(region, accountID, streamName, consumerName string, createdAtMs int64) string {
	if region == "" {
		region = DefaultKinesisRegion
	}
	return fmt.Sprintf("arn:aws:kinesis:%s:%s:stream/%s/consumer/%s:%d",
		region, accountID, streamName, consumerName, createdAtMs)
}

// RegisterKinesisConsumer registers an ACTIVE lab EFO consumer on a stream.
func (s *Store) RegisterKinesisConsumer(accountID, region, streamName, consumerName string) (KinesisConsumer, error) {
	if err := s.EnsureKinesisConsumerSchema(); err != nil {
		return KinesisConsumer{}, err
	}
	streamName = strings.TrimSpace(streamName)
	consumerName = strings.TrimSpace(consumerName)
	if streamName == "" {
		return KinesisConsumer{}, fmt.Errorf("register consumer: StreamName is required")
	}
	if consumerName == "" {
		return KinesisConsumer{}, fmt.Errorf("register consumer: ConsumerName is required")
	}
	st, err := s.getKinesisStream(accountID, streamName)
	if err != nil {
		return KinesisConsumer{}, err
	}
	existing, err := s.ListKinesisConsumers(accountID, streamName)
	if err != nil {
		return KinesisConsumer{}, err
	}
	for _, c := range existing {
		if c.ConsumerName == consumerName {
			return KinesisConsumer{}, ErrKinesisConsumerExists
		}
	}
	if len(existing) >= maxLabKinesisConsumersPerStream {
		return KinesisConsumer{}, fmt.Errorf("%w: lab supports at most %d consumers per stream", ErrKinesisInvalidShard, maxLabKinesisConsumersPerStream)
	}
	now := time.Now().UTC().UnixMilli()
	arn := KinesisConsumerARN(region, accountID, streamName, consumerName, now)
	_, err = s.db.Exec(
		`INSERT INTO kinesis_consumers (account_id, stream_name, consumer_name, consumer_arn, consumer_status, created_at)
		 VALUES (?, ?, ?, ?, 'ACTIVE', ?)`,
		accountID, streamName, consumerName, arn, now,
	)
	if err != nil {
		return KinesisConsumer{}, fmt.Errorf("register consumer: %w", err)
	}
	return KinesisConsumer{
		ConsumerName:              consumerName,
		ConsumerARN:               arn,
		ConsumerStatus:            "ACTIVE",
		ConsumerCreationTimestamp: now,
		StreamARN:                 st.StreamARN,
		StreamName:                streamName,
	}, nil
}

// DescribeKinesisConsumer returns a consumer by name or ARN.
func (s *Store) DescribeKinesisConsumer(accountID, streamName, consumerName, consumerARN string) (KinesisConsumer, error) {
	if err := s.EnsureKinesisConsumerSchema(); err != nil {
		return KinesisConsumer{}, err
	}
	consumerARN = strings.TrimSpace(consumerARN)
	consumerName = strings.TrimSpace(consumerName)
	streamName = strings.TrimSpace(streamName)
	if consumerARN != "" {
		var c KinesisConsumer
		err := s.db.QueryRow(
			`SELECT consumer_name, consumer_arn, consumer_status, created_at, stream_name FROM kinesis_consumers
			 WHERE account_id = ? AND consumer_arn = ?`,
			accountID, consumerARN,
		).Scan(&c.ConsumerName, &c.ConsumerARN, &c.ConsumerStatus, &c.ConsumerCreationTimestamp, &c.StreamName)
		if errors.Is(err, sql.ErrNoRows) {
			return KinesisConsumer{}, ErrKinesisConsumerNotFound
		}
		if err != nil {
			return KinesisConsumer{}, fmt.Errorf("describe consumer: %w", err)
		}
		st, err := s.getKinesisStream(accountID, c.StreamName)
		if err != nil {
			return KinesisConsumer{}, err
		}
		c.StreamARN = st.StreamARN
		return c, nil
	}
	if streamName == "" || consumerName == "" {
		return KinesisConsumer{}, fmt.Errorf("describe consumer: ConsumerARN or StreamName+ConsumerName required")
	}
	if _, err := s.getKinesisStream(accountID, streamName); err != nil {
		return KinesisConsumer{}, err
	}
	var c KinesisConsumer
	err := s.db.QueryRow(
		`SELECT consumer_name, consumer_arn, consumer_status, created_at, stream_name FROM kinesis_consumers
		 WHERE account_id = ? AND stream_name = ? AND consumer_name = ?`,
		accountID, streamName, consumerName,
	).Scan(&c.ConsumerName, &c.ConsumerARN, &c.ConsumerStatus, &c.ConsumerCreationTimestamp, &c.StreamName)
	if errors.Is(err, sql.ErrNoRows) {
		return KinesisConsumer{}, ErrKinesisConsumerNotFound
	}
	if err != nil {
		return KinesisConsumer{}, fmt.Errorf("describe consumer: %w", err)
	}
	st, err := s.getKinesisStream(accountID, streamName)
	if err != nil {
		return KinesisConsumer{}, err
	}
	c.StreamARN = st.StreamARN
	return c, nil
}

// ListKinesisConsumers lists consumers for a stream.
func (s *Store) ListKinesisConsumers(accountID, streamName string) ([]KinesisConsumer, error) {
	if err := s.EnsureKinesisConsumerSchema(); err != nil {
		return nil, err
	}
	st, err := s.getKinesisStream(accountID, streamName)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT consumer_name, consumer_arn, consumer_status, created_at FROM kinesis_consumers
		 WHERE account_id = ? AND stream_name = ? ORDER BY consumer_name`,
		accountID, streamName,
	)
	if err != nil {
		return nil, fmt.Errorf("list consumers: %w", err)
	}
	defer rows.Close()
	var out []KinesisConsumer
	for rows.Next() {
		var c KinesisConsumer
		if err := rows.Scan(&c.ConsumerName, &c.ConsumerARN, &c.ConsumerStatus, &c.ConsumerCreationTimestamp); err != nil {
			return nil, fmt.Errorf("list consumers: scan: %w", err)
		}
		c.StreamName = streamName
		c.StreamARN = st.StreamARN
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeregisterKinesisConsumer removes a consumer by name or ARN.
func (s *Store) DeregisterKinesisConsumer(accountID, streamName, consumerName, consumerARN string) error {
	if err := s.EnsureKinesisConsumerSchema(); err != nil {
		return err
	}
	c, err := s.DescribeKinesisConsumer(accountID, streamName, consumerName, consumerARN)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`DELETE FROM kinesis_consumers WHERE account_id = ? AND stream_name = ? AND consumer_name = ?`,
		accountID, c.StreamName, c.ConsumerName,
	)
	if err != nil {
		return fmt.Errorf("deregister consumer: %w", err)
	}
	return nil
}

// UpdateKinesisShardCount sets shard count within 1..MaxLabKinesisShardCount (lab Immediate scaling).
func (s *Store) UpdateKinesisShardCount(accountID, name string, targetShardCount int) (KinesisStream, error) {
	st, err := s.getKinesisStream(accountID, name)
	if err != nil {
		return KinesisStream{}, err
	}
	if targetShardCount < 1 || targetShardCount > MaxLabKinesisShardCount {
		return KinesisStream{}, fmt.Errorf("%w: TargetShardCount must be 1..%d", ErrKinesisInvalidShard, MaxLabKinesisShardCount)
	}
	if targetShardCount == st.ShardCount {
		return st, nil
	}
	_, err = s.db.Exec(
		`UPDATE kinesis_streams SET shard_count = ? WHERE account_id = ? AND stream_name = ?`,
		targetShardCount, accountID, name,
	)
	if err != nil {
		return KinesisStream{}, fmt.Errorf("update shard count: %w", err)
	}
	st.ShardCount = targetShardCount
	return st, nil
}

// SubscribeToShardLab returns records for a consumer subscription using the same
// iterator path as GetRecords (HTTP long-poll stand-in; no HTTP/2 event stream).
func (s *Store) SubscribeToShardLab(accountID, consumerARN, shardID, iteratorType, startingSequenceNumber string, limit int) (
	consumer KinesisConsumer, records []KinesisRecord, continuation string, err error,
) {
	consumer, err = s.DescribeKinesisConsumer(accountID, "", "", consumerARN)
	if err != nil {
		return KinesisConsumer{}, nil, "", err
	}
	it, err := s.GetKinesisShardIterator(accountID, consumer.StreamName, shardID, iteratorType, startingSequenceNumber)
	if err != nil {
		return KinesisConsumer{}, nil, "", err
	}
	records, nextIt, err := s.GetKinesisRecords(it, limit)
	if err != nil {
		return KinesisConsumer{}, nil, "", err
	}
	continuation = nextIt
	if len(records) > 0 {
		continuation = records[len(records)-1].SequenceNumber
	}
	return consumer, records, continuation, nil
}
