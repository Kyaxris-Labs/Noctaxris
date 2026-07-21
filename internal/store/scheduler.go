package store

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

const (
	// DefaultSchedulerRegion is the lab region embedded in Scheduler ARNs.
	DefaultSchedulerRegion = "us-east-1"
	// DefaultScheduleGroup is the lab default schedule group name.
	DefaultScheduleGroup = "default"
	schedulerStateEnabled  = "ENABLED"
	schedulerStateDisabled = "DISABLED"
)

var (
	ErrScheduleAlreadyExists = errors.New("ConflictException")
	ErrNoSuchSchedule        = errors.New("ResourceNotFoundException")
	ErrInvalidScheduleExpr   = errors.New("ValidationException")
	ErrInvalidScheduleTarget = errors.New("ValidationException")
)

// Schedule is an EventBridge Scheduler schedule row.
type Schedule struct {
	AccountID          string
	Name               string
	GroupName          string
	ScheduleARN        string
	Expression         string
	State              string
	TargetARN          string
	RoleARN            string
	Input              string
	NextRun            string
	CreationDate       string
	LastModificationDate string
}

// CreateScheduleInput is the create payload.
type CreateScheduleInput struct {
	Name       string
	GroupName  string
	Expression string
	State      string
	TargetARN  string
	RoleARN    string
	Input      string
}

const schedulerSchema = `
CREATE TABLE IF NOT EXISTS scheduler_schedules (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  group_name TEXT NOT NULL DEFAULT 'default',
  schedule_arn TEXT NOT NULL,
  expression TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'ENABLED',
  target_arn TEXT NOT NULL,
  role_arn TEXT NOT NULL DEFAULT '',
  input TEXT NOT NULL DEFAULT '',
  next_run TEXT NOT NULL DEFAULT '',
  creation_date TEXT NOT NULL,
  last_modification_date TEXT NOT NULL,
  PRIMARY KEY (account_id, group_name, name)
);
CREATE INDEX IF NOT EXISTS idx_scheduler_due
  ON scheduler_schedules(state, next_run);
`

// EnsureSchedulerSchema creates Scheduler tables if missing.
func EnsureSchedulerSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure scheduler schema: db is nil")
	}
	if _, err := db.Exec(schedulerSchema); err != nil {
		return fmt.Errorf("ensure scheduler schema: %w", err)
	}
	return nil
}

// EnsureSchedulerSchema ensures Scheduler tables on an open store.
func (s *Store) EnsureSchedulerSchema() error {
	return EnsureSchedulerSchema(s.db)
}

// ScheduleARN builds arn:aws:scheduler:REGION:ACCOUNT:schedule/GROUP/NAME.
func ScheduleARN(region, accountID, groupName, name string) string {
	if region == "" {
		region = DefaultSchedulerRegion
	}
	if groupName == "" {
		groupName = DefaultScheduleGroup
	}
	return fmt.Sprintf("arn:aws:scheduler:%s:%s:schedule/%s/%s", region, accountID, groupName, name)
}

func scanSchedule(row *sql.Row) (Schedule, error) {
	var sch Schedule
	err := row.Scan(
		&sch.AccountID, &sch.Name, &sch.GroupName, &sch.ScheduleARN, &sch.Expression, &sch.State,
		&sch.TargetARN, &sch.RoleARN, &sch.Input, &sch.NextRun, &sch.CreationDate, &sch.LastModificationDate,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Schedule{}, ErrNoSuchSchedule
	}
	if err != nil {
		return Schedule{}, fmt.Errorf("scan schedule: %w", err)
	}
	return sch, nil
}

func normalizeScheduleGroup(group string) string {
	group = strings.TrimSpace(group)
	if group == "" {
		return DefaultScheduleGroup
	}
	return group
}

func normalizeScheduleState(state string) string {
	state = strings.ToUpper(strings.TrimSpace(state))
	if state == "" {
		return schedulerStateEnabled
	}
	return state
}

