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
	ErrASGNotFound   = errors.New("ValidationError")
	ErrASGExists     = errors.New("AlreadyExists")
	ErrASGBadRequest = errors.New("ValidationError")
)

const DefaultASGRegion = "us-east-1"

const asgSchema = `
CREATE TABLE IF NOT EXISTS asg_launch_configurations (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  image_id TEXT NOT NULL,
  instance_type TEXT NOT NULL,
  key_name TEXT NOT NULL DEFAULT '',
  security_groups TEXT NOT NULL DEFAULT '[]',
  user_data TEXT NOT NULL DEFAULT '',
  iam_instance_profile TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, name)
);
CREATE TABLE IF NOT EXISTS asg_groups (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  launch_configuration_name TEXT NOT NULL DEFAULT '',
  min_size INTEGER NOT NULL,
  max_size INTEGER NOT NULL,
  desired_capacity INTEGER NOT NULL,
  availability_zones TEXT NOT NULL DEFAULT '[]',
  vpc_zone_identifier TEXT NOT NULL DEFAULT '',
  health_check_type TEXT NOT NULL DEFAULT 'EC2',
  default_cooldown INTEGER NOT NULL DEFAULT 300,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, name)
);
CREATE TABLE IF NOT EXISTS asg_instances (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  asg_name TEXT NOT NULL,
  instance_id TEXT NOT NULL,
  attached_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, asg_name, instance_id)
);
CREATE INDEX IF NOT EXISTS idx_asg_instances_lookup
  ON asg_instances(account_id, region, asg_name);
`

// ASGLaunchConfiguration is a launch configuration row.
type ASGLaunchConfiguration struct {
	LaunchConfigurationName string
	ARN                     string
	ImageID                 string
	InstanceType            string
	KeyName                 string
	SecurityGroups          string // JSON array string
	UserData                string
	IamInstanceProfile      string
	CreatedAt               int64
	Region                  string
}

// ASGInstance is a lab EC2 instance attached to an auto scaling group.
type ASGInstance struct {
	InstanceID           string
	LifecycleState       string
	HealthStatus         string
	AvailabilityZone     string
	ProtectedFromScaleIn bool
}

// AutoScalingGroup is an ASG row. Instances are reconciled to DesiredCapacity via lab EC2.
type AutoScalingGroup struct {
	AutoScalingGroupName    string
	ARN                     string
	LaunchConfigurationName string
	MinSize                 int
	MaxSize                 int
	DesiredCapacity         int
	AvailabilityZones       string // JSON array string
	VPCZoneIdentifier       string
	HealthCheckType         string
	DefaultCooldown         int
	CreatedAt               int64
	Region                  string
	Instances               []ASGInstance
}

// ClampASGDesired returns desired clamped into [minSize, maxSize].
func ClampASGDesired(desired, minSize, maxSize int) int {
	if maxSize < minSize {
		maxSize = minSize
	}
	if desired < minSize {
		return minSize
	}
	if desired > maxSize {
		return maxSize
	}
	return desired
}

func asgLifecycleForEC2State(state string) string {
	switch state {
	case EC2StateRunning:
		return "InService"
	case EC2StatePending, EC2StateShuttingDown, EC2StateStopping:
		return "Pending"
	case EC2StateTerminated:
		return "Terminated"
	case EC2StateStopped:
		return "Standby"
	default:
		return "Pending"
	}
}

func (s *Store) loadASGInstances(accountID, region, asgName string) ([]ASGInstance, error) {
	ids, err := s.listASGMemberInstanceIDs(accountID, region, asgName)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	ec2s, err := s.DescribeInstances(accountID, region, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]EC2Instance, len(ec2s))
	for _, inst := range ec2s {
		byID[inst.InstanceID] = inst
	}
	out := make([]ASGInstance, 0, len(ids))
	for _, id := range ids {
		inst, ok := byID[id]
		if !ok || inst.StateName == EC2StateTerminated {
			continue
		}
		out = append(out, ASGInstance{
			InstanceID:       inst.InstanceID,
			LifecycleState:   asgLifecycleForEC2State(inst.StateName),
			HealthStatus:     "Healthy",
			AvailabilityZone: inst.AvailabilityZone,
		})
	}
	return out, nil
}

