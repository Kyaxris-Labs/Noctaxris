# STS

**Status:** shipped

All 11 STS actions are routed. Federation is fail-closed without configured IdP. Lab MFA applies to `GetSessionToken`.

## Implemented

| Action | Behavior |
|--------|----------|
| `GetCallerIdentity` | After SigV4 only (no IAM permission check) |
| `AssumeRole` | Temp credentials, trust evaluation, cross-account dual eval when needed |
| `GetSessionToken` | Lab MFA token (not RFC 6238 TOTP). Requires long-term IAM user or root credentials (not temporary/role). Does not require `sts:GetSessionToken` after SigV4 |
| `GetFederationToken` | Calling IAM user identity ∩ session policy (empty session → no identity permissions) |
| `AssumeRoleWithSAML` | Crypto against configured SAML IdP (no SigV4) |
| `AssumeRoleWithWebIdentity` | Crypto against configured OIDC issuer (no SigV4) |
| `AssumeRoot` | Management account root only, into an org member account |
| `DecodeAuthorizationMessage` | Lab decode path |
| `GetAccessKeyInfo` | Lab access-key info |
| `GetDelegatedAccessToken` | Fail-closed stub |
| `GetWebIdentityToken` | Fail-closed stub |

Session policies intersect via `EvaluateWithSession` / `EvaluateFull`. Temporary credentials require `X-Amz-Security-Token` on later SigV4 calls.

### Authz notes

Most STS control-plane actions use `EvaluateFull` (identity, boundary, session, SCP, RCP). `GetCallerIdentity` skips IAM Evaluate after successful SigV4. SAML and web-identity federation skip SigV4 and require configured IdP metadata or OIDC issuer. Without IdP, those paths deny. `AssumeRoleWithWebIdentity` JWT verify requires `exp` (missing or expired fails closed) and rejects not-yet-valid `nbf` when present.

`GetSessionToken` must be called with long-term credentials. Sessions it mints cannot call IAM unless MFA was used to mint them, and cannot call STS except `AssumeRole` and `GetCallerIdentity`.

Cross-account `AssumeRole` uses `EvaluateCrossAccount` (caller identity plus role trust). `ExternalId` / `SourceIdentity` request params populate `sts:ExternalId` and `sts:SourceIdentity` / `aws:SourceIdentity` for trust Conditions. Assumed-role sessions load identity policies from the IAM role ARN (not the STS session ARN), so role attachments apply to SigV4 calls that use temporary credentials.

Role chaining: callers with a session token (or Role/Federated principal) cannot request `DurationSeconds` greater than 3600. Role `MaxSessionDuration` (CreateRole, default 3600, max 43200) caps all AssumeRole mints.

`AssumeRoleWithSAML` / `AssumeRoleWithWebIdentity` match trust `Principal.Federated` against the IdP ARN (account-root `AWS` principals do not over-allow federation callers). SAML verify is a lab subset: SignedInfo RSA, Reference DigestValue over the Assertion with Signature removed, Conditions time window, and Audience matching metadata `entityID` (not exclusive C14N). Web identity trusts can Condition on `{issuer-host}:sub` / `:aud` and, for GitHub Actions issuers, `token.actions.githubusercontent.com:*` claim keys populated from the verified JWT. OIDC providers store the full `ClientIDList` (and thumbprints for honesty; JWKS verify does not check thumbprints).

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

## Out of lab scope

- Full AssumeRoot parity with AWS Organizations / IAM Identity Center style flows (out of lab scope; lab STS covers the 11-action core)
- Rich DecodeAuthorizationMessage payloads generated from every deny path (out of lab scope)
- Production-grade GetDelegatedAccessToken and GetWebIdentityToken parity (out of lab scope)
