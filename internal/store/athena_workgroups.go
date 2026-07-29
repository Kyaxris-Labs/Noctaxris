package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AthenaWorkGroup is a persisted Athena workgroup.
type AthenaWorkGroup struct {
	Name                   string
	State                  string
	Description            string
	OutputLocation         string
	EnforceWorkGroupConfig bool
	CreatedMS              int64
}

// AthenaWorkGroupCreate is CreateWorkGroup input.
type AthenaWorkGroupCreate struct {
	Name                   string
	Description            string
	OutputLocation         string
	EnforceWorkGroupConfig bool
}

// AthenaWorkGroupUpdate is UpdateWorkGroup input (nil pointers mean leave unchanged).
type AthenaWorkGroupUpdate struct {
	Description            *string
	State                  *string
	OutputLocation         *string
	RemoveOutputLocation   bool
	EnforceWorkGroupConfig *bool
}

func (s *Store) ensureAthenaPrimaryWorkGroup(accountID string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return fmt.Errorf("%w: account_id is required", ErrAthenaBadRequest)
	}
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM athena_work_groups WHERE account_id = ? AND name = ?`,
		accountID, DefaultAthenaWorkGroup,
	).Scan(&n)
	if err != nil {
		return fmt.Errorf("check athena primary workgroup: %w", err)
	}
	if n > 0 {
		return nil
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO athena_work_groups (
		   account_id, name, state, description, output_location, enforce_work_group_config, created_ms)
		 VALUES (?, ?, 'ENABLED', '', '', 0, ?)
		 ON CONFLICT(account_id, name) DO NOTHING`,
		accountID, DefaultAthenaWorkGroup, now,
	)
	if err != nil {
		return fmt.Errorf("seed athena primary workgroup: %w", err)
	}
	return nil
}

// CreateAthenaWorkGroup creates a workgroup. Name is required; duplicate Create fails closed.
func (s *Store) CreateAthenaWorkGroup(accountID string, in AthenaWorkGroupCreate) (AthenaWorkGroup, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return AthenaWorkGroup{}, fmt.Errorf("%w: Name is required", ErrAthenaBadRequest)
	}
	if err := s.ensureAthenaPrimaryWorkGroup(accountID); err != nil {
		return AthenaWorkGroup{}, err
	}
	if _, err := s.GetAthenaWorkGroup(accountID, name); err == nil {
		return AthenaWorkGroup{}, fmt.Errorf("%w: WorkGroup already exists", ErrAthenaBadRequest)
	} else if err != nil && !errors.Is(err, ErrAthenaNotFound) {
		return AthenaWorkGroup{}, err
	}
	now := time.Now().UTC().UnixMilli()
	wg := AthenaWorkGroup{
		Name:                   name,
		State:                  "ENABLED",
		Description:            strings.TrimSpace(in.Description),
		OutputLocation:         strings.TrimSpace(in.OutputLocation),
		EnforceWorkGroupConfig: in.EnforceWorkGroupConfig,
		CreatedMS:              now,
	}
	enforce := 0
	if wg.EnforceWorkGroupConfig {
		enforce = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO athena_work_groups (
		   account_id, name, state, description, output_location, enforce_work_group_config, created_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, wg.Name, wg.State, wg.Description, wg.OutputLocation, enforce, wg.CreatedMS,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return AthenaWorkGroup{}, fmt.Errorf("%w: WorkGroup already exists", ErrAthenaBadRequest)
		}
		return AthenaWorkGroup{}, fmt.Errorf("create athena workgroup: %w", err)
	}
	return wg, nil
}

// GetAthenaWorkGroup returns a workgroup by name. Missing primary is treated as implicit ENABLED.
func (s *Store) GetAthenaWorkGroup(accountID, name string) (AthenaWorkGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultAthenaWorkGroup
	}
	if err := s.ensureAthenaPrimaryWorkGroup(accountID); err != nil {
		return AthenaWorkGroup{}, err
	}
	var wg AthenaWorkGroup
	var enforce int
	err := s.db.QueryRow(
		`SELECT name, state, description, output_location, enforce_work_group_config, created_ms
		 FROM athena_work_groups WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&wg.Name, &wg.State, &wg.Description, &wg.OutputLocation, &enforce, &wg.CreatedMS)
	if errors.Is(err, sql.ErrNoRows) {
		if name == DefaultAthenaWorkGroup {
			return AthenaWorkGroup{
				Name:  DefaultAthenaWorkGroup,
				State: "ENABLED",
			}, nil
		}
		return AthenaWorkGroup{}, fmt.Errorf("%w: WorkGroup %s is not found.", ErrAthenaNotFound, name)
	}
	if err != nil {
		return AthenaWorkGroup{}, fmt.Errorf("get athena workgroup: %w", err)
	}
	wg.EnforceWorkGroupConfig = enforce != 0
	return wg, nil
}

// ListAthenaWorkGroups returns workgroups for the account (primary is always present).
func (s *Store) ListAthenaWorkGroups(accountID string) ([]AthenaWorkGroup, error) {
	if err := s.ensureAthenaPrimaryWorkGroup(accountID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT name, state, description, output_location, enforce_work_group_config, created_ms
		 FROM athena_work_groups WHERE account_id = ? ORDER BY name ASC`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list athena workgroups: %w", err)
	}
	defer rows.Close()
	var out []AthenaWorkGroup
	for rows.Next() {
		var wg AthenaWorkGroup
		var enforce int
		if err := rows.Scan(&wg.Name, &wg.State, &wg.Description, &wg.OutputLocation, &enforce, &wg.CreatedMS); err != nil {
			return nil, fmt.Errorf("scan athena workgroup: %w", err)
		}
		wg.EnforceWorkGroupConfig = enforce != 0
		out = append(out, wg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list athena workgroups: %w", err)
	}
	if out == nil {
		out = []AthenaWorkGroup{}
	}
	return out, nil
}