func (s *Store) listASGMemberInstanceIDs(accountID, region, asgName string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT instance_id FROM asg_instances
		 WHERE account_id = ? AND region = ? AND asg_name = ?
		 ORDER BY attached_at, instance_id`,
		accountID, region, strings.TrimSpace(asgName),
	)
	if err != nil {
		return nil, fmt.Errorf("list asg member instances: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list asg member instances scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) attachASGInstance(accountID, region, asgName, instanceID string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO asg_instances (account_id, region, asg_name, instance_id, attached_at)
		 VALUES (?, ?, ?, ?, ?)`,
		accountID, region, strings.TrimSpace(asgName), strings.TrimSpace(instanceID),
		time.Now().UTC().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("attach asg instance: %w", err)
	}
	return nil
}

func (s *Store) detachASGInstance(accountID, region, asgName, instanceID string) error {
	_, err := s.db.Exec(
		`DELETE FROM asg_instances WHERE account_id = ? AND region = ? AND asg_name = ? AND instance_id = ?`,
		accountID, region, strings.TrimSpace(asgName), strings.TrimSpace(instanceID),
	)
	if err != nil {
		return fmt.Errorf("detach asg instance: %w", err)
	}
	return nil
}

func (s *Store) clearASGInstances(accountID, region, asgName string) error {
	_, err := s.db.Exec(
		`DELETE FROM asg_instances WHERE account_id = ? AND region = ? AND asg_name = ?`,
		accountID, region, strings.TrimSpace(asgName),
	)
	if err != nil {
		return fmt.Errorf("clear asg instances: %w", err)
	}
	return nil
}

// listASGActiveEC2 returns non-terminated EC2 instances currently attached to the ASG (oldest first).
func (s *Store) listASGActiveEC2(accountID, region, asgName string) ([]EC2Instance, error) {
	ids, err := s.listASGMemberInstanceIDs(accountID, region, asgName)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	ec2s, err := s.DescribeInstances(accountID, region, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]EC2Instance, len(ec2s))
	for _, inst := range ec2s {
		byID[inst.InstanceID] = inst
	}
	out := make([]EC2Instance, 0, len(ids))
	for _, id := range ids {
		inst, ok := byID[id]
		if !ok || inst.StateName == EC2StateTerminated {
			_ = s.detachASGInstance(accountID, region, asgName, id)
			continue
		}
		out = append(out, inst)
	}
	return out, nil
}

// firstASGAvailabilityZone returns the first AZ from a JSON array string, or empty.
func firstASGAvailabilityZone(azJSON string) string {
	azs := parseASGStringSlice(azJSON)
	if len(azs) == 0 {
		return ""
	}
	return azs[0]
}

func parseASGStringSlice(jsonArr string) []string {
	var out []string
	_ = json.Unmarshal([]byte(jsonArr), &out)
	return out
}

// EnsureASGSchema creates Auto Scaling tables if missing.
func EnsureASGSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure asg schema: db is nil")
	}
	if _, err := db.Exec(asgSchema); err != nil {
		return fmt.Errorf("ensure asg schema: %w", err)
	}
	return nil
}

// EnsureASGSchema ensures Auto Scaling tables on an open store.
func (s *Store) EnsureASGSchema() error {
	return EnsureASGSchema(s.db)
}

func ASGLaunchConfigurationARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultASGRegion
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	return fmt.Sprintf("arn:aws:autoscaling:%s:%s:launchConfiguration:%s:launchConfigurationName/%s", region, accountID, id, name)
}

func AutoScalingGroupARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultASGRegion
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	return fmt.Sprintf("arn:aws:autoscaling:%s:%s:autoScalingGroup:%s:autoScalingGroupName/%s", region, accountID, id, name)
}