func validateScheduleTargetARN(arn string) error {
	arn = strings.TrimSpace(arn)
	switch {
	case strings.HasPrefix(arn, "arn:aws:sqs:"),
		strings.HasPrefix(arn, "arn:aws:lambda:"),
		strings.HasPrefix(arn, "arn:aws:sns:"):
		return nil
	default:
		return fmt.Errorf("%w: target Arn must be SQS, Lambda, or SNS", ErrInvalidScheduleTarget)
	}
}

// CreateSchedule inserts a schedule and computes the first next_run.
func (s *Store) CreateSchedule(accountID, region string, in CreateScheduleInput, now time.Time) (Schedule, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Schedule{}, fmt.Errorf("%w: Name is required", ErrInvalidScheduleExpr)
	}
	group := normalizeScheduleGroup(in.GroupName)
	expr := strings.TrimSpace(in.Expression)
	if expr == "" {
		return Schedule{}, fmt.Errorf("%w: ScheduleExpression is required", ErrInvalidScheduleExpr)
	}
	target := strings.TrimSpace(in.TargetARN)
	if err := validateScheduleTargetARN(target); err != nil {
		return Schedule{}, err
	}
	state := normalizeScheduleState(in.State)
	if state != schedulerStateEnabled && state != schedulerStateDisabled {
		return Schedule{}, fmt.Errorf("%w: State must be ENABLED or DISABLED", ErrInvalidScheduleExpr)
	}
	now = now.UTC()
	next, err := NextScheduleRun(expr, now)
	if err != nil {
		return Schedule{}, fmt.Errorf("%w: %v", ErrInvalidScheduleExpr, err)
	}
	created := now.Format(time.RFC3339)
	arn := ScheduleARN(region, accountID, group, name)
	_, err = s.db.Exec(
		`INSERT INTO scheduler_schedules
		 (account_id, name, group_name, schedule_arn, expression, state, target_arn, role_arn, input,
		  next_run, creation_date, last_modification_date)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, group, arn, expr, state, target, strings.TrimSpace(in.RoleARN), strings.TrimSpace(in.Input),
		next.Format(time.RFC3339), created, created,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Schedule{}, ErrScheduleAlreadyExists
		}
		return Schedule{}, fmt.Errorf("create schedule: %w", err)
	}
	return Schedule{
		AccountID:            accountID,
		Name:                 name,
		GroupName:            group,
		ScheduleARN:          arn,
		Expression:           expr,
		State:                state,
		TargetARN:            target,
		RoleARN:              strings.TrimSpace(in.RoleARN),
		Input:                strings.TrimSpace(in.Input),
		NextRun:              next.Format(time.RFC3339),
		CreationDate:         created,
		LastModificationDate: created,
	}, nil
}

// GetSchedule returns a schedule by name (default group when group empty).
func (s *Store) GetSchedule(accountID, groupName, name string) (Schedule, error) {
	groupName = normalizeScheduleGroup(groupName)
	row := s.db.QueryRow(
		`SELECT account_id, name, group_name, schedule_arn, expression, state, target_arn, role_arn, input,
		        next_run, creation_date, last_modification_date
		 FROM scheduler_schedules WHERE account_id = ? AND group_name = ? AND name = ?`,
		accountID, groupName, name,
	)
	return scanSchedule(row)
}

// ListSchedules returns schedules for an account, optionally filtered by group.
func (s *Store) ListSchedules(accountID, groupName string) ([]Schedule, error) {
	var (
		rows *sql.Rows
		err  error
	)
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		rows, err = s.db.Query(
			`SELECT account_id, name, group_name, schedule_arn, expression, state, target_arn, role_arn, input,
			        next_run, creation_date, last_modification_date
			 FROM scheduler_schedules WHERE account_id = ? ORDER BY group_name, name`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT account_id, name, group_name, schedule_arn, expression, state, target_arn, role_arn, input,
			        next_run, creation_date, last_modification_date
			 FROM scheduler_schedules WHERE account_id = ? AND group_name = ? ORDER BY name`,
			accountID, groupName,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()
	out := []Schedule{}
	for rows.Next() {
		var sch Schedule
		if err := rows.Scan(
			&sch.AccountID, &sch.Name, &sch.GroupName, &sch.ScheduleARN, &sch.Expression, &sch.State,
			&sch.TargetARN, &sch.RoleARN, &sch.Input, &sch.NextRun, &sch.CreationDate, &sch.LastModificationDate,
		); err != nil {
			return nil, fmt.Errorf("list schedules: %w", err)
		}
		out = append(out, sch)
	}
	return out, rows.Err()
}