// DeleteAthenaWorkGroup deletes a workgroup. primary cannot be deleted.
func (s *Store) DeleteAthenaWorkGroup(accountID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: WorkGroup is required", ErrAthenaBadRequest)
	}
	if name == DefaultAthenaWorkGroup {
		return fmt.Errorf("%w: The primary workgroup cannot be deleted.", ErrAthenaBadRequest)
	}
	if err := s.ensureAthenaPrimaryWorkGroup(accountID); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`DELETE FROM athena_work_groups WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete athena workgroup: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: WorkGroup %s is not found.", ErrAthenaNotFound, name)
	}
	return nil
}

// UpdateAthenaWorkGroup updates description, state, and/or configuration fields.
func (s *Store) UpdateAthenaWorkGroup(accountID, name string, in AthenaWorkGroupUpdate) (AthenaWorkGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return AthenaWorkGroup{}, fmt.Errorf("%w: WorkGroup is required", ErrAthenaBadRequest)
	}
	wg, err := s.GetAthenaWorkGroup(accountID, name)
	if err != nil {
		return AthenaWorkGroup{}, err
	}
	if in.Description != nil {
		wg.Description = strings.TrimSpace(*in.Description)
	}
	if in.State != nil {
		st := strings.ToUpper(strings.TrimSpace(*in.State))
		if st != "ENABLED" && st != "DISABLED" {
			return AthenaWorkGroup{}, fmt.Errorf("%w: State must be ENABLED or DISABLED", ErrAthenaBadRequest)
		}
		wg.State = st
	}
	if in.RemoveOutputLocation {
		wg.OutputLocation = ""
	} else if in.OutputLocation != nil {
		wg.OutputLocation = strings.TrimSpace(*in.OutputLocation)
	}
	if in.EnforceWorkGroupConfig != nil {
		wg.EnforceWorkGroupConfig = *in.EnforceWorkGroupConfig
	}
	enforce := 0
	if wg.EnforceWorkGroupConfig {
		enforce = 1
	}
	_, err = s.db.Exec(
		`INSERT INTO athena_work_groups (
		   account_id, name, state, description, output_location, enforce_work_group_config, created_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, name) DO UPDATE SET
		   state = excluded.state,
		   description = excluded.description,
		   output_location = excluded.output_location,
		   enforce_work_group_config = excluded.enforce_work_group_config`,
		accountID, wg.Name, wg.State, wg.Description, wg.OutputLocation, enforce, wg.CreatedMS,
	)
	if err != nil {
		return AthenaWorkGroup{}, fmt.Errorf("update athena workgroup: %w", err)
	}
	return wg, nil
}

// applyAthenaWorkGroupStart loads the workgroup for StartQueryExecution and applies
// OutputLocation / DISABLED rules. Missing primary is treated as implicit ENABLED.
func (s *Store) applyAthenaWorkGroupStart(accountID string, in *AthenaStartInput) error {
	if in == nil {
		return fmt.Errorf("%w: Start input is required", ErrAthenaBadRequest)
	}
	wgName := strings.TrimSpace(in.WorkGroup)
	if wgName == "" {
		wgName = DefaultAthenaWorkGroup
	}
	in.WorkGroup = wgName

	wg, err := s.GetAthenaWorkGroup(accountID, wgName)
	if err != nil {
		if errors.Is(err, ErrAthenaNotFound) {
			return fmt.Errorf("%w: WorkGroup %s is not found.", ErrAthenaBadRequest, wgName)
		}
		return err
	}
	if strings.EqualFold(wg.State, "DISABLED") {
		return fmt.Errorf("%w: WorkGroup %s is DISABLED", ErrAthenaBadRequest, wgName)
	}
	reqOut := strings.TrimSpace(in.OutputLocation)
	if wg.EnforceWorkGroupConfig {
		if wg.OutputLocation != "" {
			in.OutputLocation = wg.OutputLocation
		}
	} else if reqOut == "" && wg.OutputLocation != "" {
		in.OutputLocation = wg.OutputLocation
	} else {
		in.OutputLocation = reqOut
	}
	return nil
}
