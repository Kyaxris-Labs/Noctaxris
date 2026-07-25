package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrVPCFlowNotFound    = errors.New("InvalidFlowLogId.NotFound")
	ErrVPCFlowBadRequest  = errors.New("InvalidParameterValue")
)

const (
	VPCFlowLogFormatV2Default = "${version} ${account-id} ${interface-id} ${srcaddr} ${dstaddr} ${srcport} ${dstport} ${protocol} ${packets} ${bytes} ${start} ${end} ${action} ${log-status}"
	DefaultVPCFlowRegion    = "us-east-1"
)

const vpcFlowSchema = `
CREATE TABLE IF NOT EXISTS vpc_flow_logs (
  account_id TEXT NOT NULL,
  flow_log_id TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  traffic_type TEXT NOT NULL,
  log_destination_type TEXT NOT NULL,
  log_destination TEXT NOT NULL,
  log_format TEXT NOT NULL,
  deliver_logs_permission_arn TEXT NOT NULL DEFAULT '',
  log_group_name TEXT NOT NULL DEFAULT '',
  s3_bucket TEXT NOT NULL DEFAULT '',
  s3_prefix TEXT NOT NULL DEFAULT '',
  region TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, flow_log_id)
);
`

// VPCFlowLog is lab flow log metadata (no real VPC plane).
type VPCFlowLog struct {
	FlowLogID                 string
	ResourceID                string
	ResourceType              string
	TrafficType               string
	LogDestinationType        string
	LogDestination            string
	LogFormat                 string
	DeliverLogsPermissionArn  string
	LogGroupName              string
	S3Bucket                  string
	S3Prefix                  string
	Region                    string
	CreatedAt                 int64
}

// VPCFlowRecord is one custom-format v2 flow log line.
type VPCFlowRecord struct {
	Version     int
	AccountID   string
	InterfaceID string
	SrcAddr     string
	DstAddr     string
	SrcPort     int
	DstPort     int
	Protocol    int
	Packets     int64
	Bytes       int64
	Start       int64
	End         int64
	Action      string
	LogStatus   string
}

// CreateVPCFlowLogInput is CreateFlowLogs lite input.
type CreateVPCFlowLogInput struct {
	ResourceIDs              []string
	ResourceType             string
	TrafficType              string
	LogDestinationType       string
	LogDestination           string
	LogFormat                string
	DeliverLogsPermissionArn string
	Region                   string
}

// EnsureVPCFlowSchema creates VPC flow log tables if missing.
func EnsureVPCFlowSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure vpc flow schema: db is nil")
	}
	if _, err := db.Exec(vpcFlowSchema); err != nil {
		return fmt.Errorf("ensure vpc flow schema: %w", err)
	}
	return nil
}

func (s *Store) EnsureVPCFlowSchema() error {
	return EnsureVPCFlowSchema(s.db)
}

func opaqueEC2ID(prefix string) string {
	raw := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(raw) > 17 {
		raw = raw[:17]
	}
	return prefix + raw
}

func normalizeVPCFlowResourceType(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "VPC"
	}
	switch strings.ToUpper(v) {
	case "VPC", "SUBNET", "NETWORKINTERFACE":
		if strings.EqualFold(v, "subnet") {
			return "Subnet"
		}
		if strings.EqualFold(v, "networkinterface") {
			return "NetworkInterface"
		}
		return "VPC"
	default:
		return v
	}
}

func normalizeTrafficType(v string) string {
	v = strings.TrimSpace(strings.ToUpper(v))
	if v == "" {
		return "ALL"
	}
	switch v {
	case "ACCEPT", "REJECT", "ALL":
		return v
	default:
		return "ALL"
	}
}

func normalizeLogDestinationType(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "s3":
		return "s3"
	case "cloud-watch-logs", "cloudwatchlogs", "logs":
		return "cloud-watch-logs"
	default:
		return v
	}
}