// UpdateScheduleInput is a partial update payload (empty fields keep prior values).
type UpdateScheduleInput struct {
	Expression *string
	State      *string
	TargetARN  *string
	RoleARN    *string
	Input      *string
}

// UpdateSchedule updates mutable schedule fields and recomputes next_run when expression changes.
func (s *Store) UpdateSchedule(accountID, groupName, name string, in UpdateScheduleInput, now time.Time) (Schedule, error) {
	sch, err := s.GetSchedule(accountID, groupName, name)
	if err != nil {
		return Schedule{}, err
	}
	expr := sch.Expression
	state := sch.State
	target := sch.TargetARN
	role := sch.RoleARN
	input := sch.Input
	recomputeNext := false
	if in.Expression != nil {
		expr = strings.TrimSpace(*in.Expression)
		if expr == "" {
			return Schedule{}, fmt.Errorf("%w: ScheduleExpression is required", ErrInvalidScheduleExpr)
		}
		recomputeNext = true
	}
	if in.State != nil {
		state = normalizeScheduleState(*in.State)
		if state != schedulerStateEnabled && state != schedulerStateDisabled {
			return Schedule{}, fmt.Errorf("%w: State must be ENABLED or DISABLED", ErrInvalidScheduleExpr)
		}
	}
	if in.TargetARN != nil {
		target = strings.TrimSpace(*in.TargetARN)
		if err := validateScheduleTargetARN(target); err != nil {
			return Schedule{}, err
		}
	}
	if in.RoleARN != nil {
		role = strings.TrimSpace(*in.RoleARN)
	}
	if in.Input != nil {
		input = strings.TrimSpace(*in.Input)
	}
	now = now.UTC()
	nextRun := sch.NextRun
	if recomputeNext {
		next, err := NextScheduleRun(expr, now)
		if err != nil {
			return Schedule{}, fmt.Errorf("%w: %v", ErrInvalidScheduleExpr, err)
		}
		nextRun = next.Format(time.RFC3339)
	}
	mod := now.Format(time.RFC3339)
	_, err = s.db.Exec(
		`UPDATE scheduler_schedules
		 SET expression = ?, state = ?, target_arn = ?, role_arn = ?, input = ?, next_run = ?, last_modification_date = ?
		 WHERE account_id = ? AND group_name = ? AND name = ?`,
		expr, state, target, role, input, nextRun, mod,
		accountID, normalizeScheduleGroup(groupName), name,
	)
	if err != nil {
		return Schedule{}, fmt.Errorf("update schedule: %w", err)
	}
	return s.GetSchedule(accountID, groupName, name)
}

// DeleteSchedule removes a schedule.
func (s *Store) DeleteSchedule(accountID, groupName, name string) error {
	res, err := s.db.Exec(
		`DELETE FROM scheduler_schedules WHERE account_id = ? AND group_name = ? AND name = ?`,
		accountID, normalizeScheduleGroup(groupName), name,
	)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoSuchSchedule
	}
	return nil
}

