package store

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/google/uuid"
)

var (
	ErrFirehoseExists   = errors.New("ResourceInUseException")
	ErrFirehoseNotFound = errors.New("ResourceNotFoundException")
	ErrFirehoseBadReq   = errors.New("InvalidArgumentException")
)

const DefaultFirehoseRegion = "us-east-1"

const actionESHttpPut = "es:ESHttpPut"

const (
	firehoseOSHostPrefix    = "noctaxris-opensearch-"
	firehoseOSHostPrefixAlt = "noctaxris-data-opensearch-"
	firehoseOSHTTPTimeout   = 10 * time.Second
)

const firehoseSchema = `
CREATE TABLE IF NOT EXISTS firehose_streams (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  stream_arn TEXT NOT NULL,
  dest_type TEXT NOT NULL,
  dest_bucket TEXT NOT NULL DEFAULT '',
  dest_prefix TEXT NOT NULL DEFAULT '',
  dest_lambda_arn TEXT NOT NULL DEFAULT '',
  dest_opensearch_domain TEXT NOT NULL DEFAULT '',
  dest_opensearch_index TEXT NOT NULL DEFAULT '',
  role_arn TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS firehose_records (
  account_id TEXT NOT NULL,
  stream_name TEXT NOT NULL,
  record_id TEXT NOT NULL,
  data BLOB NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (record_id)
);
CREATE INDEX IF NOT EXISTS idx_fh_records ON firehose_records(account_id, stream_name);
`

// FirehoseOpenSearchIndexer indexes one Firehose record into an OpenSearch destination.
// Tests inject this via SetFirehoseOpenSearchIndexer; when unset, Put uses the default nested HTTP indexer.
type FirehoseOpenSearchIndexer func(accountID string, stream FirehoseStream, recordID string, data []byte) error

var firehoseOSIndexerByStore sync.Map // *Store -> FirehoseOpenSearchIndexer

// FirehoseStream is a delivery stream row.
type FirehoseStream struct {
	Name                 string
	StreamARN            string
	DestType             string // S3, Lambda, or OpenSearch
	DestBucket           string
	DestPrefix           string
	DestLambdaARN        string
	DestOpenSearchDomain string
	DestOpenSearchIndex  string
	RoleARN              string
	CreatedAt            int64
}

