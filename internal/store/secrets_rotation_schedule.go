package store

import (
	"database/sql"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SecretRotationRules is the lab subset of AWS RotationRulesType.
// AutomaticallyAfterDays and ScheduleExpression are mutually exclusive (AWS shape).
type SecretRotationRules struct {
	AutomaticallyAfterDays int64
	ScheduleExpression     string
	Duration               string
}

var (
	rateDaysRE         = regexp.MustCompile(`(?i)^rate\((\d+)\s+days?\)$`)
	rateHoursRE        = regexp.MustCompile(`(?i)^rate\((\d+)\s+hours?\)$`)
	rotationDurationRE = regexp.MustCompile(`(?i)^(\d+)h$`)
	secretsCronExprRE  = regexp.MustCompile(`(?i)^cron\(\s*([^\s]+)\s+([^\s]+)\s+([^\s]+)\s+([^\s]+)\s+([^\s]+)\s+([^\s]+)\s*\)$`)

	rotationJitterMu   sync.Mutex
	rotationJitterRand *rand.Rand
)

// EnsureSecretsRotationScheduleSchema adds RotationRules schedule columns.
func EnsureSecretsRotationScheduleSchema(db *sql.DB) error {
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE secretsmanager_secrets ADD COLUMN rotation_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE secretsmanager_secrets ADD COLUMN automatically_after_days INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE secretsmanager_secrets ADD COLUMN schedule_expression TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE secretsmanager_secrets ADD COLUMN rotation_duration TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE secretsmanager_secrets ADD COLUMN next_rotation_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE secretsmanager_secrets ADD COLUMN last_rotated_date TEXT NOT NULL DEFAULT ''`,
	}); err != nil {
		return fmt.Errorf("ensure secrets rotation schedule schema: %w", err)
	}
	return nil
}

func (s *Store) EnsureSecretsRotationScheduleSchema() error {
	return EnsureSecretsRotationScheduleSchema(s.db)
}

// ValidateSecretRotationRules rejects empty rules and AutomaticallyAfterDays+ScheduleExpression together.
func ValidateSecretRotationRules(rules SecretRotationRules) error {
	days := rules.AutomaticallyAfterDays
	expr := strings.TrimSpace(rules.ScheduleExpression)
	dur := strings.TrimSpace(rules.Duration)
	if days <= 0 && expr == "" {
		if dur != "" {
			return fmt.Errorf("ValidationException: Duration requires AutomaticallyAfterDays or ScheduleExpression")
		}
		return fmt.Errorf("ValidationException: RotationRules requires AutomaticallyAfterDays or ScheduleExpression")
	}
	if days > 0 && expr != "" {
		return fmt.Errorf("ValidationException: RotationRules cannot set both AutomaticallyAfterDays and ScheduleExpression")
	}
	if days > 0 && (days < 1 || days > 1000) {
		return fmt.Errorf("ValidationException: AutomaticallyAfterDays must be between 1 and 1000")
	}
	if expr != "" {
		if _, err := nextRotationTime(SecretRotationRules{ScheduleExpression: expr}, time.Unix(0, 0).UTC()); err != nil {
			return err
		}
	}
	if dur != "" {
		if err := validateRotationDuration(rules); err != nil {
			return err
		}
	}
	return nil
}

func parseRotationDurationHours(dur string) (int, error) {
	dur = strings.TrimSpace(dur)
	if dur == "" {
		return 0, nil
	}
	m := rotationDurationRE.FindStringSubmatch(dur)
	if m == nil {
		return 0, fmt.Errorf("ValidationException: Duration must match Nh (for example 3h)")
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 {
		return 0, fmt.Errorf("ValidationException: invalid Duration %q", dur)
	}
	return n, nil
}

func validateRotationDuration(rules SecretRotationRules) error {
	hours, err := parseRotationDurationHours(rules.Duration)
	if err != nil {
		return err
	}
	if hours == 0 {
		return nil
	}
	expr := strings.TrimSpace(rules.ScheduleExpression)
	if rules.AutomaticallyAfterDays > 0 || rateDaysRE.MatchString(expr) || secretsCronExprRE.MatchString(expr) {
		if hours > 24 {
			return fmt.Errorf("ValidationException: Duration must not exceed 24h for day-based schedules")
		}
		return nil
	}
	if m := rateHoursRE.FindStringSubmatch(expr); m != nil {
		n, _ := strconv.Atoi(m[1])
		if hours > n {
			return fmt.Errorf("ValidationException: Duration must not exceed the rate interval")
		}
		return nil
	}
	return fmt.Errorf("ValidationException: Duration is not valid for ScheduleExpression")
}

func nextRotationTime(rules SecretRotationRules, from time.Time) (time.Time, error) {
	from = from.UTC()
	if rules.AutomaticallyAfterDays > 0 {
		return from.Add(time.Duration(rules.AutomaticallyAfterDays) * 24 * time.Hour), nil
	}
	expr := strings.TrimSpace(rules.ScheduleExpression)
	if m := rateDaysRE.FindStringSubmatch(expr); m != nil {
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || n < 1 {
			return time.Time{}, fmt.Errorf("ValidationException: invalid rate expression %q", expr)
		}
		if n > 999 {
			return time.Time{}, fmt.Errorf("ValidationException: rate days must be at most 999")
		}
		return from.Add(time.Duration(n) * 24 * time.Hour), nil
	}
	if m := rateHoursRE.FindStringSubmatch(expr); m != nil {
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || n < 1 {
			return time.Time{}, fmt.Errorf("ValidationException: invalid rate expression %q", expr)
		}
		if n < 4 {
			return time.Time{}, fmt.Errorf("ValidationException: rate hours must be at least 4")
		}
		return from.Add(time.Duration(n) * time.Hour), nil
	}
	if m := secretsCronExprRE.FindStringSubmatch(expr); m != nil {
		minute, hour, dom, month, dow, year := m[1], m[2], m[3], m[4], m[5], m[6]
		if strings.TrimSpace(minute) != "0" {
			return time.Time{}, fmt.Errorf("ValidationException: Secrets Manager cron minutes must be 0")
		}
		if strings.TrimSpace(year) != "*" {
			return time.Time{}, fmt.Errorf("ValidationException: Secrets Manager cron year must be *")
		}
		next, err := nextCronRun(from, minute, hour, dom, month, dow, year)
		if err != nil {
			return time.Time{}, fmt.Errorf("ValidationException: %v", err)
		}
		return next, nil
	}
	return time.Time{}, fmt.Errorf("ValidationException: lab ScheduleExpression supports rate(N days|hours) or cron(0 H D M Dow *)")
}

// applyRotationWindowJitter adds a random offset in [0, Duration) to the window start.
// Empty Duration leaves next unchanged (backward compatible).
func applyRotationWindowJitter(next time.Time, duration string, rnd *rand.Rand) (time.Time, error) {
	hours, err := parseRotationDurationHours(duration)
	if err != nil {
		return time.Time{}, err
	}
	if hours == 0 {
		return next, nil
	}
	if rnd == nil {
		rnd = rotationJitterRNG()
	}
	window := int64(hours) * int64(time.Hour)
	offset := time.Duration(rnd.Int63n(window))
	return next.Add(offset), nil
}

func rotationJitterRNG() *rand.Rand {
	rotationJitterMu.Lock()
	defer rotationJitterMu.Unlock()
	if rotationJitterRand != nil {
		return rotationJitterRand
	}
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

// SetRotationJitterRand injects a deterministic RNG for Duration in-window jitter tests.
// Pass nil to restore process-default random offsets.
func (s *Store) SetRotationJitterRand(rnd *rand.Rand) {
	_ = s
	rotationJitterMu.Lock()
	defer rotationJitterMu.Unlock()
	rotationJitterRand = rnd
}

// SetSecretRotationRules persists RotationRules and enables automatic rotation.
// When rotateImmediately is false, next_rotation_date is now+interval (no rotate).
// When true, next_rotation_date is left for MarkSecretRotated after the caller rotates.
func (s *Store) SetSecretRotationRules(
	accountID, nameOrARN string,
	rules SecretRotationRules,
	now time.Time,
	rotateImmediately bool,
) error {
	if err := ValidateSecretRotationRules(rules); err != nil {
		return err
	}
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return err
	}
	if _, err := s.getSecretRow(accountID, name); err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	nextTime, err := nextRotationTime(rules, now)
	if err != nil {
		return err
	}
	nextTime, err = applyRotationWindowJitter(nextTime, rules.Duration, nil)
	if err != nil {
		return err
	}
	days := rules.AutomaticallyAfterDays
	if days <= 0 {
		if m := rateDaysRE.FindStringSubmatch(strings.TrimSpace(rules.ScheduleExpression)); m != nil {
			n, _ := strconv.ParseInt(m[1], 10, 64)
			days = n
		}
	}
	next := ""
	if !rotateImmediately {
		next = nextTime.Format(time.RFC3339)
	}
	if rotateImmediately {
		_, err = s.db.Exec(
			`UPDATE secretsmanager_secrets
			 SET rotation_enabled = 1,
			     automatically_after_days = ?,
			     schedule_expression = ?,
			     rotation_duration = ?
			 WHERE account_id = ? AND name = ?`,
			days,
			strings.TrimSpace(rules.ScheduleExpression),
			strings.TrimSpace(rules.Duration),
			accountID, name,
		)
	} else {
		_, err = s.db.Exec(
			`UPDATE secretsmanager_secrets
			 SET rotation_enabled = 1,
			     automatically_after_days = ?,
			     schedule_expression = ?,
			     rotation_duration = ?,
			     next_rotation_date = ?
			 WHERE account_id = ? AND name = ?`,
			days,
			strings.TrimSpace(rules.ScheduleExpression),
			strings.TrimSpace(rules.Duration),
			next,
			accountID, name,
		)
	}
	if err != nil {
		return fmt.Errorf("set secret rotation rules: %w", err)
	}
	return nil
}

// MarkSecretRotated records LastRotatedDate and advances NextRotationDate from persisted rules.
func (s *Store) MarkSecretRotated(accountID, nameOrARN string, now time.Time) error {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return err
	}
	row, err := s.getSecretRow(accountID, name)
	if err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	rules := SecretRotationRules{
		AutomaticallyAfterDays: row.AutomaticallyAfterDays,
		ScheduleExpression:     row.ScheduleExpression,
		Duration:               row.RotationDuration,
	}
	// Prefer ScheduleExpression when present (AWS mutual exclusion).
	if strings.TrimSpace(rules.ScheduleExpression) != "" {
		rules.AutomaticallyAfterDays = 0
	}
	nextTime, err := nextRotationTime(rules, now)
	if err != nil {
		return err
	}
	nextTime, err = applyRotationWindowJitter(nextTime, rules.Duration, nil)
	if err != nil {
		return err
	}
	next := nextTime.Format(time.RFC3339)
	_, err = s.db.Exec(
		`UPDATE secretsmanager_secrets
		 SET last_rotated_date = ?, next_rotation_date = ?
		 WHERE account_id = ? AND name = ?`,
		now.Format(time.RFC3339), next, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("mark secret rotated: %w", err)
	}
	return nil
}

// SecretRotationDue is one secret due for automatic rotation.
type SecretRotationDue struct {
	AccountID string
	Name      string
	ARN       string
}

// ListDueSecretRotations returns enabled secrets with next_rotation_date <= now.
func (s *Store) ListDueSecretRotations(now time.Time) ([]SecretRotationDue, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	rows, err := s.db.Query(
		`SELECT account_id, name, arn FROM secretsmanager_secrets
		 WHERE rotation_enabled = 1
		   AND next_rotation_date != ''
		   AND next_rotation_date <= ?
		   AND (deletion_date = '' OR deletion_date IS NULL)
		 ORDER BY next_rotation_date, account_id, name`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("list due secret rotations: %w", err)
	}
	defer rows.Close()
	var out []SecretRotationDue
	for rows.Next() {
		var d SecretRotationDue
		if err := rows.Scan(&d.AccountID, &d.Name, &d.ARN); err != nil {
			return nil, fmt.Errorf("list due secret rotations: scan: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ProcessDueSecretRotations rotates due secrets. When rotate is nil, uses lab RotateSecret (random).
// After each successful rotate, advances LastRotatedDate / NextRotationDate.
func (s *Store) ProcessDueSecretRotations(now time.Time, rotate func(accountID, name string) error) (int, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	due, err := s.ListDueSecretRotations(now)
	if err != nil {
		return 0, err
	}
	if rotate == nil {
		rotate = func(accountID, name string) error {
			_, err := s.RotateSecret(accountID, name)
			return err
		}
	}
	n := 0
	for _, d := range due {
		orphan, err := s.HasPendingRotationOrphan(d.AccountID, d.Name)
		if err != nil {
			return n, err
		}
		if orphan {
			continue
		}
		if err := rotate(d.AccountID, d.Name); err != nil {
			return n, fmt.Errorf("scheduled rotate %s: %w", d.Name, err)
		}
		if err := s.MarkSecretRotated(d.AccountID, d.Name, now); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
