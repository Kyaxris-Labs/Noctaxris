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

// ASGScalingPolicy is a stored Auto Scaling scaling policy.
type ASGScalingPolicy struct {
	PolicyName              string
	PolicyARN               string
	AutoScalingGroupName    string
	PolicyType              string
	AdjustmentType          string
	ScalingAdjustment       int
	Cooldown                int
	EstimatedInstanceWarmup *int
	TargetTracking          *ASGTargetTrackingConfiguration
	Region                  string
}

// ASGTargetTrackingConfiguration is the TargetTrackingScaling subset.
type ASGTargetTrackingConfiguration struct {
	TargetValue          float64
	PredefinedMetricType string
	ResourceLabel        string
}

// ASGLifecycleHook is a stored lifecycle hook.
type ASGLifecycleHook struct {
	LifecycleHookName     string
	AutoScalingGroupName  string
	LifecycleTransition   string
	NotificationTargetARN string
	RoleARN               string
	NotificationMetadata  string
	HeartbeatTimeout      int
	GlobalTimeout         int
	DefaultResult         string
}

// ASGDescribedInstance is an ASG member for DescribeAutoScalingInstances.
type ASGDescribedInstance struct {
	InstanceID              string
	AutoScalingGroupName    string
	LifecycleState          string
	HealthStatus            string
	AvailabilityZone        string
	ProtectedFromScaleIn    bool
	LaunchConfigurationName string
}

func ASGScalingPolicyARN(region, accountID, asgName, policyName string) string {
	if region == "" {
		region = DefaultASGRegion
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	return fmt.Sprintf(
		"arn:aws:autoscaling:%s:%s:scalingPolicy:%s:autoScalingGroupName/%s:policyName/%s",
		region, accountID, id, asgName, policyName,
	)
}

func (s *Store) requireASG(accountID, region, name string) (AutoScalingGroup, error) {
	groups, err := s.DescribeAutoScalingGroups(accountID, region, []string{name})
	if err != nil {
		return AutoScalingGroup{}, err
	}
	if len(groups) == 0 {
		return AutoScalingGroup{}, ErrASGNotFound
	}
	return groups[0], nil
}

func (s *Store) clearASGPoliciesHooksTGs(accountID, region, asgName string) error {
	asgName = strings.TrimSpace(asgName)
	for _, q := range []string{
		`DELETE FROM asg_scaling_policies WHERE account_id = ? AND region = ? AND asg_name = ?`,
		`DELETE FROM asg_lifecycle_hooks WHERE account_id = ? AND region = ? AND asg_name = ?`,
		`DELETE FROM asg_target_groups WHERE account_id = ? AND region = ? AND asg_name = ?`,
	} {
		if _, err := s.db.Exec(q, accountID, region, asgName); err != nil {
			return fmt.Errorf("clear asg related rows: %w", err)
		}
	}
	return nil
}

func (s *Store) listASGTargetGroupARNs(accountID, region, asgName string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT target_group_arn FROM asg_target_groups
		 WHERE account_id = ? AND region = ? AND asg_name = ?
		 ORDER BY attached_at, target_group_arn`,
		accountID, region, strings.TrimSpace(asgName),
	)
	if err != nil {
		return nil, fmt.Errorf("list asg target groups: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var arn string
		if err := rows.Scan(&arn); err != nil {
			return nil, fmt.Errorf("list asg target groups scan: %w", err)
		}
		out = append(out, arn)
	}
	return out, rows.Err()
}

// PutScalingPolicy creates or updates a scaling policy (SimpleScaling / TargetTrackingScaling).
func (s *Store) PutScalingPolicy(accountID, region, asgName, policyName, policyType, adjustmentType string,
	scalingAdjustment, cooldown int, estimatedWarmup *int, tt *ASGTargetTrackingConfiguration,
) (ASGScalingPolicy, error) {
	if region == "" {
		region = DefaultASGRegion
	}
	asgName = strings.TrimSpace(asgName)
	policyName = strings.TrimSpace(policyName)
	if asgName == "" || policyName == "" {
		return ASGScalingPolicy{}, fmt.Errorf("%w: AutoScalingGroupName and PolicyName required", ErrASGBadRequest)
	}
	if _, err := s.requireASG(accountID, region, asgName); err != nil {
		return ASGScalingPolicy{}, err
	}
	if policyType == "" {
		policyType = "SimpleScaling"
	}
	if cooldown <= 0 {
		cooldown = 300
	}
	ttJSON := ""
	if tt != nil {
		b, err := json.Marshal(tt)
		if err != nil {
			return ASGScalingPolicy{}, fmt.Errorf("marshal target tracking: %w", err)
		}
		ttJSON = string(b)
	}
	now := time.Now().UTC().UnixMilli()
	existing, err := s.DescribePolicies(accountID, region, asgName, []string{policyName})
	if err != nil {
		return ASGScalingPolicy{}, err
	}
	arn := ASGScalingPolicyARN(region, accountID, asgName, policyName)
	if len(existing) == 1 {
		arn = existing[0].PolicyARN
		_, err = s.db.Exec(
			`UPDATE asg_scaling_policies SET policy_type = ?, adjustment_type = ?, scaling_adjustment = ?,
			 cooldown = ?, estimated_instance_warmup = ?, target_tracking_json = ?
			 WHERE account_id = ? AND region = ? AND asg_name = ? AND policy_name = ?`,
			policyType, adjustmentType, scalingAdjustment, cooldown, nullableInt(estimatedWarmup), ttJSON,
			accountID, region, asgName, policyName,
		)
	} else {
		_, err = s.db.Exec(
			`INSERT INTO asg_scaling_policies
			 (account_id, region, asg_name, policy_name, policy_arn, policy_type, adjustment_type,
			  scaling_adjustment, cooldown, estimated_instance_warmup, target_tracking_json, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			accountID, region, asgName, policyName, arn, policyType, adjustmentType,
			scalingAdjustment, cooldown, nullableInt(estimatedWarmup), ttJSON, now,
		)
	}
	if err != nil {
		return ASGScalingPolicy{}, fmt.Errorf("put scaling policy: %w", err)
	}
	return ASGScalingPolicy{
		PolicyName: policyName, PolicyARN: arn, AutoScalingGroupName: asgName,
		PolicyType: policyType, AdjustmentType: adjustmentType, ScalingAdjustment: scalingAdjustment,
		Cooldown: cooldown, EstimatedInstanceWarmup: estimatedWarmup, TargetTracking: tt, Region: region,
	}, nil
}

