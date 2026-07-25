package store

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrKinesisStreamExists   = errors.New("ResourceInUseException")
	ErrKinesisStreamNotFound = errors.New("ResourceNotFoundException")
	ErrKinesisInvalidShard   = errors.New("InvalidArgumentException")
	ErrKinesisExpiredIterator = errors.New("ExpiredIteratorException")
)

const (
	// DefaultKinesisRegion is the lab region embedded in Kinesis ARNs.
	DefaultKinesisRegion = "us-east-1"
)

// KinesisStream is a data stream row.
type KinesisStream struct {
	StreamName   string
	StreamARN    string
	StreamStatus string
	ShardCount   int
	CreatedAt    int64
}

// KinesisRecord is one put record on a lab shard.
type KinesisRecord struct {
	SequenceNumber              string
	PartitionKey                string
	Data                        []byte
	ApproximateArrivalTimestamp int64 // epoch millis
	ShardID                     string
}

const kinesisSchema = `
CREATE TABLE IF NOT EXISTS kinesis_streams (
  account_id TEXT NOT NULL,
  stream_name TEXT NOT NULL,
  stream_arn TEXT NOT NULL,
  stream_status TEXT NOT NULL,
  shard_count INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, stream_name)
);
CREATE TABLE IF NOT EXISTS kinesis_records (
  account_id TEXT NOT NULL,
  stream_name TEXT NOT NULL,
  sequence_number TEXT NOT NULL,
  partition_key TEXT NOT NULL,
  data BLOB NOT NULL,
  arrival_ms INTEGER NOT NULL,
  seq_ord INTEGER NOT NULL,
  shard_id TEXT NOT NULL DEFAULT 'shardId-000000000000',
  PRIMARY KEY (account_id, stream_name, sequence_number)
);
CREATE INDEX IF NOT EXISTS idx_kinesis_records_ord ON kinesis_records(account_id, stream_name, seq_ord);
CREATE INDEX IF NOT EXISTS idx_kinesis_records_shard ON kinesis_records(account_id, stream_name, shard_id, seq_ord);
CREATE TABLE IF NOT EXISTS kinesis_iterators (
  iterator_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  stream_name TEXT NOT NULL,
  shard_id TEXT NOT NULL,
  next_seq_ord INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);
`