// EnsureFirehoseSchema creates Firehose tables if missing and migrates columns.
func EnsureFirehoseSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure firehose schema: db is nil")
	}
	if _, err := db.Exec(firehoseSchema); err != nil {
		return fmt.Errorf("ensure firehose schema: %w", err)
	}
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE firehose_streams ADD COLUMN dest_opensearch_domain TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE firehose_streams ADD COLUMN dest_opensearch_index TEXT NOT NULL DEFAULT ''`,
	}); err != nil {
		return fmt.Errorf("ensure firehose schema: migrate: %w", err)
	}
	return nil
}

// EnsureFirehoseSchema ensures Firehose tables on an open store.
func (s *Store) EnsureFirehoseSchema() error {
	return EnsureFirehoseSchema(s.db)
}

// SetFirehoseOpenSearchIndexer registers an optional OpenSearch PutRecord hook for this store.
func (s *Store) SetFirehoseOpenSearchIndexer(fn FirehoseOpenSearchIndexer) {
	if fn == nil {
		firehoseOSIndexerByStore.Delete(s)
		return
	}
	firehoseOSIndexerByStore.Store(s, fn)
}

func (s *Store) firehoseOpenSearchIndexer() FirehoseOpenSearchIndexer {
	if v, ok := firehoseOSIndexerByStore.Load(s); ok {
		if fn, ok := v.(FirehoseOpenSearchIndexer); ok {
			return fn
		}
	}
	return nil
}

// GetOpenSearchDomain is the Firehose-facing alias for DescribeOpenSearchDomain.
func (s *Store) GetOpenSearchDomain(accountID, name string) (OpenSearchDomain, error) {
	return s.DescribeOpenSearchDomain(accountID, name)
}

// FirehoseStreamARN builds arn:aws:firehose:REGION:ACCOUNT:deliverystream/NAME
func FirehoseStreamARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultFirehoseRegion
	}
	return fmt.Sprintf("arn:aws:firehose:%s:%s:deliverystream/%s", region, accountID, name)
}

// CreateFirehoseStream creates a delivery stream with S3 and/or Lambda destination.
func (s *Store) CreateFirehoseStream(accountID, region, name, roleARN, destType, bucket, prefix, lambdaARN string) (FirehoseStream, error) {
	return s.createFirehoseStream(accountID, region, name, roleARN, destType, bucket, prefix, lambdaARN, "", "")
}

// CreateFirehoseOpenSearchStream creates a delivery stream targeting an Active OpenSearch domain.
func (s *Store) CreateFirehoseOpenSearchStream(accountID, region, name, roleARN, domainName, indexName string) (FirehoseStream, error) {
	return s.createFirehoseStream(accountID, region, name, roleARN, "OpenSearch", "", "", "", domainName, indexName)
}

func (s *Store) createFirehoseStream(
	accountID, region, name, roleARN, destType, bucket, prefix, lambdaARN, osDomain, osIndex string,
) (FirehoseStream, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return FirehoseStream{}, fmt.Errorf("%w: DeliveryStreamName required", ErrFirehoseBadReq)
	}
	destType = strings.TrimSpace(destType)
	switch strings.ToUpper(destType) {
	case "S3", "EXTENDED_S3", "":
		destType = "S3"
		bucket = strings.TrimSpace(bucket)
		if bucket == "" {
			return FirehoseStream{}, fmt.Errorf("%w: S3 BucketARN/Bucket required", ErrFirehoseBadReq)
		}
		if strings.HasPrefix(bucket, "arn:") {
			parts := strings.Split(bucket, ":::")
			if len(parts) == 2 {
				bucket = parts[1]
			}
		}
	case "LAMBDA":
		destType = "Lambda"
		lambdaARN = strings.TrimSpace(lambdaARN)
		if lambdaARN == "" {
			return FirehoseStream{}, fmt.Errorf("%w: Lambda ARN required", ErrFirehoseBadReq)
		}
	case "OPENSEARCH", "AMAZONOPENSEARCHSERVICE", "ELASTICSEARCH":
		destType = "OpenSearch"
		osDomain = strings.TrimSpace(osDomain)
		osIndex = strings.TrimSpace(osIndex)
		if osDomain == "" {
			return FirehoseStream{}, fmt.Errorf("%w: OpenSearch DomainName/DomainARN required", ErrFirehoseBadReq)
		}
		if osIndex == "" {
			return FirehoseStream{}, fmt.Errorf("%w: OpenSearch IndexName required", ErrFirehoseBadReq)
		}
		if err := s.validateFirehoseOpenSearchDest(accountID, osDomain); err != nil {
			return FirehoseStream{}, err
		}
	case "VPCFLOW", "VPC_FLOW_LOGS":
		destType = "VPCFlow"
		bucket = strings.TrimSpace(bucket)
		if bucket == "" {
			return FirehoseStream{}, fmt.Errorf("%w: VPC flow S3 BucketARN/Bucket required", ErrFirehoseBadReq)
		}
		if strings.HasPrefix(bucket, "arn:") {
			parts := strings.Split(bucket, ":::")
			if len(parts) == 2 {
				bucket = parts[1]
			}
		}
	default:
		return FirehoseStream{}, fmt.Errorf("%w: unsupported destination type %q", ErrFirehoseBadReq, destType)
	}
	arn := FirehoseStreamARN(region, accountID, name)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO firehose_streams (account_id, name, stream_arn, dest_type, dest_bucket, dest_prefix, dest_lambda_arn, dest_opensearch_domain, dest_opensearch_index, role_arn, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, arn, destType, bucket, prefix, lambdaARN, osDomain, osIndex, roleARN, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return FirehoseStream{}, ErrFirehoseExists
		}
		return FirehoseStream{}, fmt.Errorf("create delivery stream: %w", err)
	}
	return FirehoseStream{
		Name: name, StreamARN: arn, DestType: destType, DestBucket: bucket,
		DestPrefix: prefix, DestLambdaARN: lambdaARN,
		DestOpenSearchDomain: osDomain, DestOpenSearchIndex: osIndex,
		RoleARN: roleARN, CreatedAt: now,
	}, nil
}

func (s *Store) validateFirehoseOpenSearchDest(accountID, domainName string) error {
	d, err := s.GetOpenSearchDomain(accountID, domainName)
	if err != nil {
		if errors.Is(err, ErrOpenSearchDomainNotFound) {
			return fmt.Errorf("%w: OpenSearch domain %q not found", ErrFirehoseBadReq, domainName)
		}
		return fmt.Errorf("%w: OpenSearch domain lookup: %v", ErrFirehoseBadReq, err)
	}
	if d.DomainStatus != OpenSearchDomainStatusActive {
		return fmt.Errorf("%w: OpenSearch domain %q must be Active (got %s)", ErrFirehoseBadReq, domainName, d.DomainStatus)
	}
	endpoint := strings.TrimSpace(d.StubEndpoint)
	if endpoint == "" || strings.HasPrefix(endpoint, "stub://") {
		return fmt.Errorf("%w: OpenSearch domain %q has stub or empty endpoint", ErrFirehoseBadReq, domainName)
	}
	host, port, err := parseFirehoseOpenSearchEndpoint(endpoint)
	if err != nil {
		return fmt.Errorf("%w: OpenSearch domain %q endpoint refused", ErrFirehoseBadReq, domainName)
	}
	if err := firehoseValidateNestedOpenSearchHost(host); err != nil {
		return fmt.Errorf("%w: OpenSearch domain %q endpoint refused: %v", ErrFirehoseBadReq, domainName, err)
	}
	if port != OpenSearchNestedPort {
		return fmt.Errorf("%w: OpenSearch domain %q must use nested port %d", ErrFirehoseBadReq, domainName, OpenSearchNestedPort)
	}
	return nil
}

// GetFirehoseStream returns a delivery stream.
func (s *Store) GetFirehoseStream(accountID, name string) (FirehoseStream, error) {
	var st FirehoseStream
	err := s.db.QueryRow(
		`SELECT name, stream_arn, dest_type, dest_bucket, dest_prefix, dest_lambda_arn, dest_opensearch_domain, dest_opensearch_index, role_arn, created_at
		 FROM firehose_streams WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(
		&st.Name, &st.StreamARN, &st.DestType, &st.DestBucket, &st.DestPrefix, &st.DestLambdaARN,
		&st.DestOpenSearchDomain, &st.DestOpenSearchIndex, &st.RoleARN, &st.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return FirehoseStream{}, ErrFirehoseNotFound
	}
	if err != nil {
		return FirehoseStream{}, fmt.Errorf("get delivery stream: %w", err)
	}
	return st, nil
}

