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
	ErrCFNChangeSetNotFound = errors.New("ChangeSetNotFound")
	ErrCFNChangeSetFailed   = errors.New("InvalidChangeSetStatus")
)

// CFNChange is one proposed resource change.
type CFNChange struct {
	Action             string
	LogicalResourceID  string
	ResourceType       string
	PhysicalResourceID string
}

// CFNChangeSet is a persisted change set.
type CFNChangeSet struct {
	ChangeSetID   string
	ChangeSetName string
	StackName     string
	StackID       string
	Status        string
	StatusReason  string
	TemplateBody  string
	Changes       []CFNChange
	CreationTime  int64
}

func (s *Store) CreateCFNChangeSet(accountID, region, stackName, changeSetName, templateBody string) (CFNChangeSet, error) {
	stackName = strings.TrimSpace(stackName)
	changeSetName = strings.TrimSpace(changeSetName)
	templateBody = strings.TrimSpace(templateBody)
	if stackName == "" || changeSetName == "" || templateBody == "" {
		return CFNChangeSet{}, fmt.Errorf("%w: StackName, ChangeSetName, and TemplateBody are required", ErrCFNBadTemplate)
	}
	stacks, err := s.DescribeCFNStacks(accountID, stackName)
	if err != nil {
		return CFNChangeSet{}, err
	}
	st := stacks[0]
	tpl, err := parseCFNTemplate(templateBody)
	if err != nil {
		return CFNChangeSet{}, err
	}
	for logicalID, res := range tpl.Resources {
		if !supportedCFNType(res.Type) {
			return CFNChangeSet{}, fmt.Errorf("%w: unsupported resource type %q for %s", ErrCFNBadTemplate, res.Type, logicalID)
		}
		if err := validateCFNProperties(logicalID, res.Type, res.Properties); err != nil {
			return CFNChangeSet{}, err
		}
	}
	oldTpl, _ := parseCFNTemplate(st.TemplateBody)
	existing := map[string]CFNStackResource{}
	for _, r := range st.Resources {
		existing[r.LogicalID] = r
	}
	var changes []CFNChange
	for logicalID, res := range tpl.Resources {
		if cur, ok := existing[logicalID]; ok {
			oldRes := oldTpl.Resources[logicalID]
			oldProps, _ := json.Marshal(oldRes.Properties)
			newProps, _ := json.Marshal(res.Properties)
			if cur.ResourceType == res.Type && string(oldProps) == string(newProps) {
				continue
			}
			changes = append(changes, CFNChange{
				Action: "Modify", LogicalResourceID: logicalID, ResourceType: res.Type, PhysicalResourceID: cur.PhysicalID,
			})
		} else {
			changes = append(changes, CFNChange{
				Action: "Add", LogicalResourceID: logicalID, ResourceType: res.Type,
			})
		}
	}
	for logicalID, cur := range existing {
		if _, ok := tpl.Resources[logicalID]; !ok {
			changes = append(changes, CFNChange{
				Action: "Remove", LogicalResourceID: logicalID, ResourceType: cur.ResourceType, PhysicalResourceID: cur.PhysicalID,
			})
		}
	}
	raw, _ := json.Marshal(changes)
	now := time.Now().UTC().UnixMilli()
	id := uuid.NewString()
	changeSetID := fmt.Sprintf("arn:aws:cloudformation:%s:%s:changeSet/%s/%s", cfnRegionOrDefault(region), accountID, changeSetName, id)
	_, err = s.db.Exec(
		`INSERT INTO cfn_change_sets (change_set_id, account_id, stack_id, stack_name, change_set_name, status, status_reason, template_body, changes_json, created_at)
		 VALUES (?, ?, ?, ?, ?, 'CREATE_COMPLETE', '', ?, ?, ?)`,
		changeSetID, accountID, st.StackID, st.StackName, changeSetName, templateBody, string(raw), now,
	)
	if err != nil {
		return CFNChangeSet{}, fmt.Errorf("insert change set: %w", err)
	}
	return CFNChangeSet{
		ChangeSetID: changeSetID, ChangeSetName: changeSetName, StackName: st.StackName, StackID: st.StackID,
		Status: "CREATE_COMPLETE", TemplateBody: templateBody, Changes: changes, CreationTime: now,
	}, nil
}

func cfnRegionOrDefault(region string) string {
	if region == "" {
		return DefaultCFNRegion
	}
	return region
}

func (s *Store) DescribeCFNChangeSet(accountID, changeSetNameOrID, stackName string) (CFNChangeSet, error) {
	changeSetNameOrID = strings.TrimSpace(changeSetNameOrID)
	stackName = strings.TrimSpace(stackName)
	var cs CFNChangeSet
	var changesJSON string
	var err error
	if strings.HasPrefix(changeSetNameOrID, "arn:") {
		err = s.db.QueryRow(
			`SELECT change_set_id, stack_id, stack_name, change_set_name, status, status_reason, template_body, changes_json, created_at
			 FROM cfn_change_sets WHERE account_id = ? AND change_set_id = ?`,
			accountID, changeSetNameOrID,
		).Scan(&cs.ChangeSetID, &cs.StackID, &cs.StackName, &cs.ChangeSetName, &cs.Status, &cs.StatusReason, &cs.TemplateBody, &changesJSON, &cs.CreationTime)
	} else {
		err = s.db.QueryRow(
			`SELECT change_set_id, stack_id, stack_name, change_set_name, status, status_reason, template_body, changes_json, created_at
			 FROM cfn_change_sets WHERE account_id = ? AND change_set_name = ? AND (? = '' OR stack_name = ?)`,
			accountID, changeSetNameOrID, stackName, stackName,
		).Scan(&cs.ChangeSetID, &cs.StackID, &cs.StackName, &cs.ChangeSetName, &cs.Status, &cs.StatusReason, &cs.TemplateBody, &changesJSON, &cs.CreationTime)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return CFNChangeSet{}, ErrCFNChangeSetNotFound
	}
	if err != nil {
		return CFNChangeSet{}, fmt.Errorf("describe change set: %w", err)
	}
	_ = json.Unmarshal([]byte(changesJSON), &cs.Changes)
	return cs, nil
}

