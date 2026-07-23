package store

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

const actionS3PutObject = "s3:PutObject"

// RoleSessionAllows mints a temporary session for roleARN and evaluates whether
// the role identity policies Allow action on targetARN (Gateway CredentialsArn, etc.).
func (s *Store) RoleSessionAllows(accountID, roleARN, action, targetARN, sessionName, region string) bool {
	return s.deliveryRoleSessionAllows(accountID, roleARN, action, targetARN, sessionName, region, "")
}

// deliveryAuthorizedRoleAndResource authorizes RoleArn delivery.
// Same-account (or targets without a resource-policy surface): RoleArn session Allow alone
// remains the lab path. Foreign SQS/Lambda/SNS/S3 targets require session Allow AND
// destination resource-policy Allow (AWS XA service-target shape), with SourceArn keys.
func (s *Store) deliveryAuthorizedRoleAndResource(
	delivererAccount, roleARN, action, targetARN, servicePrincipal, sourceARN, sessionName, region string,
) bool {
	roleARN = strings.TrimSpace(roleARN)
	if roleARN == "" {
		return s.deliveryTargetResourcePolicyAllows(delivererAccount, targetARN, action, servicePrincipal, sourceARN)
	}
	if !s.deliveryRoleSessionAllows(delivererAccount, roleARN, action, targetARN, sessionName, region, sourceARN) {
		return false
	}
	owner := resourceOwnerAccountFromARN(targetARN)
	if owner == "" || owner == delivererAccount || !deliveryTargetSupportsResourcePolicy(targetARN) {
		return true
	}
	return s.deliveryTargetResourcePolicyAllows(delivererAccount, targetARN, action, servicePrincipal, sourceARN)
}

func deliveryTargetSupportsResourcePolicy(targetARN string) bool {
	switch {
	case strings.HasPrefix(targetARN, "arn:aws:sqs:"),
		strings.HasPrefix(targetARN, "arn:aws:lambda:"),
		strings.HasPrefix(targetARN, "arn:aws:sns:"),
		strings.HasPrefix(targetARN, "arn:aws:s3:::"):
		return true
	default:
		return false
	}
}

