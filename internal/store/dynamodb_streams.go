package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrDynamoStreamNotFound     = errors.New("ResourceNotFoundException")
	ErrDynamoStreamExpiredIter  = errors.New("ExpiredIteratorException")
	ErrDynamoStreamInvalidShard = errors.New("ValidationException")
	ErrDynamoStreamBadView      = errors.New("ValidationException: unsupported StreamViewType")
)

const (
	// LabDynamoStreamShardID is the single shard for every lab table stream.
	LabDynamoStreamShardID = "shardId-000000000000"

	StreamViewNewImage = "NEW_IMAGE"
	StreamViewKeysOnly = "KEYS_ONLY"

	StreamStatusEnabled = "ENABLED"
)

const dynamodbStreamsSchema = `
CREATE TABLE IF NOT EXISTS dynamodb_stream_records (
  account_id TEXT NOT NULL,
  table_name TEXT NOT NULL,
  sequence_number TEXT NOT NULL,
  event_name TEXT NOT NULL,
  keys_json TEXT NOT NULL,
  new_image_json TEXT NOT NULL DEFAULT '',
  arrival_ms INTEGER NOT NULL,
  seq_ord INTEGER NOT NULL,
  PRIMARY KEY (account_id, table_name, sequence_number)
);
CREATE INDEX IF NOT EXISTS idx_ddb_stream_records_ord
  ON dynamodb_stream_records(account_id, table_name, seq_ord);
CREATE TABLE IF NOT EXISTS dynamodb_stream_iterators (
  iterator_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  table_name TEXT NOT NULL,
  shard_id TEXT NOT NULL,
  next_seq_ord INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);
`

// DynamoStreamRecord is one change record on a table stream.
type DynamoStreamRecord struct {
	SequenceNumber string
	EventName      string // INSERT, MODIFY, REMOVE
	KeysJSON       string
	NewImageJSON   string
	ArrivalMS      int64
	SeqOrd         int
}

// EnsureDynamoDBStreamsSchema adds stream columns and record/iterator tables.
func EnsureDynamoDBStreamsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure dynamodb streams schema: db is nil")
	}
	alters := []string{
		`ALTER TABLE dynamodb_tables ADD COLUMN stream_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE dynamodb_tables ADD COLUMN stream_view_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN stream_label TEXT NOT NULL DEFAULT ''`,
	}
	for _, q := range alters {
		if _, err := db.Exec(q); err != nil {
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "already exists") {
				return fmt.Errorf("ensure dynamodb streams schema: %w", err)
			}
		}
	}
	if _, err := db.Exec(dynamodbStreamsSchema); err != nil {
		return fmt.Errorf("ensure dynamodb streams schema: records: %w", err)
	}
	return nil
}

// EnsureDynamoDBStreamsSchema ensures stream tables on an open store.
func (s *Store) EnsureDynamoDBStreamsSchema() error {
	return EnsureDynamoDBStreamsSchema(s.db)
}

// DynamoStreamARN builds arn:aws:dynamodb:REGION:ACCOUNT:table/NAME/stream/LABEL
func DynamoStreamARN(region, accountID, tableName, label string) string {
	if region == "" {
		region = DefaultDynamoRegion
	}
	return fmt.Sprintf("arn:aws:dynamodb:%s:%s:table/%s/stream/%s", region, accountID, tableName, label)
}

// ParseDynamoStreamARN extracts account, table, and label from a stream ARN.
func ParseDynamoStreamARN(arn string) (accountID, tableName, label string, ok bool) {
	arn = strings.TrimSpace(arn)
	const prefix = "arn:aws:dynamodb:"
	if !strings.HasPrefix(arn, prefix) {
		return "", "", "", false
	}
	rest := strings.TrimPrefix(arn, prefix)
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) < 3 {
		return "", "", "", false
	}
	accountID = parts[1]
	resource := parts[2]
	if !strings.HasPrefix(resource, "table/") {
		return "", "", "", false
	}
	resource = strings.TrimPrefix(resource, "table/")
	idx := strings.Index(resource, "/stream/")
	if idx < 0 {
		return "", "", "", false
	}
	tableName = resource[:idx]
	label = resource[idx+len("/stream/"):]
	if accountID == "" || tableName == "" || label == "" {
		return "", "", "", false
	}
	return accountID, tableName, label, true
}