func (s *Store) ExecuteCFNChangeSet(accountID, region, changeSetNameOrID, stackName string) (CFNStack, error) {
	cs, err := s.DescribeCFNChangeSet(accountID, changeSetNameOrID, stackName)
	if err != nil {
		return CFNStack{}, err
	}
	if cs.Status != "CREATE_COMPLETE" && cs.Status != "AVAILABLE" {
		return CFNStack{}, fmt.Errorf("%w: change set status %s", ErrCFNChangeSetFailed, cs.Status)
	}
	for _, ch := range cs.Changes {
		if ch.Action == "Modify" {
			return CFNStack{}, fmt.Errorf("%w: Modify changes are not supported (fail-closed); logical id %s", ErrCFNBadTemplate, ch.LogicalResourceID)
		}
	}
	stacks, err := s.DescribeCFNStacks(accountID, cs.StackID)
	if err != nil {
		return CFNStack{}, err
	}
	st := stacks[0]
	tpl, err := parseCFNTemplate(cs.TemplateBody)
	if err != nil {
		return CFNStack{}, err
	}
	region = cfnRegionOrDefault(region)
	eval := newCFNEvalCtx(accountID, region, st.StackName)
	for _, res := range st.Resources {
		eval.setResource(res.LogicalID, res.PhysicalID, map[string]string{"Ref": res.PhysicalID, "Arn": res.PhysicalID})
	}

	// Removals first (reverse dependency-ish: by name descending).
	for _, ch := range cs.Changes {
		if ch.Action != "Remove" {
			continue
		}
		s.deleteCFNPhysical(accountID, CFNStackResource{
			LogicalID: ch.LogicalResourceID, ResourceType: ch.ResourceType, PhysicalID: ch.PhysicalResourceID,
		})
		_, _ = s.db.Exec(`DELETE FROM cfn_stack_resources WHERE stack_id = ? AND logical_id = ?`, st.StackID, ch.LogicalResourceID)
	}

	order, err := cfnDependencyOrder(tpl.Resources)
	if err != nil {
		return CFNStack{}, err
	}
	existingIDs := map[string]struct{}{}
	for _, r := range st.Resources {
		existingIDs[r.LogicalID] = struct{}{}
	}
	for _, logicalID := range order {
		if _, ok := existingIDs[logicalID]; ok {
			// kept / modify rejected earlier
			res := tpl.Resources[logicalID]
			props, resolveErr := eval.resolveProps(res.Properties)
			if resolveErr != nil {
				return CFNStack{}, resolveErr
			}
			_ = props
			continue
		}
		res := tpl.Resources[logicalID]
		props, resolveErr := eval.resolveProps(res.Properties)
		if resolveErr != nil {
			return CFNStack{}, resolveErr
		}
		physicalID, attrs, provErr := s.provisionCFNResource(accountID, region, st.StackName, st.StackID, logicalID, res.Type, props)
		if provErr != nil {
			return CFNStack{}, provErr
		}
		eval.setResource(logicalID, attrs["Ref"], attrs)
		_, err = s.db.Exec(
			`INSERT INTO cfn_stack_resources (stack_id, logical_id, resource_type, physical_id, status) VALUES (?, ?, ?, ?, 'CREATE_COMPLETE')`,
			st.StackID, logicalID, res.Type, physicalID,
		)
		if err != nil {
			return CFNStack{}, fmt.Errorf("insert stack resource: %w", err)
		}
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`UPDATE cfn_stacks SET template_body = ?, status = 'UPDATE_COMPLETE', updated_at = ? WHERE stack_id = ?`,
		cs.TemplateBody, now, st.StackID,
	)
	if err != nil {
		return CFNStack{}, fmt.Errorf("update stack after change set: %w", err)
	}
	_, _ = s.db.Exec(`UPDATE cfn_change_sets SET status = 'EXECUTE_COMPLETE' WHERE change_set_id = ?`, cs.ChangeSetID)
	out, err := s.DescribeCFNStacks(accountID, st.StackID)
	if err != nil {
		return CFNStack{}, err
	}
	return out[0], nil
}

// UpdateCFNStack applies an implicit change set (create + execute).
func (s *Store) UpdateCFNStack(accountID, region, stackName, templateBody string) (CFNStack, error) {
	name := "implicit-" + shortID()
	cs, err := s.CreateCFNChangeSet(accountID, region, stackName, name, templateBody)
	if err != nil {
		return CFNStack{}, err
	}
	return s.ExecuteCFNChangeSet(accountID, region, cs.ChangeSetID, stackName)
}