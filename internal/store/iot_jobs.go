package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	IoTJobExecQueued     = "QUEUED"
	IoTJobExecInProgress = "IN_PROGRESS"
)

// IoTJob is a control-plane job record.
type IoTJob struct {
	JobID     string
	JobARN    string
	Document  string
	Targets   []string
	Status    string
	CreatedAt int64
}

// IoTJobExecution is a per-thing job execution.
type IoTJobExecution struct {
	JobID           string
	ThingName       string
	Status          string
	ExecutionNumber int64
	VersionNumber   int64
	QueuedAt        int64
	StartedAt       int64
	LastUpdatedAt   int64
	StatusDetails   map[string]string
	JobDocument     string
}

// CreateIoTJob stores a job and QUEUED executions for each thing target.
func (s *Store) CreateIoTJob(accountID, region, jobID, document string, targets []string) (IoTJob, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTJob{}, err
	}
	region = iotRegion(region)
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return IoTJob{}, fmt.Errorf("%w: jobId required", ErrIoTBadRequest)
	}
	if len(targets) == 0 {
		return IoTJob{}, fmt.Errorf("%w: targets required", ErrIoTBadRequest)
	}
	if strings.TrimSpace(document) == "" {
		document = "{}"
	}
	now := time.Now().UTC().Unix()
	arn := IoTJobARN(region, accountID, jobID)
	rawTargets, err := json.Marshal(targets)
	if err != nil {
		return IoTJob{}, fmt.Errorf("%w: targets", ErrIoTBadRequest)
	}
	_, err = s.db.Exec(
		`INSERT INTO iot_jobs (account_id, region, job_id, job_arn, document, targets_json, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'IN_PROGRESS', ?)`,
		accountID, region, jobID, arn, document, string(rawTargets), now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return IoTJob{}, ErrIoTConflict
		}
		return IoTJob{}, fmt.Errorf("create iot job: %w", err)
	}
	for _, t := range targets {
		thing := iotThingNameFromTarget(t)
		if thing == "" {
			continue
		}
		if _, err := s.DescribeIoTThing(accountID, region, thing); err != nil {
			return IoTJob{}, err
		}
		_, err = s.db.Exec(
			`INSERT INTO iot_job_executions
			 (account_id, region, job_id, thing_name, status, execution_number, version_number,
			  queued_at, started_at, last_updated_at, status_details_json)
			 VALUES (?, ?, ?, ?, ?, 1, 1, ?, 0, ?, '{}')`,
			accountID, region, jobID, thing, IoTJobExecQueued, now, now,
		)
		if err != nil {
			return IoTJob{}, fmt.Errorf("create job execution: %w", err)
		}
	}
	return IoTJob{
		JobID: jobID, JobARN: arn, Document: document, Targets: targets,
		Status: "IN_PROGRESS", CreatedAt: now,
	}, nil
}

// DescribeIoTJob returns a job by id.
func (s *Store) DescribeIoTJob(accountID, region, jobID string) (IoTJob, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTJob{}, err
	}
	var j IoTJob
	var targetsJSON string
	err := s.db.QueryRow(
		`SELECT job_id, job_arn, document, targets_json, status, created_at
		 FROM iot_jobs WHERE account_id = ? AND region = ? AND job_id = ?`,
		accountID, iotRegion(region), strings.TrimSpace(jobID),
	).Scan(&j.JobID, &j.JobARN, &j.Document, &targetsJSON, &j.Status, &j.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return IoTJob{}, ErrIoTNotFound
	}
	if err != nil {
		return IoTJob{}, fmt.Errorf("describe iot job: %w", err)
	}
	_ = json.Unmarshal([]byte(targetsJSON), &j.Targets)
	if j.Targets == nil {
		j.Targets = []string{}
	}
	return j, nil
}

// ListPendingIoTJobExecutions returns IN_PROGRESS and QUEUED executions for a thing.
func (s *Store) ListPendingIoTJobExecutions(accountID, region, thingName string) (inProgress, queued []IoTJobExecution, err error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return nil, nil, err
	}
	region = iotRegion(region)
	thingName = strings.TrimSpace(thingName)
	rows, err := s.db.Query(
		`SELECT e.job_id, e.thing_name, e.status, e.execution_number, e.version_number,
		        e.queued_at, e.started_at, e.last_updated_at, e.status_details_json, j.document
		 FROM iot_job_executions e
		 JOIN iot_jobs j ON j.account_id = e.account_id AND j.region = e.region AND j.job_id = e.job_id
		 WHERE e.account_id = ? AND e.region = ? AND e.thing_name = ?
		   AND e.status IN (?, ?)
		 ORDER BY e.queued_at ASC, e.job_id ASC`,
		accountID, region, thingName, IoTJobExecInProgress, IoTJobExecQueued,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("list pending job executions: %w", err)
	}
	defer rows.Close()
	inProgress = []IoTJobExecution{}
	queued = []IoTJobExecution{}
	for rows.Next() {
		ex, err := scanIoTJobExecution(rows)
		if err != nil {
			return nil, nil, err
		}
		switch ex.Status {
		case IoTJobExecInProgress:
			inProgress = append(inProgress, ex)
		case IoTJobExecQueued:
			queued = append(queued, ex)
		}
	}
	return inProgress, queued, rows.Err()
}

