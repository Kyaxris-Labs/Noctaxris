# KMS

**Status:** shipped

Lab-complete customer-managed keys: sealed CMK material, key policies with explicit allow, Encrypt/Decrypt/GenerateDataKey*, ReEncrypt, grants, aliases (including lab `alias/aws/s3`, `alias/aws/dynamodb`, `alias/aws/sqs`), deletion lifecycle with post-DeletionDate purge, and key-material rotation (not flags only).

## Implemented

| Area | Actions |
|------|---------|
| Keys | `CreateKey`, `DescribeKey`, `ListKeys`, `EnableKey`, `DisableKey` |
| Lifecycle | `ScheduleKeyDeletion`, `CancelKeyDeletion` (`PendingDeletion` state. Cancel sets `Disabled`, matching AWS. On-read sweeper hard-deletes keys after `DeletionDate`, including aliases and grants) |
| Rotation | `EnableKeyRotation`, `DisableKeyRotation`, `GetKeyRotationStatus` (enable rotates sealed material immediately. Prior generations remain for Decrypt. Lab auto-rotate after `rotation_period_days`, default 365) |
| Key policy | `GetKeyPolicy`, `PutKeyPolicy` |
| Cryptographic | `Encrypt`, `Decrypt`, `GenerateDataKey`, `GenerateDataKeyWithoutPlaintext`, `ReEncrypt` (optional `EncryptionContext` bound as GCM AAD; decrypt/re-encrypt must supply the same map) |
| Grants | `CreateGrant`, `ListGrants`, `RetireGrant`, `RevokeGrant` |
| Aliases | `CreateAlias`, `ListAliases`, `DeleteAlias`, `UpdateAlias` |
| Tags | `ListResourceTags`, `TagResource`, `UntagResource`; `CreateKey` accepts `Tags` |
| Lab convenience aliases | Per-account `alias/aws/s3`, `alias/aws/dynamodb`, `alias/aws/sqs` (lab CMK approximations, not AWS-owned keys) |

CreateKey seeds a default key policy that allows the account root (and the IAM user creator when applicable). CMK material is sealed at rest under the data volume. Lab convenience aliases are created on first use per account.

### Authz notes

KMS uses `EvaluateKMS`: for key-scoped operations, identity Allow alone is not enough. The key policy (or a matching grant) must explicitly allow the principal and action. Key policy statements must name a `Principal` on put (`PutKeyPolicy` rejects Principal-less documents) and at eval missing Principal matches none. Org SCP/RCP filters apply on the data-plane path. CreateKey is identity-evaluated (no key yet). Cross-service seals (Secrets Manager, SSM SecureString, DynamoDB SSE, S3 SSE-KMS) pass AWS-shaped EncryptionContext maps into EvaluateKMS and ciphertext AAD; see those service pages for the keys.

When `EncryptionContext` is present, request condition keys include `kms:EncryptionContext:<key>` for each pair and `kms:EncryptionContextKeys` (sorted, comma-joined). Ciphertext integrity binds the same map as AES-GCM AAD: a mismatched or omitted context fails decrypt with `InvalidCiphertextException`.

Cross-account Encrypt and similar crypto APIs use a full key ARN. Both the caller identity policy and the trusting account key policy must Allow.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
KEY_JSON=$(aws kms create-key --endpoint-url "$EP" --output json)
KEY_ID=$(echo "$KEY_JSON" | python3 -c 'import sys,json; print(json.load(sys.stdin)["KeyMetadata"]["KeyId"])')

aws kms encrypt --key-id "$KEY_ID" --plaintext "$(echo -n hello | base64)" --endpoint-url "$EP"
aws kms generate-data-key --key-id "$KEY_ID" --key-spec AES_256 --endpoint-url "$EP"
aws kms create-alias --alias-name alias/lab --target-key-id "$KEY_ID" --endpoint-url "$EP"
```

Deletion lifecycle and rotation flags:

```bash
KEY_ID=$(aws kms create-key --endpoint-url "$EP" --query KeyMetadata.KeyId --output text)
aws kms schedule-key-deletion --key-id "$KEY_ID" --pending-window-in-days 7 --endpoint-url "$EP"
aws kms cancel-key-deletion --key-id "$KEY_ID" --endpoint-url "$EP"
# Cancel leaves the key Disabled (AWS-shaped). Enable before crypto use.
aws kms enable-key --key-id "$KEY_ID" --endpoint-url "$EP"
aws kms enable-key-rotation --key-id "$KEY_ID" --endpoint-url "$EP"
aws kms get-key-rotation-status --key-id "$KEY_ID" --endpoint-url "$EP"
```

Two-account cross-account Encrypt (member account B owns the key, member account A user encrypts via dual eval):

```bash
KEY_ARN=$(aws kms create-key --endpoint-url "$EP" --profile account-b --query KeyMetadata.Arn --output text)
aws kms put-key-policy --key-id "$KEY_ARN" --policy-name default --endpoint-url "$EP" --profile account-b \
  --policy '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::ACCOUNT_A:user/crypto"},"Action":"kms:Encrypt","Resource":"*"}]}'
aws kms encrypt --key-id "$KEY_ARN" --plaintext "$(echo -n hello-xa | base64)" \
  --endpoint-url "$EP" --profile account-a
```

## Out of lab scope

- Full KMS SAR beyond the lab set (Sign/Verify, MAC, GetPublicKey, asymmetric and HMAC key specs, ImportKeyMaterial, custom key stores, multi-Region replica keys, full pagination parity, `RotateKeyOnDemand` API shape) (out of lab scope; symmetric CMK lab core is shipped, including tags)
- Grant `Constraints` / distinct `GrantToken` / `GrantTokens` on crypto APIs (out of lab scope)
- Cross-account grant flows beyond key policy dual eval (out of lab scope)
- True AWS-owned managed key types beyond the lab convenience aliases above (out of lab scope)
- AWS-faithful annual rotation calendar and multi-Region material replication (out of lab scope)