// EnsureKinesisSchema creates Kinesis tables if missing.
func EnsureKinesisSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure kinesis schema: db is nil")
	}
	if _, err := db.Exec(kinesisSchema); err != nil {
		return fmt.Errorf("ensure kinesis schema: %w", err)
	}
	if err := execMigrateStmt(db,
		`ALTER TABLE kinesis_records ADD COLUMN shard_id TEXT NOT NULL DEFAULT 'shardId-000000000000'`,
		nil,
	); err != nil {
		return fmt.Errorf("ensure kinesis schema: shard_id: %w", err)
	}
	if _, err := db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_kinesis_records_shard ON kinesis_records(account_id, stream_name, shard_id, seq_ord)`,
	); err != nil {
		return fmt.Errorf("ensure kinesis schema: shard index: %w", err)
	}
	return nil
}

// EnsureKinesisSchema ensures kinesis tables on an open store.
func (s *Store) EnsureKinesisSchema() error {
	return EnsureKinesisSchema(s.db)
}

// KinesisStreamARN builds arn:aws:kinesis:REGION:ACCOUNT:stream/NAME
func KinesisStreamARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultKinesisRegion
	}
	return fmt.Sprintf("arn:aws:kinesis:%s:%s:stream/%s", region, accountID, name)
}

// CreateKinesisStream creates an ACTIVE stream with 1..MaxLabKinesisShardCount shards.
// ShardCount above the lab cap returns ErrKinesisInvalidShard (ValidationException at the API).
func (s *Store) CreateKinesisStream(accountID, region, name string, shardCount int) (KinesisStream, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return KinesisStream{}, fmt.Errorf("create kinesis stream: StreamName is required")
	}
	if shardCount <= 0 {
		shardCount = 1
	}
	if shardCount > MaxLabKinesisShardCount {
		return KinesisStream{}, fmt.Errorf("%w: ShardCount must be 1..%d", ErrKinesisInvalidShard, MaxLabKinesisShardCount)
	}
	if _, err := s.getKinesisStream(accountID, name); err == nil {
		return KinesisStream{}, ErrKinesisStreamExists
	} else if !errors.Is(err, ErrKinesisStreamNotFound) {
		return KinesisStream{}, err
	}
	now := time.Now().UTC().UnixMilli()
	arn := KinesisStreamARN(region, accountID, name)
	_, err := s.db.Exec(
		`INSERT INTO kinesis_streams (account_id, stream_name, stream_arn, stream_status, shard_count, created_at)
		 VALUES (?, ?, ?, 'ACTIVE', ?, ?)`,
		accountID, name, arn, shardCount, now,
	)
	if err != nil {
		return KinesisStream{}, fmt.Errorf("create kinesis stream: %w", err)
	}
	return KinesisStream{
		StreamName: name, StreamARN: arn, StreamStatus: "ACTIVE", ShardCount: shardCount, CreatedAt: now,
	}, nil
}

// DeleteKinesisStream deletes a stream and its records/iterators.
func (s *Store) DeleteKinesisStream(accountID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("delete kinesis stream: StreamName is required")
	}
	if _, err := s.getKinesisStream(accountID, name); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete kinesis stream: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM kinesis_records WHERE account_id = ? AND stream_name = ?`, accountID, name); err != nil {
		return fmt.Errorf("delete kinesis stream: records: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM kinesis_iterators WHERE account_id = ? AND stream_name = ?`, accountID, name); err != nil {
		return fmt.Errorf("delete kinesis stream: iterators: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM kinesis_streams WHERE account_id = ? AND stream_name = ?`, accountID, name); err != nil {
		return fmt.Errorf("delete kinesis stream: stream: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete kinesis stream: commit: %w", err)
	}
	return nil
}

// DescribeKinesisStream returns stream metadata.
func (s *Store) DescribeKinesisStream(accountID, name string) (KinesisStream, error) {
	return s.getKinesisStream(accountID, name)
}

// ListKinesisStreams lists stream names for an account.
func (s *Store) ListKinesisStreams(accountID, prefix string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT stream_name FROM kinesis_streams WHERE account_id = ? ORDER BY stream_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list kinesis streams: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("list kinesis streams: scan: %w", err)
		}
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// PutKinesisRecord appends one record and returns sequence number + shard id.
func (s *Store) PutKinesisRecord(accountID, name, partitionKey string, data []byte) (seq, shardID string, err error) {
	st, err := s.getKinesisStream(accountID, name)
	if err != nil {
		return "", "", err
	}
	partitionKey = strings.TrimSpace(partitionKey)
	if partitionKey == "" {
		return "", "", fmt.Errorf("put kinesis record: PartitionKey is required")
	}
	shardIdx := HashPartitionKeyToShard(partitionKey, st.ShardCount)
	shardID = LabKinesisShardID(shardIdx)
	seqOrd, err := s.nextKinesisSeqOrd(accountID, name)
	if err != nil {
		return "", "", err
	}
	now := time.Now().UTC().UnixMilli()
	seq = strconv.FormatInt(now, 10) + strconv.FormatInt(int64(seqOrd), 10)
	_, err = s.db.Exec(
		`INSERT INTO kinesis_records (account_id, stream_name, sequence_number, partition_key, data, arrival_ms, seq_ord, shard_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, seq, partitionKey, data, now, seqOrd, shardID,
	)
	if err != nil {
		return "", "", fmt.Errorf("put kinesis record: %w", err)
	}
	return seq, shardID, nil
}

// PutKinesisRecordsEntry is one PutRecords request entry.
type PutKinesisRecordsEntry struct {
	PartitionKey string
	Data         []byte
}

// PutKinesisRecordsResultEntry is one PutRecords result entry.
type PutKinesisRecordsResultEntry struct {
	SequenceNumber string
	ShardID        string
	ErrorCode      string
	ErrorMessage   string
}

// PutKinesisRecords appends a batch of records.
func (s *Store) PutKinesisRecords(accountID, name string, entries []PutKinesisRecordsEntry) ([]PutKinesisRecordsResultEntry, int, error) {
	if _, err := s.getKinesisStream(accountID, name); err != nil {
		return nil, 0, err
	}
	out := make([]PutKinesisRecordsResultEntry, 0, len(entries))
	failed := 0
	for _, e := range entries {
		seq, shard, err := s.PutKinesisRecord(accountID, name, e.PartitionKey, e.Data)
		if err != nil {
			failed++
			out = append(out, PutKinesisRecordsResultEntry{
				ErrorCode:    "InternalFailure",
				ErrorMessage: err.Error(),
			})
			continue
		}
		out = append(out, PutKinesisRecordsResultEntry{SequenceNumber: seq, ShardID: shard})
	}
	return out, failed, nil
}

// GetKinesisShardIterator creates a short-lived iterator. Types: TRIM_HORIZON, LATEST, AT_SEQUENCE_NUMBER.
func (s *Store) GetKinesisShardIterator(accountID, name, shardID, iteratorType, startingSequenceNumber string) (string, error) {
	st, err := s.getKinesisStream(accountID, name)
	if err != nil {
		return "", err
	}
	if !validLabKinesisShardID(shardID, st.ShardCount) {
		return "", ErrKinesisInvalidShard
	}
	shardID = normalizeLabKinesisShardID(shardID)
	nextOrd := 0
	switch strings.ToUpper(strings.TrimSpace(iteratorType)) {
	case "TRIM_HORIZON", "":
		nextOrd = 0
	case "LATEST":
		max, err := s.maxKinesisSeqOrdForShard(accountID, name, shardID)
		if err != nil {
			return "", err
		}
		nextOrd = max + 1
	case "AT_SEQUENCE_NUMBER", "AFTER_SEQUENCE_NUMBER":
		ord, recShard, err := s.seqOrdAndShardForSequence(accountID, name, startingSequenceNumber)
		if err != nil {
			return "", err
		}
		if recShard != shardID {
			return "", ErrKinesisInvalidShard
		}
		nextOrd = ord
		if strings.EqualFold(iteratorType, "AFTER_SEQUENCE_NUMBER") {
			nextOrd = ord + 1
		}
	default:
		return "", fmt.Errorf("get shard iterator: unsupported ShardIteratorType %q", iteratorType)
	}
	id := fmt.Sprintf("%s-%d", name, time.Now().UTC().UnixNano())
	expires := time.Now().UTC().Add(5 * time.Minute).Unix()
	_, err = s.db.Exec(
		`INSERT INTO kinesis_iterators (iterator_id, account_id, stream_name, shard_id, next_seq_ord, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, accountID, name, shardID, nextOrd, expires,
	)
	if err != nil {
		return "", fmt.Errorf("get shard iterator: %w", err)
	}
	return id, nil
}

// GetKinesisRecords reads records from an iterator and advances it.
func (s *Store) GetKinesisRecords(iterator string, limit int) (records []KinesisRecord, nextIterator string, err error) {
	iterator = strings.TrimSpace(iterator)
	if iterator == "" {
		return nil, "", fmt.Errorf("get records: ShardIterator is required")
	}
	var accountID, name, shardID string
	var nextOrd int
	var expires int64
	err = s.db.QueryRow(
		`SELECT account_id, stream_name, shard_id, next_seq_ord, expires_at FROM kinesis_iterators WHERE iterator_id = ?`,
		iterator,
	).Scan(&accountID, &name, &shardID, &nextOrd, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrKinesisExpiredIterator
	}
	if err != nil {
		return nil, "", fmt.Errorf("get records: load iterator: %w", err)
	}
	if time.Now().UTC().Unix() > expires {
		_, _ = s.db.Exec(`DELETE FROM kinesis_iterators WHERE iterator_id = ?`, iterator)
		return nil, "", ErrKinesisExpiredIterator
	}
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query(
		`SELECT sequence_number, partition_key, data, arrival_ms, seq_ord, COALESCE(shard_id, ?) FROM kinesis_records
		 WHERE account_id = ? AND stream_name = ? AND COALESCE(shard_id, ?) = ? AND seq_ord >= ?
		 ORDER BY seq_ord ASC LIMIT ?`,
		LabKinesisShardID(0), accountID, name, LabKinesisShardID(0), shardID, nextOrd, limit,
	)
	if err != nil {
		return nil, "", fmt.Errorf("get records: query: %w", err)
	}
	defer rows.Close()

	lastOrd := nextOrd - 1
	for rows.Next() {
		var rec KinesisRecord
		var ord int
		if err := rows.Scan(&rec.SequenceNumber, &rec.PartitionKey, &rec.Data, &rec.ApproximateArrivalTimestamp, &ord, &rec.ShardID); err != nil {
			return nil, "", fmt.Errorf("get records: scan: %w", err)
		}
		if rec.ShardID == "" {
			rec.ShardID = shardID
		}
		records = append(records, rec)
		lastOrd = ord
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	newNext := nextOrd
	if len(records) > 0 {
		newNext = lastOrd + 1
	}
	nextID := fmt.Sprintf("%s-%d", name, time.Now().UTC().UnixNano())
	newExpires := time.Now().UTC().Add(5 * time.Minute).Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, "", fmt.Errorf("get records: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM kinesis_iterators WHERE iterator_id = ?`, iterator); err != nil {
		return nil, "", fmt.Errorf("get records: delete old iterator: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO kinesis_iterators (iterator_id, account_id, stream_name, shard_id, next_seq_ord, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		nextID, accountID, name, shardID, newNext, newExpires,
	); err != nil {
		return nil, "", fmt.Errorf("get records: insert next iterator: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("get records: commit: %w", err)
	}
	return records, nextID, nil
}

func (s *Store) getKinesisStream(accountID, name string) (KinesisStream, error) {
	var st KinesisStream
	err := s.db.QueryRow(
		`SELECT stream_name, stream_arn, stream_status, shard_count, created_at FROM kinesis_streams
		 WHERE account_id = ? AND stream_name = ?`,
		accountID, name,
	).Scan(&st.StreamName, &st.StreamARN, &st.StreamStatus, &st.ShardCount, &st.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return KinesisStream{}, ErrKinesisStreamNotFound
	}
	if err != nil {
		return KinesisStream{}, fmt.Errorf("get kinesis stream: %w", err)
	}
	return st, nil
}

func (s *Store) nextKinesisSeqOrd(accountID, name string) (int, error) {
	max, err := s.maxKinesisSeqOrd(accountID, name)
	if err != nil {
		return 0, err
	}
	return max + 1, nil
}

func (s *Store) maxKinesisSeqOrd(accountID, name string) (int, error) {
	var n sql.NullInt64
	err := s.db.QueryRow(
		`SELECT MAX(seq_ord) FROM kinesis_records WHERE account_id = ? AND stream_name = ?`,
		accountID, name,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("max kinesis seq: %w", err)
	}
	if !n.Valid {
		return -1, nil
	}
	return int(n.Int64), nil
}

func (s *Store) maxKinesisSeqOrdForShard(accountID, name, shardID string) (int, error) {
	var n sql.NullInt64
	err := s.db.QueryRow(
		`SELECT MAX(seq_ord) FROM kinesis_records
		 WHERE account_id = ? AND stream_name = ? AND COALESCE(shard_id, ?) = ?`,
		accountID, name, LabKinesisShardID(0), shardID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("max kinesis seq for shard: %w", err)
	}
	if !n.Valid {
		return -1, nil
	}
	return int(n.Int64), nil
}

func (s *Store) seqOrdAndShardForSequence(accountID, name, sequenceNumber string) (int, string, error) {
	var ord int
	var shardID string
	err := s.db.QueryRow(
		`SELECT seq_ord, COALESCE(shard_id, ?) FROM kinesis_records
		 WHERE account_id = ? AND stream_name = ? AND sequence_number = ?`,
		LabKinesisShardID(0), accountID, name, sequenceNumber,
	).Scan(&ord, &shardID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrKinesisInvalidShard
	}
	if err != nil {
		return 0, "", fmt.Errorf("seq ord for sequence: %w", err)
	}
	return ord, shardID, nil
}

// DecodeKinesisData decodes base64 Data from JSON API payloads.
func DecodeKinesisData(v any) ([]byte, error) {
	switch d := v.(type) {
	case string:
		if d == "" {
			return nil, nil
		}
		b, err := base64.StdEncoding.DecodeString(d)
		if err != nil {
			return []byte(d), nil
		}
		return b, nil
	case []byte:
		return d, nil
	default:
		return nil, fmt.Errorf("unsupported data type")
	}
}
