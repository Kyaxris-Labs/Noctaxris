package store

import (
	"fmt"
	"strings"
)

// deliveryRoleSessionConditionKeys builds ConditionKeys for RoleArn delivery
// identity evaluation (PrincipalAccount/Arn/Type/Region + ResourceTag when known).
// Callers may overlay aws:SourceArn / aws:SourceAccount from DeliverySourceConditionKeys.
func (s *Store) deliveryRoleSessionConditionKeys(accountID, roleName, targetARN, region string) map[string]string {
	if region == "" {
		region = DefaultEventsRegion
	}
	keys := map[string]string{
		"aws:PrincipalAccount": accountID,
		"aws:RequestedRegion":  region,
		"aws:PrincipalType":    "AssumedRole",
	}
	if roleName != "" && accountID != "" {
		keys["aws:PrincipalArn"] = fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)
	}
	s.mergeDeliveryResourceTagKeys(keys, accountID, targetARN)
	return keys
}

func (s *Store) mergeDeliveryResourceTagKeys(keys map[string]string, accountID, resource string) {
	resource = strings.TrimSpace(resource)
	if keys == nil || resource == "" || resource == "*" {
		return
	}
	tagAccount := resourceOwnerAccountFromARN(resource)
	if tagAccount == "" {
		tagAccount = accountID
	}
	if tagAccount == "" {
		return
	}
	tags, err := s.ListResourceTags(tagAccount, resource)
	if err != nil || len(tags) == 0 {
		return
	}
	svcPrefix := deliveryServiceResourceTagPrefix(resource)
	for _, tag := range tags {
		k := strings.TrimSpace(tag.Key)
		if k == "" {
			continue
		}
		keys["aws:ResourceTag/"+k] = tag.Value
		if svcPrefix != "" {
			keys[svcPrefix+k] = tag.Value
		}
	}
}

func deliveryServiceResourceTagPrefix(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 3 {
		return ""
	}
	switch strings.ToLower(parts[2]) {
	case "ecr":
		return "ecr:ResourceTag/"
	case "ssm":
		return "ssm:resourceTag/"
	case "secretsmanager":
		return "secretsmanager:ResourceTag/"
	case "iam":
		return "iam:ResourceTag/"
	case "ecs":
		return "ecs:ResourceTag/"
	default:
		return ""
	}
}