func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

// DescribePolicies lists scaling policies, optionally filtered by group and/or names.
func (s *Store) DescribePolicies(accountID, region, asgName string, policyNames []string) ([]ASGScalingPolicy, error) {
	if region == "" {
		region = DefaultASGRegion
	}
	rows, err := s.db.Query(
		`SELECT asg_name, policy_name, policy_arn, policy_type, adjustment_type, scaling_adjustment,
		        cooldown, estimated_instance_warmup, target_tracking_json, region
		 FROM asg_scaling_policies WHERE account_id = ? AND region = ? ORDER BY created_at, policy_name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("describe policies: %w", err)
	}
	defer rows.Close()
	asgName = strings.TrimSpace(asgName)
	want := map[string]struct{}{}
	for _, n := range policyNames {
		n = strings.TrimSpace(n)
		if n != "" {
			want[n] = struct{}{}
		}
	}
	var out []ASGScalingPolicy
	for rows.Next() {
		var p ASGScalingPolicy
		var warmup sql.NullInt64
		var ttJSON string
		if err := rows.Scan(&p.AutoScalingGroupName, &p.PolicyName, &p.PolicyARN, &p.PolicyType,
			&p.AdjustmentType, &p.ScalingAdjustment, &p.Cooldown, &warmup, &ttJSON, &p.Region); err != nil {
			return nil, fmt.Errorf("describe policies scan: %w", err)
		}
		if asgName != "" && p.AutoScalingGroupName != asgName {
			continue
		}
		if len(want) > 0 {
			if _, ok := want[p.PolicyName]; !ok {
				continue
			}
		}
		if warmup.Valid {
			v := int(warmup.Int64)
			p.EstimatedInstanceWarmup = &v
		}
		if strings.TrimSpace(ttJSON) != "" {
			var tt ASGTargetTrackingConfiguration
			if err := json.Unmarshal([]byte(ttJSON), &tt); err == nil {
				p.TargetTracking = &tt
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePolicy deletes a scaling policy by name or ARN.
func (s *Store) DeletePolicy(accountID, region, asgName, policyNameOrARN string) error {
	if region == "" {
		region = DefaultASGRegion
	}
	asgName = strings.TrimSpace(asgName)
	policyNameOrARN = strings.TrimSpace(policyNameOrARN)
	if policyNameOrARN == "" {
		return fmt.Errorf("%w: PolicyName required", ErrASGBadRequest)
	}
	if asgName != "" {
		if _, err := s.requireASG(accountID, region, asgName); err != nil {
			return err
		}
	}
	res, err := s.db.Exec(
		`DELETE FROM asg_scaling_policies
		 WHERE account_id = ? AND region = ?
		   AND (policy_name = ? OR policy_arn = ?)
		   AND (? = '' OR asg_name = ?)`,
		accountID, region, policyNameOrARN, policyNameOrARN, asgName, asgName,
	)
	if err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}
	if n == 0 {
		return ErrASGNotFound
	}
	return nil
}

// PutLifecycleHook creates or updates a lifecycle hook.
func (s *Store) PutLifecycleHook(accountID, region, asgName, hookName, transition, notificationTargetARN,
	roleARN, notificationMetadata string, heartbeatTimeout *int, defaultResult string,
) (ASGLifecycleHook, error) {
	if region == "" {
		region = DefaultASGRegion
	}
	asgName = strings.TrimSpace(asgName)
	hookName = strings.TrimSpace(hookName)
	if asgName == "" || hookName == "" {
		return ASGLifecycleHook{}, fmt.Errorf("%w: AutoScalingGroupName and LifecycleHookName required", ErrASGBadRequest)
	}
	if _, err := s.requireASG(accountID, region, asgName); err != nil {
		return ASGLifecycleHook{}, err
	}
	timeout := 3600
	if heartbeatTimeout != nil && *heartbeatTimeout > 0 {
		timeout = *heartbeatTimeout
	}
	if defaultResult == "" {
		defaultResult = "ABANDON"
	}
	defaultResult = strings.ToUpper(strings.TrimSpace(defaultResult))
	if defaultResult != "CONTINUE" && defaultResult != "ABANDON" {
		return ASGLifecycleHook{}, fmt.Errorf("%w: DefaultResult must be CONTINUE or ABANDON", ErrASGBadRequest)
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO asg_lifecycle_hooks
		 (account_id, region, asg_name, hook_name, lifecycle_transition, notification_target_arn,
		  role_arn, notification_metadata, heartbeat_timeout, default_result, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, region, asg_name, hook_name) DO UPDATE SET
		   lifecycle_transition = excluded.lifecycle_transition,
		   notification_target_arn = CASE
		     WHEN excluded.notification_target_arn = '' THEN asg_lifecycle_hooks.notification_target_arn
		     ELSE excluded.notification_target_arn END,
		   role_arn = CASE
		     WHEN excluded.role_arn = '' THEN asg_lifecycle_hooks.role_arn
		     ELSE excluded.role_arn END,
		   notification_metadata = CASE
		     WHEN excluded.notification_metadata = '' THEN asg_lifecycle_hooks.notification_metadata
		     ELSE excluded.notification_metadata END,
		   heartbeat_timeout = excluded.heartbeat_timeout,
		   default_result = excluded.default_result`,
		accountID, region, asgName, hookName, strings.TrimSpace(transition),
		strings.TrimSpace(notificationTargetARN), strings.TrimSpace(roleARN),
		notificationMetadata, timeout, defaultResult, now,
	)
	if err != nil {
		return ASGLifecycleHook{}, fmt.Errorf("put lifecycle hook: %w", err)
	}
	return ASGLifecycleHook{
		LifecycleHookName: hookName, AutoScalingGroupName: asgName,
		LifecycleTransition: strings.TrimSpace(transition),
		NotificationTargetARN: strings.TrimSpace(notificationTargetARN),
		RoleARN: strings.TrimSpace(roleARN), NotificationMetadata: notificationMetadata,
		HeartbeatTimeout: timeout, GlobalTimeout: 172800, DefaultResult: defaultResult,
	}, nil
}