// ListDueSchedules returns ENABLED schedules with next_run <= now.
func (s *Store) ListDueSchedules(now time.Time) ([]Schedule, error) {
	nowStr := now.UTC().Format(time.RFC3339)
	rows, err := s.db.Query(
		`SELECT account_id, name, group_name, schedule_arn, expression, state, target_arn, role_arn, input,
		        next_run, creation_date, last_modification_date
		 FROM scheduler_schedules
		 WHERE state = ? AND next_run != '' AND next_run <= ?
		 ORDER BY next_run, account_id, group_name, name`,
		schedulerStateEnabled, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("list due schedules: %w", err)
	}
	defer rows.Close()
	out := []Schedule{}
	for rows.Next() {
		var sch Schedule
		if err := rows.Scan(
			&sch.AccountID, &sch.Name, &sch.GroupName, &sch.ScheduleARN, &sch.Expression, &sch.State,
			&sch.TargetARN, &sch.RoleARN, &sch.Input, &sch.NextRun, &sch.CreationDate, &sch.LastModificationDate,
		); err != nil {
			return nil, fmt.Errorf("list due schedules: %w", err)
		}
		out = append(out, sch)
	}
	return out, rows.Err()
}

// AdvanceScheduleNextRun sets next_run after a successful (or attempted) delivery fire.
// One-time at() schedules are DISABLED with empty next_run after fire.
func (s *Store) AdvanceScheduleNextRun(sch Schedule, firedAt time.Time) error {
	firedAt = firedAt.UTC()
	expr := strings.TrimSpace(sch.Expression)
	if strings.HasPrefix(strings.ToLower(expr), "at(") {
		_, err := s.db.Exec(
			`UPDATE scheduler_schedules SET state = ?, next_run = '', last_modification_date = ?
			 WHERE account_id = ? AND group_name = ? AND name = ?`,
			schedulerStateDisabled, firedAt.Format(time.RFC3339),
			sch.AccountID, sch.GroupName, sch.Name,
		)
		if err != nil {
			return fmt.Errorf("advance at schedule: %w", err)
		}
		return nil
	}
	next, err := NextScheduleRun(expr, firedAt)
	if err != nil {
		return fmt.Errorf("advance schedule next_run: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE scheduler_schedules SET next_run = ?, last_modification_date = ?
		 WHERE account_id = ? AND group_name = ? AND name = ?`,
		next.Format(time.RFC3339), firedAt.Format(time.RFC3339),
		sch.AccountID, sch.GroupName, sch.Name,
	)
	if err != nil {
		return fmt.Errorf("advance schedule: %w", err)
	}
	return nil
}

// ProcessDueSchedules delivers due schedules and advances next_run. Returns delivered count.
func (s *Store) ProcessDueSchedules(now time.Time) (int, error) {
	due, err := s.ListDueSchedules(now)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, sch := range due {
		if s.schedulerDeliveryAuthorized(sch) {
			if err := s.deliverScheduleTarget(sch); err != nil {
				log.Printf("scheduler delivery failed schedule=%s target=%s err=%v",
					sch.ScheduleARN, sch.TargetARN, err)
			} else {
				delivered++
			}
		} else {
			log.Printf("scheduler delivery denied schedule=%s target=%s", sch.ScheduleARN, sch.TargetARN)
		}
		if err := s.AdvanceScheduleNextRun(sch, now); err != nil {
			log.Printf("scheduler advance failed schedule=%s err=%v", sch.ScheduleARN, err)
		}
	}
	return delivered, nil
}

func (s *Store) schedulerDeliveryAuthorized(sch Schedule) bool {
	arn := strings.TrimSpace(sch.TargetARN)
	action, ok := eventTargetDeliveryAction(arn)
	if !ok {
		return false
	}
	roleARN := strings.TrimSpace(sch.RoleARN)
	if roleARN == "" {
		return s.schedulerTargetResourcePolicyAllows(sch.AccountID, arn, action, sch.ScheduleARN)
	}
	return s.schedulerRoleSessionAllows(sch.AccountID, roleARN, action, arn)
}

func (s *Store) schedulerTargetResourcePolicyAllows(accountID, targetARN, action, sourceARN string) bool {
	policyAccount := accountID
	if owner := resourceOwnerAccountFromARN(targetARN); owner != "" {
		policyAccount = owner
	}
	policyDoc, err := s.eventTargetResourcePolicyDoc(policyAccount, targetARN)
	if err != nil {
		return false
	}
	return authz.EventTargetResourcePolicyAllows(
		policyDoc,
		action,
		targetARN,
		authz.ServicePrincipalScheduler,
		policyAccount,
		authz.DeliverySourceConditionKeys(sourceARN, ""),
	)
}

func (s *Store) schedulerRoleSessionAllows(accountID, roleARN, action, targetARN string) bool {
	roleAccountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok || roleAccountID != accountID {
		return false
	}
	if _, _, err := s.GetRole(accountID, roleName); err != nil {
		return false
	}
	secret, err := randomHexSecret(16)
	if err != nil {
		log.Printf("scheduler role session mint secret failed role=%s err=%v", roleARN, err)
		return false
	}
	sessionToken, err := randomHexSecret(16)
	if err != nil {
		log.Printf("scheduler role session mint token failed role=%s err=%v", roleARN, err)
		return false
	}
	accessKeyID, err := s.MintTempCredentialsOpts(MintTempOpts{
		AccountID:    accountID,
		RoleARN:      roleARN,
		SessionName:  "scheduler-delivery",
		Secret:       secret,
		SessionToken: sessionToken,
		Expires:      time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		log.Printf("scheduler role session mint failed role=%s err=%v", roleARN, err)
		return false
	}

	docs, err := s.identityPolicyDocsForRoleARN(roleARN)
	if err != nil {
		log.Printf("scheduler role policy load failed role=%s err=%v", roleARN, err)
		return false
	}
	boundaryDoc := ""
	if doc, ok, err := s.PermissionsBoundaryDoc(accountID, "role", roleName); err == nil && ok {
		boundaryDoc = doc
	}
	scpDocs, err := s.SCPDocsForAccount(accountID)
	if err != nil {
		return false
	}
	rcpDocs, err := s.RCPDocsForAccount(accountID)
	if err != nil {
		return false
	}
	principal := identity.RoleSessionPrincipal(accountID, roleName, "scheduler-delivery", accessKeyID)
	ctx := authz.RequestContext{
		Principal: principal,
		Action:    action,
		Resource:  targetARN,
		Region:    DefaultSchedulerRegion,
	}
	in := authz.EvalInputs{
		IdentityDocs:        docs,
		BoundaryDoc:         boundaryDoc,
		SCPDocs:             scpDocs,
		RCPDocs:             rcpDocs,
		IsManagementAccount: s.IsManagementAccount(accountID),
	}
	return authz.EvaluateFull(ctx, in) == authz.Allow
}

func (s *Store) deliverScheduleTarget(sch Schedule) error {
	body := strings.TrimSpace(sch.Input)
	if body == "" {
		body = fmt.Sprintf(`{"scheduleArn":%q,"time":%q}`, sch.ScheduleARN, time.Now().UTC().Format(time.RFC3339))
	}
	arn := strings.TrimSpace(sch.TargetARN)
	switch {
	case strings.HasPrefix(arn, "arn:aws:sqs:"):
		queueName, err := queueNameFromARN(arn)
		if err != nil {
			return err
		}
		_, err = s.SendMessage(sch.AccountID, queueName, []byte(body), false, nil, "", nil)
		return err
	case strings.HasPrefix(arn, "arn:aws:lambda:"):
		functionName, qualifier := ParseFunctionQualifier(arn)
		if functionName == "" {
			return fmt.Errorf("empty function name")
		}
		_, err := s.EnqueueAsyncInvoke(sch.AccountID, functionName, qualifier, body)
		return err
	case strings.HasPrefix(arn, "arn:aws:sns:"):
		topic, err := s.GetTopicByARN(arn)
		if err != nil {
			return err
		}
		_, err = s.Publish(sch.AccountID, topic.TopicName, body, "", nil)
		return err
	default:
		return fmt.Errorf("unsupported target %s", arn)
	}
}
