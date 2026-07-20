package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrTextractInvalidParameter = errors.New("InvalidParameterException")
	ErrTextractInvalidS3Object  = errors.New("InvalidS3ObjectException")
)

const DefaultTextractRegion = "us-east-1"

const textractSchema = `
CREATE TABLE IF NOT EXISTS textract_detections (
  account_id TEXT NOT NULL,
  detection_id TEXT NOT NULL,
  source_kind TEXT NOT NULL,
  source_ref TEXT NOT NULL DEFAULT '',
  feature_types TEXT NOT NULL DEFAULT '',
  blocks_json TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, detection_id)
);
`

// TextractDetection is a persisted DetectDocumentText / AnalyzeDocument stub record.
type TextractDetection struct {
	DetectionID  string
	SourceKind   string
	SourceRef    string
	FeatureTypes string
	BlocksJSON   string
	CreatedAt    int64
}

// EnsureTextractSchema creates Textract tables if missing.
func EnsureTextractSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure textract schema: db is nil")
	}
	if _, err := db.Exec(textractSchema); err != nil {
		return fmt.Errorf("ensure textract schema: %w", err)
	}
	return nil
}

// EnsureTextractSchema ensures Textract tables on an open store.
func (s *Store) EnsureTextractSchema() error {
	return EnsureTextractSchema(s.db)
}

// TextractCannedBlocks returns a deterministic PAGE/LINE/WORD Block list.
func TextractCannedBlocks() []map[string]any {
	return []map[string]any{
		{
			"BlockType": "PAGE",
			"Id":        "page-1",
			"Page":      1,
			"Relationships": []map[string]any{
				{"Type": "CHILD", "Ids": []string{"line-1"}},
			},
		},
		{
			"BlockType":  "LINE",
			"Id":         "line-1",
			"Text":       "Noctaxris stub text",
			"Confidence": 99.0,
			"Page":       1,
			"Relationships": []map[string]any{
				{"Type": "CHILD", "Ids": []string{"word-1", "word-2", "word-3"}},
			},
		},
		{
			"BlockType":  "WORD",
			"Id":         "word-1",
			"Text":       "Noctaxris",
			"Confidence": 99.0,
			"Page":       1,
			"TextType":   "PRINTED",
		},
		{
			"BlockType":  "WORD",
			"Id":         "word-2",
			"Text":       "stub",
			"Confidence": 99.0,
			"Page":       1,
			"TextType":   "PRINTED",
		},
		{
			"BlockType":  "WORD",
			"Id":         "word-3",
			"Text":       "text",
			"Confidence": 99.0,
			"Page":       1,
			"TextType":   "PRINTED",
		},
	}
}

// DetectDocumentTextStub validates Document input and returns canned Blocks.
// Bytes or S3Object (Bucket+Name) is required. No real OCR.
func (s *Store) DetectDocumentTextStub(accountID string, hasBytes bool, bucket, key string) (TextractDetection, error) {
	return s.textractDetect(accountID, hasBytes, bucket, key, "")
}

// AnalyzeDocumentStub is DetectDocumentText plus optional FeatureTypes persistence.
func (s *Store) AnalyzeDocumentStub(accountID string, hasBytes bool, bucket, key string, featureTypes []string) (TextractDetection, error) {
	ft := strings.Join(featureTypes, ",")
	return s.textractDetect(accountID, hasBytes, bucket, key, ft)
}

func (s *Store) textractDetect(accountID string, hasBytes bool, bucket, key string, featureTypes string) (TextractDetection, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	sourceKind := ""
	sourceRef := ""
	switch {
	case hasBytes:
		sourceKind = "Bytes"
	case bucket != "" && key != "":
		sourceKind = "S3Object"
		sourceRef = bucket + "/" + key
		if _, err := s.HeadObject(accountID, bucket, key); err != nil {
			return TextractDetection{}, fmt.Errorf("%w: unable to access s3://%s/%s", ErrTextractInvalidS3Object, bucket, key)
		}
	default:
		return TextractDetection{}, fmt.Errorf("%w: Document must include Bytes or S3Object", ErrTextractInvalidParameter)
	}
	blocks, err := json.Marshal(TextractCannedBlocks())
	if err != nil {
		return TextractDetection{}, fmt.Errorf("textract detect: marshal blocks: %w", err)
	}
	id := uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO textract_detections
		 (account_id, detection_id, source_kind, source_ref, feature_types, blocks_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, id, sourceKind, sourceRef, featureTypes, string(blocks), now,
	)
	if err != nil {
		return TextractDetection{}, fmt.Errorf("textract detect: insert: %w", err)
	}
	return TextractDetection{
		DetectionID:  id,
		SourceKind:   sourceKind,
		SourceRef:    sourceRef,
		FeatureTypes: featureTypes,
		BlocksJSON:   string(blocks),
		CreatedAt:    now,
	}, nil
}