// DescribeLifecycleHooks lists lifecycle hooks for a group.
func (s *Store) DescribeLifecycleHooks(accountID, region, asgName string, hookNames []string) ([]ASGLifecycleHook, error) {
	if region == "" {
		region = DefaultASGRegion
	}
	asgName = strings.TrimSpace(asgName)
	if asgName == "" {
		return nil, fmt.Errorf("%w: AutoScalingGroupName required", ErrASGBadRequest)
	}
	if _, err := s.requireASG(accountID, region, asgName); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT hook_name, asg_name, lifecycle_transition, notification_target_arn, role_arn,
		        notification_metadata, heartbeat_timeout, default_result
		 FROM asg_lifecycle_hooks WHERE account_id = ? AND region = ? AND asg_name = ?
		 ORDER BY created_at, hook_name`,
		accountID, region, asgName,
	)
	if err != nil {
		return nil, fmt.Errorf("describe lifecycle hooks: %w", err)
	}
	defer rows.Close()
	want := map[string]struct{}{}
	for _, n := range hookNames {
		n = strings.TrimSpace(n)
		if n != "" {
			want[n] = struct{}{}
		}
	}
	var out []ASGLifecycleHook
	for rows.Next() {
		var h ASGLifecycleHook
		if err := rows.Scan(&h.LifecycleHookName, &h.AutoScalingGroupName, &h.LifecycleTransition,
			&h.NotificationTargetARN, &h.RoleARN, &h.NotificationMetadata, &h.HeartbeatTimeout, &h.DefaultResult); err != nil {
			return nil, fmt.Errorf("describe lifecycle hooks scan: %w", err)
		}
		if len(want) > 0 {
			if _, ok := want[h.LifecycleHookName]; !ok {
				continue
			}
		}
		h.GlobalTimeout = 172800
		out = append(out, h)
	}
	return out, rows.Err()
}

// DeleteLifecycleHook removes a lifecycle hook.
func (s *Store) DeleteLifecycleHook(accountID, region, asgName, hookName string) error {
	if region == "" {
		region = DefaultASGRegion
	}
	asgName = strings.TrimSpace(asgName)
	hookName = strings.TrimSpace(hookName)
	if asgName == "" || hookName == "" {
		return fmt.Errorf("%w: AutoScalingGroupName and LifecycleHookName required", ErrASGBadRequest)
	}
	if _, err := s.requireASG(accountID, region, asgName); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`DELETE FROM asg_lifecycle_hooks WHERE account_id = ? AND region = ? AND asg_name = ? AND hook_name = ?`,
		accountID, region, asgName, hookName,
	)
	if err != nil {
		return fmt.Errorf("delete lifecycle hook: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete lifecycle hook: %w", err)
	}
	if n == 0 {
		return ErrASGNotFound
	}
	return nil
}

// AttachASGInstances attaches existing lab EC2 instances to an ASG (no DinD required).
// Bumps DesiredCapacity by newly attached count; fails if that would exceed MaxSize.
func (s *Store) AttachASGInstances(accountID, region, asgName string, instanceIDs []string) error {
	if region == "" {
		region = DefaultASGRegion
	}
	asgName = strings.TrimSpace(asgName)
	if asgName == "" {
		return fmt.Errorf("%w: AutoScalingGroupName required", ErrASGBadRequest)
	}
	g, err := s.requireASG(accountID, region, asgName)
	if err != nil {
		return err
	}
	var toAttach []string
	seen := map[string]struct{}{}
	for _, id := range instanceIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		inst, err := s.GetEC2Instance(accountID, region, id)
		if err != nil {
			if errors.Is(err, ErrEC2NotFound) {
				return fmt.Errorf("%w: instance %s not found", ErrASGBadRequest, id)
			}
			return err
		}
		if inst.StateName == EC2StateTerminated {
			return fmt.Errorf("%w: instance %s is terminated", ErrASGBadRequest, id)
		}
		other, err := s.findASGForInstance(accountID, region, id)
		if err != nil {
			return err
		}
		if other != "" && other != asgName {
			return fmt.Errorf("%w: instance %s is already in Auto Scaling group %s", ErrASGBadRequest, id, other)
		}
		if other == asgName {
			continue
		}
		toAttach = append(toAttach, id)
	}
	if len(toAttach) == 0 {
		return nil
	}
	newDesired := g.DesiredCapacity + len(toAttach)
	if newDesired > g.MaxSize {
		return fmt.Errorf("%w: attaching instances would exceed MaxSize", ErrASGBadRequest)
	}
	for _, id := range toAttach {
		if err := s.attachASGInstance(accountID, region, asgName, id); err != nil {
			return err
		}
	}
	return s.SetDesiredCapacity(accountID, region, asgName, newDesired)
}

func (s *Store) findASGForInstance(accountID, region, instanceID string) (string, error) {
	var asgName string
	err := s.db.QueryRow(
		`SELECT asg_name FROM asg_instances WHERE account_id = ? AND region = ? AND instance_id = ? LIMIT 1`,
		accountID, region, strings.TrimSpace(instanceID),
	).Scan(&asgName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find asg for instance: %w", err)
	}
	return asgName, nil
}

// DetachASGInstances removes instances from ASG membership. Optionally decrements DesiredCapacity (floor MinSize).
func (s *Store) DetachASGInstances(accountID, region, asgName string, instanceIDs []string, decrementDesired bool) error {
	if region == "" {
		region = DefaultASGRegion
	}
	asgName = strings.TrimSpace(asgName)
	if asgName == "" {
		return fmt.Errorf("%w: AutoScalingGroupName required", ErrASGBadRequest)
	}
	g, err := s.requireASG(accountID, region, asgName)
	if err != nil {
		return err
	}
	var detached int
	for _, id := range instanceIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		members, err := s.listASGMemberInstanceIDs(accountID, region, asgName)
		if err != nil {
			return err
		}
		found := false
		for _, m := range members {
			if m == id {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		if err := s.detachASGInstance(accountID, region, asgName, id); err != nil {
			return err
		}
		detached++
	}
	if decrementDesired && detached > 0 {
		newDesired := g.DesiredCapacity - detached
		if newDesired < g.MinSize {
			newDesired = g.MinSize
		}
		return s.SetDesiredCapacity(accountID, region, asgName, newDesired)
	}
	return nil
}

// DescribeAutoScalingInstances lists ASG members, optionally filtered by instance IDs.
func (s *Store) DescribeAutoScalingInstances(accountID, region string, instanceIDs []string) ([]ASGDescribedInstance, error) {
	if region == "" {
		region = DefaultASGRegion
	}
	rows, err := s.db.Query(
		`SELECT i.asg_name, i.instance_id, g.launch_configuration_name
		 FROM asg_instances i
		 JOIN asg_groups g ON g.account_id = i.account_id AND g.region = i.region AND g.name = i.asg_name
		 WHERE i.account_id = ? AND i.region = ?
		 ORDER BY i.attached_at, i.instance_id`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("describe auto scaling instances: %w", err)
	}
	defer rows.Close()
	want := map[string]struct{}{}
	for _, id := range instanceIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = struct{}{}
		}
	}
	var out []ASGDescribedInstance
	for rows.Next() {
		var asgName, instanceID, lcName string
		if err := rows.Scan(&asgName, &instanceID, &lcName); err != nil {
			return nil, fmt.Errorf("describe auto scaling instances scan: %w", err)
		}
		if len(want) > 0 {
			if _, ok := want[instanceID]; !ok {
				continue
			}
		}
		inst, err := s.GetEC2Instance(accountID, region, instanceID)
		if err != nil {
			if errors.Is(err, ErrEC2NotFound) {
				continue
			}
			return nil, err
		}
		if inst.StateName == EC2StateTerminated {
			continue
		}
		out = append(out, ASGDescribedInstance{
			InstanceID:              instanceID,
			AutoScalingGroupName:    asgName,
			LifecycleState:          asgLifecycleForEC2State(inst.StateName),
			HealthStatus:            "Healthy",
			AvailabilityZone:        inst.AvailabilityZone,
			LaunchConfigurationName: lcName,
		})
	}
	return out, rows.Err()
}

// AttachLoadBalancerTargetGroups attaches ELBv2 target group ARNs to an ASG.
// Validates each ARN exists in the ELBv2 store when present.
func (s *Store) AttachLoadBalancerTargetGroups(accountID, region, asgName string, tgARNs []string) error {
	if region == "" {
		region = DefaultASGRegion
	}
	asgName = strings.TrimSpace(asgName)
	if asgName == "" {
		return fmt.Errorf("%w: AutoScalingGroupName required", ErrASGBadRequest)
	}
	if _, err := s.requireASG(accountID, region, asgName); err != nil {
		return err
	}
	if len(tgARNs) == 0 {
		return fmt.Errorf("%w: TargetGroupARNs required", ErrASGBadRequest)
	}
	now := time.Now().UTC().UnixMilli()
	for _, arn := range tgARNs {
		arn = strings.TrimSpace(arn)
		if arn == "" {
			continue
		}
		tgs, err := s.DescribeELBv2TargetGroups(accountID, []string{arn})
		if err != nil {
			return err
		}
		if len(tgs) == 0 {
			return fmt.Errorf("%w: target group %s not found", ErrASGBadRequest, arn)
		}
		_, err = s.db.Exec(
			`INSERT OR IGNORE INTO asg_target_groups (account_id, region, asg_name, target_group_arn, attached_at)
			 VALUES (?, ?, ?, ?, ?)`,
			accountID, region, asgName, arn, now,
		)
		if err != nil {
			return fmt.Errorf("attach load balancer target group: %w", err)
		}
	}
	return nil
}

// DetachLoadBalancerTargetGroups detaches ELBv2 target group ARNs from an ASG.
func (s *Store) DetachLoadBalancerTargetGroups(accountID, region, asgName string, tgARNs []string) error {
	if region == "" {
		region = DefaultASGRegion
	}
	asgName = strings.TrimSpace(asgName)
	if asgName == "" {
		return fmt.Errorf("%w: AutoScalingGroupName required", ErrASGBadRequest)
	}
	if _, err := s.requireASG(accountID, region, asgName); err != nil {
		return err
	}
	for _, arn := range tgARNs {
		arn = strings.TrimSpace(arn)
		if arn == "" {
			continue
		}
		_, err := s.db.Exec(
			`DELETE FROM asg_target_groups WHERE account_id = ? AND region = ? AND asg_name = ? AND target_group_arn = ?`,
			accountID, region, asgName, arn,
		)
		if err != nil {
			return fmt.Errorf("detach load balancer target group: %w", err)
		}
	}
	return nil
}

// DescribeLoadBalancerTargetGroups lists target group ARNs attached to an ASG.
func (s *Store) DescribeLoadBalancerTargetGroups(accountID, region, asgName string) ([]string, error) {
	if region == "" {
		region = DefaultASGRegion
	}
	asgName = strings.TrimSpace(asgName)
	if asgName == "" {
		return nil, fmt.Errorf("%w: AutoScalingGroupName required", ErrASGBadRequest)
	}
	if _, err := s.requireASG(accountID, region, asgName); err != nil {
		return nil, err
	}
	return s.listASGTargetGroupARNs(accountID, region, asgName)
}