// ListFirehoseStreams lists delivery stream names.
func (s *Store) ListFirehoseStreams(accountID string) ([]FirehoseStream, error) {
	rows, err := s.db.Query(
		`SELECT name, stream_arn, dest_type, dest_bucket, dest_prefix, dest_lambda_arn, dest_opensearch_domain, dest_opensearch_index, role_arn, created_at
		 FROM firehose_streams WHERE account_id = ? ORDER BY created_at`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list delivery streams: %w", err)
	}
	defer rows.Close()
	var out []FirehoseStream
	for rows.Next() {
		var st FirehoseStream
		if err := rows.Scan(
			&st.Name, &st.StreamARN, &st.DestType, &st.DestBucket, &st.DestPrefix, &st.DestLambdaARN,
			&st.DestOpenSearchDomain, &st.DestOpenSearchIndex, &st.RoleARN, &st.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("list delivery streams scan: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// DeleteFirehoseStream deletes a delivery stream.
func (s *Store) DeleteFirehoseStream(accountID, name string) error {
	res, err := s.db.Exec(`DELETE FROM firehose_streams WHERE account_id = ? AND name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete delivery stream: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrFirehoseNotFound
	}
	_, _ = s.db.Exec(`DELETE FROM firehose_records WHERE account_id = ? AND stream_name = ?`, accountID, name)
	return nil
}

// PutFirehoseRecord delivers one record to S3 and/or enqueues a Lambda async invoke.
// Delivery requires RoleARN session Allow (like Scheduler) or a destination resource
// policy Allow for firehose.amazonaws.com when RoleARN is omitted.
// OpenSearch has no resource-policy surface yet: RoleARN is required (fail closed).
func (s *Store) PutFirehoseRecord(accountID, streamName string, data []byte) (recordID string, err error) {
	st, err := s.GetFirehoseStream(accountID, streamName)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("%w: Data required", ErrFirehoseBadReq)
	}
	if !s.firehoseDeliveryAuthorized(accountID, st) {
		return "", fmt.Errorf("%w: delivery not authorized for destination", ErrFirehoseBadReq)
	}
	recordID = uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO firehose_records (account_id, stream_name, record_id, data, created_at) VALUES (?, ?, ?, ?, ?)`,
		accountID, streamName, recordID, data, now,
	)
	if err != nil {
		return "", fmt.Errorf("put record insert: %w", err)
	}
	if st.DestType == "S3" && st.DestBucket != "" {
		prefix := st.DestPrefix
		if prefix != "" && !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		key := fmt.Sprintf("%s%s", prefix, recordID)
		_, err = s.PutObject(accountID, st.DestBucket, key, PutObjectMeta{
			Data: data, PlainSize: int64(len(data)), ContentType: "application/octet-stream",
		})
		if err != nil {
			return "", fmt.Errorf("put record s3: %w", err)
		}
	}
	if st.DestType == "VPCFlow" && st.DestBucket != "" {
		line, lineErr := LineFromFirehoseRecordData(accountID, data)
		if lineErr != nil {
			return "", fmt.Errorf("%w: %v", ErrFirehoseBadReq, lineErr)
		}
		body := []byte(line + "\n")
		key := VPCFlowDeliveryObjectKey(accountID, DefaultFirehoseRegion, st.DestPrefix, time.Now().UTC())
		if _, err = s.PutObject(accountID, st.DestBucket, key, PutObjectMeta{
			Data: body, PlainSize: int64(len(body)), ContentType: "text/plain",
		}); err != nil {
			return "", fmt.Errorf("put record vpc flow s3: %w", err)
		}
	}
	if st.DestType == "Lambda" && strings.TrimSpace(st.DestLambdaARN) != "" {
		if err := s.deliverFirehoseToLambda(accountID, st, recordID, data, now); err != nil {
			return "", err
		}
	}
	if st.DestType == "OpenSearch" && strings.TrimSpace(st.DestOpenSearchDomain) != "" {
		if err := s.deliverFirehoseToOpenSearch(accountID, st, recordID, data); err != nil {
			return "", err
		}
	}
	return recordID, nil
}

func (s *Store) firehoseDeliveryAuthorized(accountID string, st FirehoseStream) bool {
	if st.DestType == "OpenSearch" {
		targetARN, action, ok := firehoseDestinationTarget(accountID, st)
		if !ok {
			return false
		}
		// No OpenSearch resource-policy surface yet: require RoleARN session Allow (fail closed).
		if strings.TrimSpace(st.RoleARN) == "" {
			return false
		}
		return s.deliveryRoleSessionAllows(
			accountID, st.RoleARN, action, targetARN, "firehose-delivery", DefaultFirehoseRegion, st.StreamARN,
		)
	}
	targetARN, action, ok := firehoseDestinationTarget(accountID, st)
	if !ok {
		return false
	}
	return s.deliveryAuthorizedRoleAndResource(
		accountID, st.RoleARN, action, targetARN, authz.ServicePrincipalFirehose, st.StreamARN, "firehose-delivery", DefaultFirehoseRegion,
	)
}

func firehoseDestinationTarget(accountID string, st FirehoseStream) (targetARN, action string, ok bool) {
	switch st.DestType {
	case "S3", "VPCFlow":
		bucket := strings.TrimSpace(st.DestBucket)
		if bucket == "" {
			return "", "", false
		}
		return BucketARN(bucket), actionS3PutObject, true
	case "Lambda":
		arn := strings.TrimSpace(st.DestLambdaARN)
		if arn == "" {
			return "", "", false
		}
		return arn, actionLambdaInvokeFunction, true
	case "OpenSearch":
		domain := strings.TrimSpace(st.DestOpenSearchDomain)
		if domain == "" {
			return "", "", false
		}
		return OpenSearchDomainARN(DefaultFirehoseRegion, accountID, domain), actionESHttpPut, true
	default:
		return "", "", false
	}
}

func (s *Store) deliverFirehoseToLambda(accountID string, st FirehoseStream, recordID string, data []byte, arrivalMs int64) error {
	functionName, qualifier := ParseFunctionQualifier(st.DestLambdaARN)
	if functionName == "" {
		return fmt.Errorf("%w: Lambda destination function is empty", ErrFirehoseBadReq)
	}
	if _, _, err := s.ResolveFunction(accountID, functionName, qualifier); err != nil {
		return fmt.Errorf("put record lambda: %w", err)
	}
	eventJSON, err := firehoseLambdaEventJSON(st.StreamARN, recordID, data, arrivalMs)
	if err != nil {
		return fmt.Errorf("put record lambda event: %w", err)
	}
	if _, err := s.EnqueueAsyncInvoke(accountID, functionName, qualifier, eventJSON); err != nil {
		return fmt.Errorf("put record lambda invoke: %w", err)
	}
	return nil
}

func (s *Store) deliverFirehoseToOpenSearch(accountID string, st FirehoseStream, recordID string, data []byte) error {
	if indexer := s.firehoseOpenSearchIndexer(); indexer != nil {
		if err := indexer(accountID, st, recordID, data); err != nil {
			return fmt.Errorf("put record opensearch: %w", err)
		}
		return nil
	}
	if err := s.defaultFirehoseOpenSearchIndex(accountID, st, data); err != nil {
		return fmt.Errorf("put record opensearch: %w", err)
	}
	return nil
}

func (s *Store) defaultFirehoseOpenSearchIndex(accountID string, st FirehoseStream, data []byte) error {
	domainName := strings.TrimSpace(st.DestOpenSearchDomain)
	indexName := strings.TrimSpace(st.DestOpenSearchIndex)
	if domainName == "" || indexName == "" {
		return fmt.Errorf("%w: OpenSearch destination incomplete", ErrFirehoseBadReq)
	}
	if !safeFirehoseOpenSearchPathSegment(indexName) {
		return fmt.Errorf("%w: OpenSearch IndexName invalid", ErrFirehoseBadReq)
	}
	d, err := s.GetOpenSearchDomain(accountID, domainName)
	if err != nil {
		return err
	}
	if d.DomainStatus != OpenSearchDomainStatusActive {
		return fmt.Errorf("%w: OpenSearch domain not Active", ErrFirehoseBadReq)
	}
	endpoint := strings.TrimSpace(d.StubEndpoint)
	if endpoint == "" || strings.HasPrefix(endpoint, "stub://") {
		return fmt.Errorf("%w: OpenSearch domain has stub endpoint", ErrFirehoseBadReq)
	}
	host, port, err := parseFirehoseOpenSearchEndpoint(endpoint)
	if err != nil {
		return err
	}
	if err := firehoseValidateNestedOpenSearchHost(host); err != nil {
		return err
	}
	if port != OpenSearchNestedPort {
		return fmt.Errorf("refusing non-nested opensearch port %d", port)
	}
	targetURL := fmt.Sprintf("http://%s/%s/_doc", net.JoinHostPort(host, strconv.Itoa(port)), url.PathEscape(indexName))
	body := firehoseOpenSearchDocBody(data)
	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout: firehoseOSHTTPTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("stopped after %d redirects", len(via))
			}
			if err := firehoseValidateNestedOpenSearchHost(req.URL.Hostname()); err != nil {
				return fmt.Errorf("refusing redirect off nested network: %w", err)
			}
			p := req.URL.Port()
			if p == "" {
				p = "80"
			}
			portNum, aerr := strconv.Atoi(p)
			if aerr != nil || portNum != OpenSearchNestedPort {
				return fmt.Errorf("refusing redirect to non-nested opensearch port %q", p)
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("opensearch index status %d", resp.StatusCode)
	}
	return nil
}

func firehoseOpenSearchDocBody(data []byte) []byte {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && json.Valid(trimmed) {
		return trimmed
	}
	wrapped, err := json.Marshal(map[string]string{
		"data": base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return []byte(`{}`)
	}
	return wrapped
}

// firehoseValidateNestedOpenSearchHost mirrors the OpenSearch query-plane allowlist
// (noctaxris-opensearch-* / noctaxris-data-opensearch-*; no loopback/IP/WAN).
func firehoseValidateNestedOpenSearchHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("nested opensearch endpoint is empty")
	}
	lower := strings.ToLower(host)
	switch lower {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "*", "host.docker.internal":
		return fmt.Errorf("refusing non-nested opensearch host %q", host)
	}
	if strings.Contains(host, "/") || strings.Contains(host, "\\") {
		return fmt.Errorf("invalid nested opensearch host")
	}
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("refusing IP opensearch host %q (nested container DNS name required)", host)
	}
	if !strings.HasPrefix(lower, firehoseOSHostPrefix) && !strings.HasPrefix(lower, firehoseOSHostPrefixAlt) {
		return fmt.Errorf("nested opensearch host %q is not a data-plane endpoint", host)
	}
	suffix := strings.TrimPrefix(lower, firehoseOSHostPrefix)
	if strings.HasPrefix(lower, firehoseOSHostPrefixAlt) {
		suffix = strings.TrimPrefix(lower, firehoseOSHostPrefixAlt)
	}
	if suffix == "" {
		return fmt.Errorf("nested opensearch host %q is missing a domain suffix", host)
	}
	return nil
}

func parseFirehoseOpenSearchEndpoint(endpoint string) (host string, port int, err error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", 0, fmt.Errorf("empty opensearch endpoint")
	}
	if strings.Contains(endpoint, "://") {
		u, perr := url.Parse(endpoint)
		if perr != nil || u.Host == "" {
			return "", 0, fmt.Errorf("invalid opensearch endpoint")
		}
		endpoint = u.Host
	}
	h, p, perr := net.SplitHostPort(endpoint)
	if perr != nil {
		if firehoseValidateNestedOpenSearchHost(endpoint) == nil {
			return endpoint, OpenSearchNestedPort, nil
		}
		return "", 0, fmt.Errorf("invalid opensearch endpoint")
	}
	port, aerr := strconv.Atoi(p)
	if aerr != nil {
		return "", 0, fmt.Errorf("invalid opensearch port")
	}
	return h, port, nil
}