func parseS3LogDestination(dest string) (bucket, prefix string, err error) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return "", "", fmt.Errorf("%w: LogDestination is required", ErrVPCFlowBadRequest)
	}
	if strings.HasPrefix(dest, "arn:aws:s3:::") {
		rest := strings.TrimPrefix(dest, "arn:aws:s3:::")
		rest = strings.TrimPrefix(rest, "/")
		if i := strings.Index(rest, "/"); i >= 0 {
			return rest[:i], strings.Trim(rest[i+1:], "/"), nil
		}
		return rest, "", nil
	}
	if strings.HasPrefix(dest, "s3://") {
		rest := strings.TrimPrefix(dest, "s3://")
		if i := strings.Index(rest, "/"); i >= 0 {
			return rest[:i], strings.Trim(rest[i+1:], "/"), nil
		}
		return rest, "", nil
	}
	return "", "", fmt.Errorf("%w: LogDestination must be an S3 ARN or s3:// URI", ErrVPCFlowBadRequest)
}

// CreateVPCFlowLog registers lab flow log metadata and opaque resource IDs.
func (s *Store) CreateVPCFlowLog(accountID string, in CreateVPCFlowLogInput) (VPCFlowLog, error) {
	if err := s.EnsureVPCFlowSchema(); err != nil {
		return VPCFlowLog{}, err
	}
	region := strings.TrimSpace(in.Region)
	if region == "" {
		region = DefaultVPCFlowRegion
	}
	destType := normalizeLogDestinationType(in.LogDestinationType)
	if destType != "s3" && destType != "cloud-watch-logs" {
		return VPCFlowLog{}, fmt.Errorf("%w: LogDestinationType must be s3 or cloud-watch-logs", ErrVPCFlowBadRequest)
	}
	logDest := strings.TrimSpace(in.LogDestination)
	if logDest == "" {
		return VPCFlowLog{}, fmt.Errorf("%w: LogDestination is required", ErrVPCFlowBadRequest)
	}
	logFormat := strings.TrimSpace(in.LogFormat)
	if logFormat == "" {
		logFormat = VPCFlowLogFormatV2Default
	}

	resourceIDs := in.ResourceIDs
	if len(resourceIDs) == 0 {
		resourceIDs = []string{opaqueEC2ID("vpc-")}
	}
	resourceID := strings.TrimSpace(resourceIDs[0])
	if resourceID == "" {
		resourceID = opaqueEC2ID("vpc-")
	}

	var logGroup, s3Bucket, s3Prefix string
	switch destType {
	case "s3":
		bucket, prefix, err := parseS3LogDestination(logDest)
		if err != nil {
			return VPCFlowLog{}, err
		}
		if _, err := s.GetBucket(accountID, bucket); err != nil {
			if errors.Is(err, ErrNoSuchBucket) {
				return VPCFlowLog{}, fmt.Errorf("%w: S3 bucket does not exist", ErrVPCFlowBadRequest)
			}
			return VPCFlowLog{}, fmt.Errorf("create vpc flow log: bucket: %w", err)
		}
		s3Bucket, s3Prefix = bucket, prefix
	case "cloud-watch-logs":
		group, err := parseLogGroupNameFromARN(logDest)
		if err != nil {
			return VPCFlowLog{}, fmt.Errorf("%w: LogDestination must be a CloudWatch Logs log group ARN", ErrVPCFlowBadRequest)
		}
		if _, err := s.getLogGroup(accountID, group); err != nil {
			if errors.Is(err, ErrLogGroupNotFound) {
				return VPCFlowLog{}, fmt.Errorf("%w: CloudWatch Logs log group does not exist", ErrVPCFlowBadRequest)
			}
			return VPCFlowLog{}, fmt.Errorf("create vpc flow log: log group: %w", err)
		}
		logGroup = group
	}

	flowLogID := opaqueEC2ID("fl-")
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO vpc_flow_logs (
			account_id, flow_log_id, resource_id, resource_type, traffic_type,
			log_destination_type, log_destination, log_format, deliver_logs_permission_arn,
			log_group_name, s3_bucket, s3_prefix, region, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, flowLogID, resourceID, normalizeVPCFlowResourceType(in.ResourceType),
		normalizeTrafficType(in.TrafficType), destType, logDest, logFormat,
		strings.TrimSpace(in.DeliverLogsPermissionArn),
		logGroup, s3Bucket, s3Prefix, region, now,
	)
	if err != nil {
		return VPCFlowLog{}, fmt.Errorf("create vpc flow log: %w", err)
	}
	return VPCFlowLog{
		FlowLogID:                flowLogID,
		ResourceID:               resourceID,
		ResourceType:             normalizeVPCFlowResourceType(in.ResourceType),
		TrafficType:              normalizeTrafficType(in.TrafficType),
		LogDestinationType:       destType,
		LogDestination:           logDest,
		LogFormat:                logFormat,
		DeliverLogsPermissionArn: strings.TrimSpace(in.DeliverLogsPermissionArn),
		LogGroupName:             logGroup,
		S3Bucket:                 s3Bucket,
		S3Prefix:                 s3Prefix,
		Region:                   region,
		CreatedAt:                now,
	}, nil
}