// UpdateTableStreamSpec enables or disables a table stream.
func (s *Store) UpdateTableStreamSpec(accountID, name string, enabled bool, viewType string) (DynamoTable, error) {
	table, err := s.GetTable(accountID, name)
	if err != nil {
		return DynamoTable{}, err
	}
	if enabled {
		viewType = strings.ToUpper(strings.TrimSpace(viewType))
		if viewType == "" {
			viewType = StreamViewNewImage
		}
		if viewType != StreamViewNewImage && viewType != StreamViewKeysOnly {
			return DynamoTable{}, ErrDynamoStreamBadView
		}
		label := table.StreamLabel
		if label == "" {
			label = time.Now().UTC().Format("2006-01-02T15:04:05.000")
		}
		_, err = s.db.Exec(
			`UPDATE dynamodb_tables SET stream_enabled = 1, stream_view_type = ?, stream_label = ?
			 WHERE account_id = ? AND table_name = ?`,
			viewType, label, accountID, name,
		)
		if err != nil {
			return DynamoTable{}, fmt.Errorf("update table stream: %w", err)
		}
	} else {
		_, err = s.db.Exec(
			`UPDATE dynamodb_tables SET stream_enabled = 0, stream_view_type = '', stream_label = ''
			 WHERE account_id = ? AND table_name = ?`,
			accountID, name,
		)
		if err != nil {
			return DynamoTable{}, fmt.Errorf("disable table stream: %w", err)
		}
	}
	return s.GetTable(accountID, name)
}