func safeFirehoseOpenSearchPathSegment(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	if strings.ContainsAny(s, `/\`) {
		return false
	}
	return true
}

func firehoseLambdaEventJSON(streamARN, recordID string, data []byte, arrivalMs int64) (string, error) {
	payload := map[string]any{
		"deliveryStreamArn":           streamARN,
		"recordId":                    recordID,
		"approximateArrivalTimestamp": arrivalMs,
		"data":                        base64.StdEncoding.EncodeToString(data),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// PutFirehoseRecordBatch puts multiple records. Returns failed indexes.
func (s *Store) PutFirehoseRecordBatch(accountID, streamName string, records [][]byte) (failed []int, err error) {
	if _, err := s.GetFirehoseStream(accountID, streamName); err != nil {
		return nil, err
	}
	for i, data := range records {
		if _, err := s.PutFirehoseRecord(accountID, streamName, data); err != nil {
			failed = append(failed, i)
		}
	}
	return failed, nil
}

// DecodeFirehoseData decodes base64 Firehose Data fields.
func DecodeFirehoseData(v any) ([]byte, error) {
	switch t := v.(type) {
	case string:
		return base64.StdEncoding.DecodeString(t)
	case []byte:
		return t, nil
	default:
		return nil, fmt.Errorf("data must be base64 string")
	}
}

// ParseFirehoseOpenSearchDomainRef extracts a domain name from DomainName or DomainARN.
func ParseFirehoseOpenSearchDomainRef(domainARN, domainName string) string {
	name := strings.TrimSpace(domainName)
	if name != "" {
		return name
	}
	arn := strings.TrimSpace(domainARN)
	const marker = ":domain/"
	if i := strings.Index(arn, marker); i >= 0 {
		return strings.TrimSpace(arn[i+len(marker):])
	}
	return ""
}
