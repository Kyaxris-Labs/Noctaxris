# Changelog

## v2 (in progress)

Clear remaining deferred depth for the lab core (except microVMs), then add lab-complete SSM Parameter Store, Secrets Manager, SNS, EventBridge, ECR, and ECS.

### Shipped so far

- Condition-key catalogs for lab-core services from servicereference JSON plus global seed
- Authz Condition rules per ADR-0005 §7 (catalog-unknown deny, unpopulated known fail-closed on positive operators, AWS-faithful Null / StringNotEquals / IfExists)
- IAM groups (membership, managed and inline group policies), permissions boundaries for users and roles, instance profiles, OIDC and SAML IdP CRUD
- `EvaluateFull` identity plus boundary plus SCP and RCP intersection wired into shared `authorize`
- Organizations ListAccounts, OUs, EnablePolicyType, SCP and RCP CreatePolicy / AttachPolicy / DetachPolicy / DescribePolicy
- IAM virtual MFA devices, lab MFA token validation, `GetSessionToken` without IAM permission gate, MFA sessions set `aws:MultiFactorAuthPresent`

Tracking list: [docs/deferred.md](docs/deferred.md).

## Lab core

First shippable service set for Docker-first local AWS-shaped labs.

### Included

- Secure Compose defaults: loopback publish, no host `docker.sock`, sealed secrets
- SigV4 (header and query), IAM lab APIs, all 11 STS actions, Organizations CreateAccount
- Lab-complete KMS (keys, policies, crypto, grants, aliases)
- Lab-complete S3 (path-style objects, bucket policy, SSE-S3/SSE-KMS, presigned GET/PUT)
- Lab-complete DynamoDB and SQS (items/messages, resource/queue policies, encryption options)
- Lab-complete Lambda (zip `python3.12`, PassRole + `lambda.amazonaws.com` trust, nested DinD sync Invoke)

### Explicitly later

See [docs/deferred.md](docs/deferred.md) for SAR depth, FIFO, image-based Lambda, condition-key codegen, new lab cores, and post-v2 microVMs.