// CreateLaunchConfiguration stores a launch configuration template.
func (s *Store) CreateLaunchConfiguration(accountID, region, name, imageID, instanceType, keyName, userData, iamProfile string, securityGroups []string) (ASGLaunchConfiguration, error) {
	if region == "" {
		region = DefaultASGRegion
	}
	name = strings.TrimSpace(name)
	imageID = strings.TrimSpace(imageID)
	instanceType = strings.TrimSpace(instanceType)
	if name == "" || imageID == "" || instanceType == "" {
		return ASGLaunchConfiguration{}, fmt.Errorf("%w: LaunchConfigurationName, ImageId, and InstanceType required", ErrASGBadRequest)
	}
	sgJSON := "[]"
	if len(securityGroups) > 0 {
		parts := make([]string, 0, len(securityGroups))
		for _, g := range securityGroups {
			parts = append(parts, `"`+strings.ReplaceAll(g, `"`, ``)+`"`)
		}
		sgJSON = "[" + strings.Join(parts, ",") + "]"
	}
	now := time.Now().UTC().UnixMilli()
	arn := ASGLaunchConfigurationARN(region, accountID, name)
	_, err := s.db.Exec(
		`INSERT INTO asg_launch_configurations
		 (account_id, region, name, arn, image_id, instance_type, key_name, security_groups, user_data, iam_instance_profile, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, region, name, arn, imageID, instanceType, keyName, sgJSON, userData, iamProfile, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ASGLaunchConfiguration{}, ErrASGExists
		}
		return ASGLaunchConfiguration{}, fmt.Errorf("create launch configuration: %w", err)
	}
	return ASGLaunchConfiguration{
		LaunchConfigurationName: name, ARN: arn, ImageID: imageID, InstanceType: instanceType,
		KeyName: keyName, SecurityGroups: sgJSON, UserData: userData, IamInstanceProfile: iamProfile,
		CreatedAt: now, Region: region,
	}, nil
}

// DescribeLaunchConfigurations lists launch configurations, optionally filtered by names.
func (s *Store) DescribeLaunchConfigurations(accountID, region string, names []string) ([]ASGLaunchConfiguration, error) {
	if region == "" {
		region = DefaultASGRegion
	}
	rows, err := s.db.Query(
		`SELECT name, arn, image_id, instance_type, key_name, security_groups, user_data, iam_instance_profile, created_at, region
		 FROM asg_launch_configurations WHERE account_id = ? AND region = ? ORDER BY created_at, name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("describe launch configurations: %w", err)
	}
	defer rows.Close()
	want := map[string]struct{}{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			want[n] = struct{}{}
		}
	}
	var out []ASGLaunchConfiguration
	for rows.Next() {
		var lc ASGLaunchConfiguration
		if err := rows.Scan(&lc.LaunchConfigurationName, &lc.ARN, &lc.ImageID, &lc.InstanceType, &lc.KeyName,
			&lc.SecurityGroups, &lc.UserData, &lc.IamInstanceProfile, &lc.CreatedAt, &lc.Region); err != nil {
			return nil, fmt.Errorf("describe launch configurations scan: %w", err)
		}
		if len(want) > 0 {
			if _, ok := want[lc.LaunchConfigurationName]; !ok {
				continue
			}
		}
		out = append(out, lc)
	}
	return out, rows.Err()
}