// GetIoTJobExecution returns one execution. jobID "$next" is the oldest pending execution.
func (s *Store) GetIoTJobExecution(accountID, region, thingName, jobID string) (IoTJobExecution, error) {
	if strings.TrimSpace(jobID) == "$next" {
		inProgress, queued, err := s.ListPendingIoTJobExecutions(accountID, region, thingName)
		if err != nil {
			return IoTJobExecution{}, err
		}
		if len(inProgress) > 0 {
			return inProgress[0], nil
		}
		if len(queued) > 0 {
			return queued[0], nil
		}
		return IoTJobExecution{}, ErrIoTNotFound
	}
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTJobExecution{}, err
	}
	row := s.db.QueryRow(
		`SELECT e.job_id, e.thing_name, e.status, e.execution_number, e.version_number,
		        e.queued_at, e.started_at, e.last_updated_at, e.status_details_json, j.document
		 FROM iot_job_executions e
		 JOIN iot_jobs j ON j.account_id = e.account_id AND j.region = e.region AND j.job_id = e.job_id
		 WHERE e.account_id = ? AND e.region = ? AND e.thing_name = ? AND e.job_id = ?`,
		accountID, iotRegion(region), strings.TrimSpace(thingName), strings.TrimSpace(jobID),
	)
	ex, err := scanIoTJobExecution(row)
	if errors.Is(err, sql.ErrNoRows) {
		return IoTJobExecution{}, ErrIoTNotFound
	}
	return ex, err
}

// StartNextIoTJobExecution moves the oldest QUEUED (or existing IN_PROGRESS) execution to IN_PROGRESS.
func (s *Store) StartNextIoTJobExecution(accountID, region, thingName string, statusDetails map[string]string) (IoTJobExecution, error) {
	inProgress, queued, err := s.ListPendingIoTJobExecutions(accountID, region, thingName)
	if err != nil {
		return IoTJobExecution{}, err
	}
	var target IoTJobExecution
	switch {
	case len(inProgress) > 0:
		target = inProgress[0]
	case len(queued) > 0:
		target = queued[0]
	default:
		return IoTJobExecution{}, ErrIoTNotFound
	}
	now := time.Now().UTC().Unix()
	if statusDetails == nil {
		statusDetails = map[string]string{}
	}
	detailsJSON, err := json.Marshal(statusDetails)
	if err != nil {
		return IoTJobExecution{}, fmt.Errorf("%w: statusDetails", ErrIoTBadRequest)
	}
	started := target.StartedAt
	if started == 0 {
		started = now
	}
	version := target.VersionNumber + 1
	_, err = s.db.Exec(
		`UPDATE iot_job_executions
		 SET status = ?, started_at = ?, last_updated_at = ?, version_number = ?, status_details_json = ?
		 WHERE account_id = ? AND region = ? AND job_id = ? AND thing_name = ?`,
		IoTJobExecInProgress, started, now, version, string(detailsJSON),
		accountID, iotRegion(region), target.JobID, strings.TrimSpace(thingName),
	)
	if err != nil {
		return IoTJobExecution{}, fmt.Errorf("start next job execution: %w", err)
	}
	target.Status = IoTJobExecInProgress
	target.StartedAt = started
	target.LastUpdatedAt = now
	target.VersionNumber = version
	target.StatusDetails = statusDetails
	return target, nil
}

type jobExecScanner interface {
	Scan(dest ...any) error
}

func scanIoTJobExecution(sc jobExecScanner) (IoTJobExecution, error) {
	var ex IoTJobExecution
	var detailsJSON string
	err := sc.Scan(
		&ex.JobID, &ex.ThingName, &ex.Status, &ex.ExecutionNumber, &ex.VersionNumber,
		&ex.QueuedAt, &ex.StartedAt, &ex.LastUpdatedAt, &detailsJSON, &ex.JobDocument,
	)
	if err != nil {
		return IoTJobExecution{}, err
	}
	_ = json.Unmarshal([]byte(detailsJSON), &ex.StatusDetails)
	if ex.StatusDetails == nil {
		ex.StatusDetails = map[string]string{}
	}
	return ex, nil
}

func iotThingNameFromTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	const marker = ":thing/"
	if i := strings.LastIndex(target, marker); i >= 0 {
		return strings.TrimSpace(target[i+len(marker):])
	}
	return target
}
