package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CFNStackDriftDetection is a drift detection job row.
type CFNStackDriftDetection struct {
	StackDriftDetectionID string
	StackID               string
	StackDriftStatus      string
	DetectionStatus       string
	Timestamp             int64
}

// CFNResourceDrift is one resource drift result.
type CFNResourceDrift struct {
	LogicalResourceID  string
	ResourceType       string
	PhysicalResourceID string
	StackResourceDriftStatus string
}

// DetectCFNStackDrift compares live resources to the stack template for allowlisted types.
func (s *Store) DetectCFNStackDrift(accountID, stackNameOrID string) (CFNStackDriftDetection, error) {
	stacks, err := s.DescribeCFNStacks(accountID, stackNameOrID)
	if err != nil {
		return CFNStackDriftDetection{}, err
	}
	st := stacks[0]
	tpl, err := parseCFNTemplate(st.TemplateBody)
	if err != nil {
		return CFNStackDriftDetection{}, err
	}
	_, _ = s.db.Exec(`DELETE FROM cfn_resource_drift WHERE stack_id = ?`, st.StackID)
	stackDrift := "IN_SYNC"
	for _, res := range st.Resources {
		status := "NOT_CHECKED"
		expected, ok := tpl.Resources[res.LogicalID]
		if !ok {
			status = "DELETED"
			stackDrift = "DRIFTED"
		} else if res.ResourceType == "AWS::CloudFormation::Stack" {
			status = "NOT_CHECKED"
		} else {
			status = s.cfnLiveDriftStatus(accountID, res, expected)
			if status == "MODIFIED" {
				stackDrift = "DRIFTED"
			}
		}
		_, err = s.db.Exec(
			`INSERT INTO cfn_resource_drift (stack_id, logical_id, resource_type, physical_id, drift_status)
			 VALUES (?, ?, ?, ?, ?)`,
			st.StackID, res.LogicalID, res.ResourceType, res.PhysicalID, status,
		)
		if err != nil {
			return CFNStackDriftDetection{}, fmt.Errorf("insert resource drift: %w", err)
		}
	}
	id := uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO cfn_stack_drift (detection_id, account_id, stack_id, stack_drift_status, detection_status, created_at)
		 VALUES (?, ?, ?, ?, 'DETECTION_COMPLETE', ?)`,
		id, accountID, st.StackID, stackDrift, now,
	)
	if err != nil {
		return CFNStackDriftDetection{}, fmt.Errorf("insert stack drift: %w", err)
	}
	return CFNStackDriftDetection{
		StackDriftDetectionID: id, StackID: st.StackID, StackDriftStatus: stackDrift,
		DetectionStatus: "DETECTION_COMPLETE", Timestamp: now,
	}, nil
}

func (s *Store) cfnLiveDriftStatus(accountID string, res CFNStackResource, expected cfnResource) string {
	switch res.ResourceType {
	case "AWS::S3::Bucket":
		if _, err := s.GetBucket(accountID, res.PhysicalID); err != nil {
			return "DELETED"
		}
		want := cfnStringProp(expected.Properties, "BucketName")
		if want != "" && want != res.PhysicalID {
			return "MODIFIED"
		}
		return "IN_SYNC"
	case "AWS::IAM::Role":
		name := res.PhysicalID
		if _, n, ok := parseIAMRoleARN(res.PhysicalID); ok {
			name = n
		}
		if _, err := s.GetRoleRecord(accountID, name); err != nil {
			return "DELETED"
		}
		want := cfnStringProp(expected.Properties, "RoleName")
		if want != "" && want != name {
			return "MODIFIED"
		}
		return "IN_SYNC"
	case "AWS::SQS::Queue":
		q, err := s.GetQueueByURL(res.PhysicalID)
		if err != nil {
			return "DELETED"
		}
		want := cfnStringProp(expected.Properties, "QueueName")
		if want != "" && want != q.QueueName {
			return "MODIFIED"
		}
		return "IN_SYNC"
	case "AWS::DynamoDB::Table":
		if _, err := s.GetTable(accountID, res.PhysicalID); err != nil {
			return "DELETED"
		}
		return "IN_SYNC"
	case "AWS::Lambda::Function":
		if _, err := s.GetFunction(accountID, res.PhysicalID); err != nil {
			return "DELETED"
		}
		return "IN_SYNC"
	case "AWS::SSM::Parameter":
		p, err := s.GetParameter(accountID, res.PhysicalID, true)
		if err != nil {
			return "DELETED"
		}
		want := cfnStringProp(expected.Properties, "Value")
		if want != "" && want != p.Value {
			return "MODIFIED"
		}
		return "IN_SYNC"
	case "AWS::IAM::ManagedPolicy", "AWS::IAM::Policy":
		if _, err := s.GetManagedPolicy(res.PhysicalID); err != nil {
			return "DELETED"
		}
		return "IN_SYNC"
	case "AWS::S3::BucketPolicy":
		bucket := strings.TrimSuffix(res.PhysicalID, "#BucketPolicy")
		if _, err := s.GetBucketPolicy(accountID, bucket); err != nil {
			return "DELETED"
		}
		return "IN_SYNC"
	case "AWS::Lambda::Permission":
		fn, sid, ok := splitCFNLambdaPermissionPhysical(res.PhysicalID)
		if !ok {
			return "NOT_CHECKED"
		}
		pol, err := s.GetFunctionPolicy(accountID, fn)
		if err != nil {
			return "DELETED"
		}
		if !strings.Contains(pol, `"Sid":"`+sid+`"`) && !strings.Contains(pol, `"Sid": "`+sid+`"`) {
			// Sid may be encoded without spaces; also accept substring match on statement id.
			if !strings.Contains(pol, sid) {
				return "DELETED"
			}
		}
		return "IN_SYNC"
	case "AWS::SNS::Topic", "AWS::KMS::Key", "AWS::Events::EventBus", "AWS::Events::Rule",
		"AWS::SecretsManager::Secret", "AWS::SQS::QueuePolicy":
		return "IN_SYNC"
	default:
		return "NOT_CHECKED"
	}
}

func (s *Store) DescribeCFNStackDriftDetectionStatus(accountID, detectionID string) (CFNStackDriftDetection, error) {
	detectionID = strings.TrimSpace(detectionID)
	var d CFNStackDriftDetection
	err := s.db.QueryRow(
		`SELECT detection_id, stack_id, stack_drift_status, detection_status, created_at
		 FROM cfn_stack_drift WHERE account_id = ? AND detection_id = ?`,
		accountID, detectionID,
	).Scan(&d.StackDriftDetectionID, &d.StackID, &d.StackDriftStatus, &d.DetectionStatus, &d.Timestamp)
	if errors.Is(err, sql.ErrNoRows) {
		return CFNStackDriftDetection{}, ErrCFNStackNotFound
	}
	if err != nil {
		return CFNStackDriftDetection{}, fmt.Errorf("describe drift detection: %w", err)
	}
	return d, nil
}

func (s *Store) DescribeCFNStackResourceDrifts(accountID, stackNameOrID string) ([]CFNResourceDrift, error) {
	stacks, err := s.DescribeCFNStacks(accountID, stackNameOrID)
	if err != nil {
		return nil, err
	}
	st := stacks[0]
	rows, err := s.db.Query(
		`SELECT logical_id, resource_type, physical_id, drift_status FROM cfn_resource_drift WHERE stack_id = ?`,
		st.StackID,
	)
	if err != nil {
		return nil, fmt.Errorf("list resource drifts: %w", err)
	}
	defer rows.Close()
	var out []CFNResourceDrift
	for rows.Next() {
		var d CFNResourceDrift
		if err := rows.Scan(&d.LogicalResourceID, &d.ResourceType, &d.PhysicalResourceID, &d.StackResourceDriftStatus); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}