// DeleteLaunchConfiguration deletes a launch configuration.
func (s *Store) DeleteLaunchConfiguration(accountID, region, name string) error {
	if region == "" {
		region = DefaultASGRegion
	}
	res, err := s.db.Exec(
		`DELETE FROM asg_launch_configurations WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, strings.TrimSpace(name),
	)
	if err != nil {
		return fmt.Errorf("delete launch configuration: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete launch configuration: %w", err)
	}
	if n == 0 {
		return ErrASGNotFound
	}
	return nil
}

// CreateAutoScalingGroup creates a group. Call ReconcileASGCapacity to match DesiredCapacity.
func (s *Store) CreateAutoScalingGroup(accountID, region, name, lcName string, minSize, maxSize, desired int, azs []string, vpcZones, healthCheck string, cooldown int) (AutoScalingGroup, error) {
	if region == "" {
		region = DefaultASGRegion
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return AutoScalingGroup{}, fmt.Errorf("%w: AutoScalingGroupName required", ErrASGBadRequest)
	}
	desired = ClampASGDesired(desired, minSize, maxSize)
	if minSize < 0 || maxSize < minSize {
		return AutoScalingGroup{}, fmt.Errorf("%w: invalid capacity bounds", ErrASGBadRequest)
	}
	if healthCheck == "" {
		healthCheck = "EC2"
	}
	if cooldown <= 0 {
		cooldown = 300
	}
	azJSON := "[]"
	if len(azs) > 0 {
		parts := make([]string, 0, len(azs))
		for _, z := range azs {
			parts = append(parts, `"`+z+`"`)
		}
		azJSON = "[" + strings.Join(parts, ",") + "]"
	}
	now := time.Now().UTC().UnixMilli()
	arn := AutoScalingGroupARN(region, accountID, name)
	_, err := s.db.Exec(
		`INSERT INTO asg_groups
		 (account_id, region, name, arn, launch_configuration_name, min_size, max_size, desired_capacity,
		  availability_zones, vpc_zone_identifier, health_check_type, default_cooldown, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, region, name, arn, strings.TrimSpace(lcName), minSize, maxSize, desired,
		azJSON, vpcZones, healthCheck, cooldown, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return AutoScalingGroup{}, ErrASGExists
		}
		return AutoScalingGroup{}, fmt.Errorf("create auto scaling group: %w", err)
	}
	return AutoScalingGroup{
		AutoScalingGroupName: name, ARN: arn, LaunchConfigurationName: strings.TrimSpace(lcName),
		MinSize: minSize, MaxSize: maxSize, DesiredCapacity: desired,
		AvailabilityZones: azJSON, VPCZoneIdentifier: vpcZones, HealthCheckType: healthCheck,
		DefaultCooldown: cooldown, CreatedAt: now, Region: region,
	}, nil
}