func (s *Store) getVPCFlowLog(accountID, flowLogID string) (VPCFlowLog, error) {
	if err := s.EnsureVPCFlowSchema(); err != nil {
		return VPCFlowLog{}, err
	}
	row := s.db.QueryRow(
		`SELECT flow_log_id, resource_id, resource_type, traffic_type, log_destination_type,
			log_destination, log_format, deliver_logs_permission_arn, log_group_name,
			s3_bucket, s3_prefix, region, created_at
		 FROM vpc_flow_logs WHERE account_id = ? AND flow_log_id = ?`,
		accountID, flowLogID,
	)
	var fl VPCFlowLog
	if err := row.Scan(
		&fl.FlowLogID, &fl.ResourceID, &fl.ResourceType, &fl.TrafficType, &fl.LogDestinationType,
		&fl.LogDestination, &fl.LogFormat, &fl.DeliverLogsPermissionArn, &fl.LogGroupName,
		&fl.S3Bucket, &fl.S3Prefix, &fl.Region, &fl.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return VPCFlowLog{}, ErrVPCFlowNotFound
		}
		return VPCFlowLog{}, fmt.Errorf("get vpc flow log: %w", err)
	}
	return fl, nil
}

// FormatVPCFlowLogV2Line renders one space-delimited custom format v2 record.
func FormatVPCFlowLogV2Line(rec VPCFlowRecord) string {
	version := rec.Version
	if version == 0 {
		version = 2
	}
	logStatus := rec.LogStatus
	if logStatus == "" {
		logStatus = "OK"
	}
	fields := []string{
		strconv.Itoa(version),
		rec.AccountID,
		rec.InterfaceID,
		rec.SrcAddr,
		rec.DstAddr,
		strconv.Itoa(rec.SrcPort),
		strconv.Itoa(rec.DstPort),
		strconv.Itoa(rec.Protocol),
		strconv.FormatInt(rec.Packets, 10),
		strconv.FormatInt(rec.Bytes, 10),
		strconv.FormatInt(rec.Start, 10),
		strconv.FormatInt(rec.End, 10),
		rec.Action,
		logStatus,
	}
	return strings.Join(fields, " ")
}

// LineFromFirehoseRecordData formats PutRecord payload as a VPC Flow Logs custom format v2 line.
func LineFromFirehoseRecordData(accountID string, data []byte) (string, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("empty record data")
	}
	parts := strings.Fields(string(trimmed))
	if len(parts) >= 14 && parts[0] == "2" {
		return string(trimmed), nil
	}
	var rec VPCFlowRecord
	if err := json.Unmarshal(trimmed, &rec); err == nil {
		if rec.AccountID == "" {
			rec.AccountID = accountID
		}
		return FormatVPCFlowLogV2Line(rec), nil
	}
	return "", fmt.Errorf("record must be a v2 flow line or JSON VPCFlowRecord")
}

// DefaultVPCFlowSampleRecords returns ACCEPT and REJECT sample lines for lab inject.
func DefaultVPCFlowSampleRecords(accountID, interfaceID string, at time.Time) []VPCFlowRecord {
	if interfaceID == "" {
		interfaceID = opaqueEC2ID("eni-")
	}
	start := at.Unix()
	end := start + 60
	return []VPCFlowRecord{
		{
			Version: 2, AccountID: accountID, InterfaceID: interfaceID,
			SrcAddr: "10.0.1.10", DstAddr: "10.0.2.20", SrcPort: 44322, DstPort: 443,
			Protocol: 6, Packets: 12, Bytes: 4096, Start: start, End: end,
			Action: "ACCEPT", LogStatus: "OK",
		},
		{
			Version: 2, AccountID: accountID, InterfaceID: interfaceID,
			SrcAddr: "203.0.113.50", DstAddr: "10.0.1.10", SrcPort: 51514, DstPort: 22,
			Protocol: 6, Packets: 3, Bytes: 180, Start: start + 5, End: end + 5,
			Action: "REJECT", LogStatus: "OK",
		},
	}
}