// AppendDynamoStreamRecord writes a change record when the table stream is enabled.
func (s *Store) AppendDynamoStreamRecord(accountID, tableName, eventName string, keysJSON, newImageJSON []byte) error {
	table, err := s.GetTable(accountID, tableName)
	if err != nil {
		return err
	}
	if !table.StreamEnabled {
		return nil
	}
	view := table.StreamViewType
	keys := string(keysJSON)
	newImg := ""
	if view == StreamViewNewImage && eventName != "REMOVE" {
		newImg = string(newImageJSON)
	}
	seqOrd, err := s.nextDynamoStreamSeqOrd(accountID, tableName)
	if err != nil {
		return err
	}
	now := time.Now().UTC().UnixMilli()
	seq := strconv.FormatInt(now, 10) + strconv.FormatInt(int64(seqOrd), 10)
	_, err = s.db.Exec(
		`INSERT INTO dynamodb_stream_records
		 (account_id, table_name, sequence_number, event_name, keys_json, new_image_json, arrival_ms, seq_ord)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, tableName, seq, eventName, keys, newImg, now, seqOrd,
	)
	if err != nil {
		return fmt.Errorf("append dynamodb stream record: %w", err)
	}
	return nil
}

// ListDynamoStreams lists enabled streams, optionally filtered by table name.
func (s *Store) ListDynamoStreams(accountID, tableName string) ([]DynamoTable, error) {
	q := `SELECT ` + dynamoTableSelect + ` FROM dynamodb_tables
	      WHERE account_id = ? AND stream_enabled = 1`
	args := []any{accountID}
	if strings.TrimSpace(tableName) != "" {
		q += ` AND table_name = ?`
		args = append(args, tableName)
	}
	q += ` ORDER BY table_name`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list dynamodb streams: %w", err)
	}
	defer rows.Close()
	out := []DynamoTable{}
	for rows.Next() {
		t, err := scanDynamoTable(rows)
		if err != nil {
			return nil, fmt.Errorf("list dynamodb streams: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DescribeDynamoStream returns stream metadata for an enabled table stream.
func (s *Store) DescribeDynamoStream(accountID, tableName, label string) (DynamoTable, error) {
	table, err := s.GetTable(accountID, tableName)
	if err != nil {
		return DynamoTable{}, err
	}
	if !table.StreamEnabled {
		return DynamoTable{}, ErrDynamoStreamNotFound
	}
	if label != "" && table.StreamLabel != label {
		return DynamoTable{}, ErrDynamoStreamNotFound
	}
	return table, nil
}

// GetDynamoStreamShardIterator creates a short-lived iterator.
func (s *Store) GetDynamoStreamShardIterator(accountID, tableName, shardID, iteratorType, startingSequenceNumber string) (string, error) {
	table, err := s.GetTable(accountID, tableName)
	if err != nil {
		return "", err
	}
	if !table.StreamEnabled {
		return "", ErrDynamoStreamNotFound
	}
	if shardID != "" && shardID != LabDynamoStreamShardID {
		return "", ErrDynamoStreamInvalidShard
	}
	nextOrd := 0
	switch strings.ToUpper(strings.TrimSpace(iteratorType)) {
	case "TRIM_HORIZON", "AT_SEQUENCE_NUMBER", "":
		if strings.EqualFold(iteratorType, "AT_SEQUENCE_NUMBER") {
			ord, err := s.dynamoStreamSeqOrdForSequence(accountID, tableName, startingSequenceNumber)
			if err != nil {
				return "", err
			}
			nextOrd = ord
		} else {
			nextOrd = 0
		}
	case "LATEST":
		max, err := s.maxDynamoStreamSeqOrd(accountID, tableName)
		if err != nil {
			return "", err
		}
		nextOrd = max + 1
	case "AFTER_SEQUENCE_NUMBER":
		ord, err := s.dynamoStreamSeqOrdForSequence(accountID, tableName, startingSequenceNumber)
		if err != nil {
			return "", err
		}
		nextOrd = ord + 1
	default:
		return "", fmt.Errorf("get dynamodb stream iterator: unsupported ShardIteratorType %q", iteratorType)
	}
	id := fmt.Sprintf("%s-%d", tableName, time.Now().UTC().UnixNano())
	expires := time.Now().UTC().Add(5 * time.Minute).Unix()
	_, err = s.db.Exec(
		`INSERT INTO dynamodb_stream_iterators (iterator_id, account_id, table_name, shard_id, next_seq_ord, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, accountID, tableName, LabDynamoStreamShardID, nextOrd, expires,
	)
	if err != nil {
		return "", fmt.Errorf("get dynamodb stream iterator: %w", err)
	}
	return id, nil
}

// GetDynamoStreamRecords reads records from an iterator and advances it.
func (s *Store) GetDynamoStreamRecords(iterator string, limit int) (records []DynamoStreamRecord, nextIterator string, err error) {
	iterator = strings.TrimSpace(iterator)
	if iterator == "" {
		return nil, "", fmt.Errorf("get dynamodb stream records: ShardIterator is required")
	}
	var accountID, name, shardID string
	var nextOrd int
	var expires int64
	err = s.db.QueryRow(
		`SELECT account_id, table_name, shard_id, next_seq_ord, expires_at FROM dynamodb_stream_iterators WHERE iterator_id = ?`,
		iterator,
	).Scan(&accountID, &name, &shardID, &nextOrd, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrDynamoStreamExpiredIter
	}
	if err != nil {
		return nil, "", fmt.Errorf("get dynamodb stream records: load iterator: %w", err)
	}
	if time.Now().UTC().Unix() > expires {
		_, _ = s.db.Exec(`DELETE FROM dynamodb_stream_iterators WHERE iterator_id = ?`, iterator)
		return nil, "", ErrDynamoStreamExpiredIter
	}
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query(
		`SELECT sequence_number, event_name, keys_json, new_image_json, arrival_ms, seq_ord
		 FROM dynamodb_stream_records
		 WHERE account_id = ? AND table_name = ? AND seq_ord >= ?
		 ORDER BY seq_ord ASC LIMIT ?`,
		accountID, name, nextOrd, limit,
	)
	if err != nil {
		return nil, "", fmt.Errorf("get dynamodb stream records: query: %w", err)
	}
	defer rows.Close()

	lastOrd := nextOrd - 1
	for rows.Next() {
		var rec DynamoStreamRecord
		if err := rows.Scan(&rec.SequenceNumber, &rec.EventName, &rec.KeysJSON, &rec.NewImageJSON, &rec.ArrivalMS, &rec.SeqOrd); err != nil {
			return nil, "", fmt.Errorf("get dynamodb stream records: scan: %w", err)
		}
		records = append(records, rec)
		lastOrd = rec.SeqOrd
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
		return nil, "", fmt.Errorf("get dynamodb stream records: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM dynamodb_stream_iterators WHERE iterator_id = ?`, iterator); err != nil {
		return nil, "", fmt.Errorf("get dynamodb stream records: delete old: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO dynamodb_stream_iterators (iterator_id, account_id, table_name, shard_id, next_seq_ord, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		nextID, accountID, name, shardID, newNext, newExpires,
	); err != nil {
		return nil, "", fmt.Errorf("get dynamodb stream records: insert next: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("get dynamodb stream records: commit: %w", err)
	}
	return records, nextID, nil
}

// DynamoStreamKeysJSON builds Keys AttributeValue map JSON from primary key strings.
func DynamoStreamKeysJSON(table DynamoTable, itemPK, itemSK string) ([]byte, error) {
	keys := map[string]any{
		table.HashKeyName: map[string]string{"S": itemPK},
	}
	if table.HasRangeKey() {
		keys[table.RangeKeyName] = map[string]string{"S": itemSK}
	}
	return json.Marshal(keys)
}

func (s *Store) nextDynamoStreamSeqOrd(accountID, name string) (int, error) {
	max, err := s.maxDynamoStreamSeqOrd(accountID, name)
	if err != nil {
		return 0, err
	}
	return max + 1, nil
}

func (s *Store) maxDynamoStreamSeqOrd(accountID, name string) (int, error) {
	var n sql.NullInt64
	err := s.db.QueryRow(
		`SELECT MAX(seq_ord) FROM dynamodb_stream_records WHERE account_id = ? AND table_name = ?`,
		accountID, name,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("max dynamodb stream seq: %w", err)
	}
	if !n.Valid {
		return -1, nil
	}
	return int(n.Int64), nil
}

func (s *Store) dynamoStreamSeqOrdForSequence(accountID, name, sequenceNumber string) (int, error) {
	var ord int
	err := s.db.QueryRow(
		`SELECT seq_ord FROM dynamodb_stream_records
		 WHERE account_id = ? AND table_name = ? AND sequence_number = ?`,
		accountID, name, sequenceNumber,
	).Scan(&ord)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrDynamoStreamInvalidShard
	}
	if err != nil {
		return 0, fmt.Errorf("dynamodb stream seq ord: %w", err)
	}
	return ord, nil
}