// DescribeAutoScalingGroups lists groups, optionally filtered by names.
func (s *Store) DescribeAutoScalingGroups(accountID, region string, names []string) ([]AutoScalingGroup, error) {
	if region == "" {
		region = DefaultASGRegion
	}
	rows, err := s.db.Query(
		`SELECT name, arn, launch_configuration_name, min_size, max_size, desired_capacity,
		        availability_zones, vpc_zone_identifier, health_check_type, default_cooldown, created_at, region
		 FROM asg_groups WHERE account_id = ? AND region = ? ORDER BY created_at, name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("describe auto scaling groups: %w", err)
	}
	defer rows.Close()
	want := map[string]struct{}{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			want[n] = struct{}{}
		}
	}
	var out []AutoScalingGroup
	for rows.Next() {
		var g AutoScalingGroup
		if err := rows.Scan(&g.AutoScalingGroupName, &g.ARN, &g.LaunchConfigurationName, &g.MinSize, &g.MaxSize,
			&g.DesiredCapacity, &g.AvailabilityZones, &g.VPCZoneIdentifier, &g.HealthCheckType, &g.DefaultCooldown,
			&g.CreatedAt, &g.Region); err != nil {
			return nil, fmt.Errorf("describe auto scaling groups scan: %w", err)
		}
		if len(want) > 0 {
			if _, ok := want[g.AutoScalingGroupName]; !ok {
				continue
			}
		}
		insts, err := s.loadASGInstances(accountID, region, g.AutoScalingGroupName)
		if err != nil {
			return nil, err
		}
		g.Instances = insts
		out = append(out, g)
	}
	return out, rows.Err()
}

// UpdateAutoScalingGroup updates capacity bounds and optional launch config / AZs.
func (s *Store) UpdateAutoScalingGroup(accountID, region, name string, minSize, maxSize, desired *int, lcName *string, azs []string, cooldown *int) (AutoScalingGroup, error) {
	groups, err := s.DescribeAutoScalingGroups(accountID, region, []string{name})
	if err != nil {
		return AutoScalingGroup{}, err
	}
	if len(groups) == 0 {
		return AutoScalingGroup{}, ErrASGNotFound
	}
	g := groups[0]
	if minSize != nil {
		g.MinSize = *minSize
	}
	if maxSize != nil {
		g.MaxSize = *maxSize
	}
	if desired != nil {
		g.DesiredCapacity = *desired
	}
	if lcName != nil {
		g.LaunchConfigurationName = strings.TrimSpace(*lcName)
	}
	if cooldown != nil && *cooldown > 0 {
		g.DefaultCooldown = *cooldown
	}
	if len(azs) > 0 {
		parts := make([]string, 0, len(azs))
		for _, z := range azs {
			parts = append(parts, `"`+z+`"`)
		}
		g.AvailabilityZones = "[" + strings.Join(parts, ",") + "]"
	}
	if g.MinSize < 0 || g.MaxSize < g.MinSize || g.DesiredCapacity < g.MinSize || g.DesiredCapacity > g.MaxSize {
		return AutoScalingGroup{}, fmt.Errorf("%w: invalid capacity bounds", ErrASGBadRequest)
	}
	_, err = s.db.Exec(
		`UPDATE asg_groups SET launch_configuration_name = ?, min_size = ?, max_size = ?, desired_capacity = ?,
		 availability_zones = ?, default_cooldown = ? WHERE account_id = ? AND region = ? AND name = ?`,
		g.LaunchConfigurationName, g.MinSize, g.MaxSize, g.DesiredCapacity, g.AvailabilityZones, g.DefaultCooldown,
		accountID, region, strings.TrimSpace(name),
	)
	if err != nil {
		return AutoScalingGroup{}, fmt.Errorf("update auto scaling group: %w", err)
	}
	return g, nil
}

// SetDesiredCapacity updates DesiredCapacity within min/max. Call ReconcileASGCapacity to apply.
func (s *Store) SetDesiredCapacity(accountID, region, name string, desired int) error {
	groups, err := s.DescribeAutoScalingGroups(accountID, region, []string{name})
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return ErrASGNotFound
	}
	g := groups[0]
	if desired < g.MinSize || desired > g.MaxSize {
		return fmt.Errorf("%w: DesiredCapacity out of min/max bounds", ErrASGBadRequest)
	}
	_, err = s.db.Exec(
		`UPDATE asg_groups SET desired_capacity = ? WHERE account_id = ? AND region = ? AND name = ?`,
		desired, accountID, region, strings.TrimSpace(name),
	)
	if err != nil {
		return fmt.Errorf("set desired capacity: %w", err)
	}
	return nil
}

// ReconcileASGCapacity launches or terminates lab EC2 instances so non-terminated
// ASG membership matches ClampASGDesired(DesiredCapacity, MinSize, MaxSize).
// Uses store.RunInstances (pending rows). Nested DinD start is owned by EC2 handlers;
// without the engine, membership stays Pending (DesiredCapacity is still stored).
func (s *Store) ReconcileASGCapacity(accountID, region, name string) (AutoScalingGroup, error) {
	if region == "" {
		region = DefaultASGRegion
	}
	name = strings.TrimSpace(name)
	groups, err := s.DescribeAutoScalingGroups(accountID, region, []string{name})
	if err != nil {
		return AutoScalingGroup{}, err
	}
	if len(groups) == 0 {
		return AutoScalingGroup{}, ErrASGNotFound
	}
	g := groups[0]
	target := ClampASGDesired(g.DesiredCapacity, g.MinSize, g.MaxSize)
	if target != g.DesiredCapacity {
		if err := s.SetDesiredCapacity(accountID, region, name, target); err != nil {
			return AutoScalingGroup{}, err
		}
		g.DesiredCapacity = target
	}

	current, err := s.listASGActiveEC2(accountID, region, name)
	if err != nil {
		return AutoScalingGroup{}, err
	}
	for len(current) > target {
		victim := current[0]
		current = current[1:]
		if err := s.terminateASGMember(accountID, region, name, victim.InstanceID); err != nil {
			return AutoScalingGroup{}, err
		}
	}
	need := target - len(current)
	if need > 0 {
		imageID := "ami-alpine"
		instanceType := "t3.micro"
		keyName := ""
		userData := ""
		iamProfile := ""
		if g.LaunchConfigurationName != "" {
			lcs, err := s.DescribeLaunchConfigurations(accountID, region, []string{g.LaunchConfigurationName})
			if err != nil {
				return AutoScalingGroup{}, err
			}
			if len(lcs) == 1 {
				imageID = lcs[0].ImageID
				instanceType = lcs[0].InstanceType
				keyName = lcs[0].KeyName
				userData = lcs[0].UserData
				iamProfile = lcs[0].IamInstanceProfile
			}
		}
		az := firstASGAvailabilityZone(g.AvailabilityZones)
		launched, err := s.RunInstances(accountID, region, RunInstancesInput{
			ImageID:            imageID,
			InstanceType:       instanceType,
			MinCount:           need,
			MaxCount:           need,
			KeyName:            keyName,
			UserData:           userData,
			IamInstanceProfile: iamProfile,
			AvailabilityZone:   az,
		})
		if err != nil {
			return AutoScalingGroup{}, err
		}
		for _, inst := range launched {
			if err := s.attachASGInstance(accountID, region, name, inst.InstanceID); err != nil {
				return AutoScalingGroup{}, err
			}
		}
	}
	groups, err = s.DescribeAutoScalingGroups(accountID, region, []string{name})
	if err != nil {
		return AutoScalingGroup{}, err
	}
	if len(groups) == 0 {
		return AutoScalingGroup{}, ErrASGNotFound
	}
	return groups[0], nil
}

func (s *Store) terminateASGMember(accountID, region, asgName, instanceID string) error {
	if err := s.SetEC2InstanceState(accountID, region, instanceID, EC2StateTerminated); err != nil && !errors.Is(err, ErrEC2NotFound) {
		return err
	}
	_ = s.ClearEC2ContainerID(accountID, region, instanceID)
	return s.detachASGInstance(accountID, region, asgName, instanceID)
}

// ASGGroupAccount carries account + group for background reconcile.
type ASGGroupAccount struct {
	AccountID string
	Region    string
	Group     AutoScalingGroup
}

// ListASGGroupsWithAccount returns all ASG groups with owning account and region.
func (s *Store) ListASGGroupsWithAccount() ([]ASGGroupAccount, error) {
	rows, err := s.db.Query(
		`SELECT account_id, region, name FROM asg_groups ORDER BY account_id, region, name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list asg groups: %w", err)
	}
	defer rows.Close()
	var out []ASGGroupAccount
	for rows.Next() {
		var acct, region, name string
		if err := rows.Scan(&acct, &region, &name); err != nil {
			return nil, fmt.Errorf("list asg groups scan: %w", err)
		}
		groups, err := s.DescribeAutoScalingGroups(acct, region, []string{name})
		if err != nil {
			return nil, err
		}
		if len(groups) == 0 {
			continue
		}
		out = append(out, ASGGroupAccount{AccountID: acct, Region: region, Group: groups[0]})
	}
	return out, rows.Err()
}

// DeleteAutoScalingGroup deletes a group. ForceDelete terminates attached lab EC2 instances first.
// Without ForceDelete, groups that still have non-terminated instances are rejected.
func (s *Store) DeleteAutoScalingGroup(accountID, region, name string, forceDelete bool) error {
	if region == "" {
		region = DefaultASGRegion
	}
	name = strings.TrimSpace(name)
	groups, err := s.DescribeAutoScalingGroups(accountID, region, []string{name})
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return ErrASGNotFound
	}
	insts, err := s.listASGActiveEC2(accountID, region, name)
	if err != nil {
		return err
	}
	if len(insts) > 0 {
		if !forceDelete {
			return fmt.Errorf("%w: AutoScalingGroup still has instances; set ForceDelete=true", ErrASGBadRequest)
		}
		for _, inst := range insts {
			if err := s.terminateASGMember(accountID, region, name, inst.InstanceID); err != nil {
				return err
			}
		}
	}
	if err := s.clearASGInstances(accountID, region, name); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`DELETE FROM asg_groups WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, name,
	)
	if err != nil {
		return fmt.Errorf("delete auto scaling group: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete auto scaling group: %w", err)
	}
	if n == 0 {
		return ErrASGNotFound
	}
	return nil
}