// deliveryRoleSessionAllows mints a temporary session for roleARN and evaluates
// whether the role identity policies Allow action on targetARN.
// sourceARN populates aws:SourceArn / aws:SourceAccount ConditionKeys when set.
func (s *Store) deliveryRoleSessionAllows(accountID, roleARN, action, targetARN, sessionName, region, sourceARN string) bool {
	roleAccountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok || roleAccountID != accountID {
		return false
	}
	if _, _, err := s.GetRole(accountID, roleName); err != nil {
		return false
	}
	secret, err := randomHexSecret(16)
	if err != nil {
		log.Printf("delivery role session mint secret failed role=%s err=%v", roleARN, err)
		return false
	}
	sessionToken, err := randomHexSecret(16)
	if err != nil {
		log.Printf("delivery role session mint token failed role=%s err=%v", roleARN, err)
		return false
	}
	accessKeyID, err := s.MintTempCredentialsOpts(MintTempOpts{
		AccountID:    accountID,
		RoleARN:      roleARN,
		SessionName:  sessionName,
		Secret:       secret,
		SessionToken: sessionToken,
		Expires:      time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		log.Printf("delivery role session mint failed role=%s err=%v", roleARN, err)
		return false
	}

	docs, err := s.identityPolicyDocsForRoleARN(roleARN)
	if err != nil {
		log.Printf("delivery role policy load failed role=%s err=%v", roleARN, err)
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
	if region == "" {
		region = DefaultEventsRegion
	}
	principal := identity.RoleSessionPrincipal(accountID, roleName, sessionName, accessKeyID)
	keys := s.deliveryRoleSessionConditionKeys(accountID, roleName, targetARN, region)
	for k, v := range authz.DeliverySourceConditionKeys(sourceARN, "") {
		keys[k] = v
	}
	ctx := authz.RequestContext{
		Principal:     principal,
		Action:        action,
		Resource:      targetARN,
		Region:        region,
		ConditionKeys: keys,
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

// DeliveryTargetResourcePolicyAllows reports whether the target resource policy
// Allows action for the given service principal (or account root).
// sourceARN populates aws:SourceArn / aws:SourceAccount for Condition evaluation.
func (s *Store) DeliveryTargetResourcePolicyAllows(accountID, targetARN, action, servicePrincipal, sourceARN string) bool {
	return s.deliveryTargetResourcePolicyAllows(accountID, targetARN, action, servicePrincipal, sourceARN)
}

// deliveryTargetResourcePolicyAllows reports whether the target resource policy
// Allows action for the given service principal (or account root).
// sourceARN populates aws:SourceArn / aws:SourceAccount for Condition evaluation.
// For SQS/Lambda/SNS ARNs, policy is loaded under the resource owner account.
func (s *Store) deliveryTargetResourcePolicyAllows(accountID, targetARN, action, servicePrincipal, sourceARN string) bool {
	policyAccount := accountID
	if owner := resourceOwnerAccountFromARN(targetARN); owner != "" {
		policyAccount = owner
	}
	policyDoc, err := s.deliveryTargetResourcePolicyDoc(policyAccount, targetARN)
	if err != nil {
		return false
	}
	keys := authz.DeliverySourceConditionKeys(sourceARN, "")
	return authz.EventTargetResourcePolicyAllows(
		policyDoc,
		action,
		targetARN,
		servicePrincipal,
		policyAccount,
		keys,
	)
}

// resourceOwnerAccountFromARN returns the account id segment for common ARNs.
func resourceOwnerAccountFromARN(arn string) string {
	arn = strings.TrimSpace(arn)
	parts := strings.Split(arn, ":")
	if len(parts) < 5 {
		return ""
	}
	acct := strings.TrimSpace(parts[4])
	if len(acct) != 12 {
		return ""
	}
	for _, r := range acct {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return acct
}

func (s *Store) deliveryTargetResourcePolicyDoc(accountID, targetARN string) (string, error) {
	switch {
	case strings.HasPrefix(targetARN, "arn:aws:sqs:"):
		queueName, err := queueNameFromARN(targetARN)
		if err != nil {
			return "", err
		}
		attrs, err := s.GetQueueAttributes(accountID, queueName)
		if err != nil {
			return "", err
		}
		return attrs["Policy"], nil
	case strings.HasPrefix(targetARN, "arn:aws:lambda:"):
		functionName, _ := ParseFunctionQualifier(targetARN)
		if functionName == "" {
			return "", fmt.Errorf("delivery target: empty function name")
		}
		doc, err := s.GetFunctionPolicy(accountID, functionName)
		if err != nil {
			if errors.Is(err, ErrNoSuchResourcePolicy) {
				return "", nil
			}
			return "", err
		}
		return doc, nil
	case strings.HasPrefix(targetARN, "arn:aws:sns:"):
		topic, err := s.GetTopicByARN(targetARN)
		if err != nil {
			return "", err
		}
		return topic.Policy, nil
	case strings.HasPrefix(targetARN, "arn:aws:s3:::"):
		bucket := strings.TrimPrefix(targetARN, "arn:aws:s3:::")
		bucket = strings.SplitN(bucket, "/", 2)[0]
		if bucket == "" {
			return "", fmt.Errorf("delivery target: empty bucket")
		}
		doc, err := s.GetBucketPolicy(accountID, bucket)
		if err != nil {
			if errors.Is(err, ErrNoSuchBucketPolicy) {
				return "", nil
			}
			return "", err
		}
		return doc, nil
	default:
		return "", fmt.Errorf("delivery target: unsupported arn %s", targetARN)
	}
}

func deliveryActionForARN(targetARN string) (string, bool) {
	switch {
	case strings.HasPrefix(targetARN, "arn:aws:sqs:"):
		return actionSQSSendMessage, true
	case strings.HasPrefix(targetARN, "arn:aws:lambda:"):
		return actionLambdaInvokeFunction, true
	case strings.HasPrefix(targetARN, "arn:aws:sns:"):
		return actionSNSPublish, true
	case strings.HasPrefix(targetARN, "arn:aws:s3:::"):
		return actionS3PutObject, true
	default:
		return "", false
	}
}
