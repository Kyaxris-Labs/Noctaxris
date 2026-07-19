# STS

**Status:** shipped

All 11 STS actions are routed. Federation is fail-closed without configured IdP. Lab MFA applies to `GetSessionToken`.

## Implemented

| Action | Behavior |
|--------|----------|
| `GetCallerIdentity` | After SigV4 only (no IAM permission check) |
| `AssumeRole` | Temp credentials, trust evaluation, cross-account dual eval when needed |
| `GetSessionToken` | Lab MFA token (not RFC 6238 TOTP). Does not require `sts:GetSessionToken` after SigV4 |
| `GetFederationToken` | Session with optional session policy intersection |
| `AssumeRoleWithSAML` | Crypto against configured SAML IdP (no SigV4) |
| `AssumeRoleWithWebIdentity` | Crypto against configured OIDC issuer (no SigV4) |
| `AssumeRoot` | Member account only when org relationship exists |
| `DecodeAuthorizationMessage` | Lab decode path |
| `GetAccessKeyInfo` | Lab access-key info |
| `GetDelegatedAccessToken` | Fail-closed stub |
| `GetWebIdentityToken` | Fail-closed stub |

Session policies intersect via `EvaluateWithSession` / `EvaluateFull`. Temporary credentials require `X-Amz-Security-Token` on later SigV4 calls.

### Authz notes

Most STS control-plane actions use `EvaluateFull` (identity, boundary, session, SCP, RCP). `GetCallerIdentity` skips IAM Evaluate after successful SigV4. SAML and web-identity federation skip SigV4 and require configured IdP metadata or OIDC issuer. Without IdP, those paths deny.

Cross-account `AssumeRole` uses `EvaluateCrossAccount` (caller identity plus role trust).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Create `LabRole` first (see [iam.md](iam.md)).

```bash
aws sts get-caller-identity --endpoint-url "$EP"

aws sts assume-role \
  --role-arn arn:aws:iam::000000000001:role/LabRole \
  --role-session-name lab \
  --endpoint-url "$EP"

aws sts get-session-token --endpoint-url "$EP"

# Fail-closed without IdP (expect AccessDenied / IdP not configured):
aws sts assume-role-with-web-identity \
  --role-arn arn:aws:iam::000000000001:role/LabRole \
  --role-session-name wid \
  --web-identity-token eyJhbGciOiJub25lIn0.e30. \
  --endpoint-url "$EP"
```

MFA and `GetSessionToken` (lab token, not RFC 6238 TOTP). After IAM `CreateVirtualMFADevice`, `Base32StringSeed` is hex-encoded seed bytes. Token is the first 6 hex chars of `sha256(seed + ":" + unixMinute)`.

```bash
aws iam create-user --user-name mfa-user --endpoint-url "$EP"
MFA_KEYS=$(aws iam create-access-key --user-name mfa-user --endpoint-url "$EP" --output json)
MFA_AKID=$(echo "$MFA_KEYS" | python3 -c 'import sys,json; print(json.load(sys.stdin)["AccessKey"]["AccessKeyId"])')
MFA_SECRET=$(echo "$MFA_KEYS" | python3 -c 'import sys,json; print(json.load(sys.stdin)["AccessKey"]["SecretAccessKey"])')

MFA_JSON=$(aws iam create-virtual-mfa-device --endpoint-url "$EP" --output json)
SERIAL=$(echo "$MFA_JSON" | python3 -c 'import sys,json; print(json.load(sys.stdin)["VirtualMFADevice"]["SerialNumber"])')
SEED_HEX=$(echo "$MFA_JSON" | python3 -c 'import sys,json; print(json.load(sys.stdin)["VirtualMFADevice"]["Base32StringSeed"])')
aws iam enable-mfa-device --user-name mfa-user --serial-number "$SERIAL" --endpoint-url "$EP"

TOKEN=$(python3 -c "import hashlib,time; seed=bytes.fromhex('$SEED_HEX'); m=int(time.time())//60; print(hashlib.sha256(seed+b':'+str(m).encode()).hexdigest()[:6])")

AWS_ACCESS_KEY_ID="$MFA_AKID" AWS_SECRET_ACCESS_KEY="$MFA_SECRET" \
  aws sts get-session-token \
  --serial-number "$SERIAL" --token-code "$TOKEN" \
  --endpoint-url "$EP"
```

Expect a session with temporary credentials. `GetSessionToken` does not require an IAM `sts:GetSessionToken` permission after SigV4.

## Not yet / deferred

- Full AssumeRoot parity with AWS Organizations / IAM Identity Center style flows
- Rich DecodeAuthorizationMessage payloads generated from every deny path
- Production-grade GetDelegatedAccessToken and GetWebIdentityToken parity