// VPCFlowDeliveryObjectKey builds an AWS-shaped VPC flow log S3 key.
func VPCFlowDeliveryObjectKey(accountID, region, prefix string, ts time.Time) string {
	yyyy, mm, dd := ts.UTC().Format("2006"), ts.UTC().Format("01"), ts.UTC().Format("02")
	ingest := ts.UTC().Unix()
	uniq := strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	base := fmt.Sprintf("AWSLogs/%s/vpcflowlogs/%s/%s/%s/%s/%s_vpcflowlogs_%s_%d_%s.log",
		accountID, region, yyyy, mm, dd, accountID, region, ingest, uniq)
	if prefix != "" {
		return strings.Trim(prefix, "/") + "/" + base
	}
	return base
}

// VPCFlowLabLogStreamName is the CloudWatch Logs stream used for lab delivery.
func VPCFlowLabLogStreamName(flowLogID string) string {
	return vpcFlowLabLogStreamName(flowLogID)
}

func vpcFlowLabLogStreamName(flowLogID string) string {
	id := strings.TrimPrefix(flowLogID, "fl-")
	if len(id) > 32 {
		id = id[:32]
	}
	return "vpcflow-" + id
}

// InjectVPCFlowLogs delivers CSV v2 lines to the flow log's configured destination.
func (s *Store) InjectVPCFlowLogs(accountID, flowLogID string, records []VPCFlowRecord, deliveredAt time.Time) (int, error) {
	fl, err := s.getVPCFlowLog(accountID, flowLogID)
	if err != nil {
		return 0, err
	}
	if len(records) == 0 {
		eni := fl.ResourceID
		if !strings.HasPrefix(eni, "eni-") {
			eni = opaqueEC2ID("eni-")
		}
		records = DefaultVPCFlowSampleRecords(accountID, eni, deliveredAt)
	}
	lines := make([]string, 0, len(records))
	for _, rec := range records {
		if rec.AccountID == "" {
			rec.AccountID = accountID
		}
		lines = append(lines, FormatVPCFlowLogV2Line(rec))
	}
	body := []byte(strings.Join(lines, "\n") + "\n")

	switch fl.LogDestinationType {
	case "s3":
		key := VPCFlowDeliveryObjectKey(accountID, fl.Region, fl.S3Prefix, deliveredAt)
		if _, err := s.PutObject(accountID, fl.S3Bucket, key, PutObjectMeta{
			Data:        body,
			PlainSize:   int64(len(body)),
			ContentType: "text/plain",
		}); err != nil {
			return 0, fmt.Errorf("vpc flow inject s3: %w", err)
		}
	case "cloud-watch-logs":
		group := fl.LogGroupName
		stream := vpcFlowLabLogStreamName(flowLogID)
		if _, err := s.getLogStream(accountID, group, stream); errors.Is(err, ErrLogStreamNotFound) {
			if _, err := s.CreateLogStream(accountID, fl.Region, group, stream); err != nil &&
				!errors.Is(err, ErrLogStreamAlreadyExists) {
				return 0, fmt.Errorf("vpc flow inject create stream: %w", err)
			}
		} else if err != nil {
			return 0, fmt.Errorf("vpc flow inject get stream: %w", err)
		}
		st, err := s.getLogStream(accountID, group, stream)
		if err != nil {
			return 0, fmt.Errorf("vpc flow inject get stream: %w", err)
		}
		events := make([]LogEvent, 0, len(lines))
		baseMillis := deliveredAt.UnixMilli()
		for i, line := range lines {
			events = append(events, LogEvent{
				Message:   line,
				Timestamp: baseMillis + int64(i),
			})
		}
		if _, _, err := s.PutLogEvents(accountID, group, stream, st.UploadSequenceToken, events); err != nil {
			return 0, fmt.Errorf("vpc flow inject put log events: %w", err)
		}
	default:
		return 0, fmt.Errorf("%w: unsupported LogDestinationType", ErrVPCFlowBadRequest)
	}
	return len(lines), nil
}
