# Cognito User Pools (cognito-idp)

**Status:** shipped (lab core)

User Pool and app client CRUD, AdminCreateUser / SignUp lite, InitiateAuth `USER_PASSWORD_AUTH` / `USER_SRP_AUTH` and refresh flows, software-token (TOTP) MFA challenge, `RevokeToken`, RS256 ID and access tokens, JWKS on the existing `:4566` listener. Lab `RoleArn` plus `LambdaConfig` trigger ARNs on Create/UpdateUserPool with PassRole for `cognito-idp.amazonaws.com`, and sync Invoke of configured lifecycle triggers.

## Implemented

| Area | Actions |
|------|---------|
| Pool | `CreateUserPool`, `DescribeUserPool`, `UpdateUserPool`, `ListUserPools`, `DeleteUserPool` |
| Client | `CreateUserPoolClient`, `DescribeUserPoolClient`, `ListUserPoolClients`, `DeleteUserPoolClient` |
| Users | `AdminCreateUser`, `SignUp`, `ConfirmSignUp`, `ForgotPassword`, `ConfirmForgotPassword`, `ResendConfirmationCode`, `UpdateUserAttributes`, `GetUserAttributeVerificationCode`, `VerifyUserAttribute` |
| Auth | `InitiateAuth` (unsigned public IdP API; AWS CLI shape without `Authorization`): `USER_PASSWORD_AUTH`, `USER_SRP_AUTH` (`SRP_A` → `PASSWORD_VERIFIER`), `CUSTOM_AUTH` (Define/Create/Verify; optional SRP nesting `SRP_A` → `PASSWORD_VERIFIER` → `CUSTOM_CHALLENGE`), `REFRESH_TOKEN_AUTH` / `REFRESH_TOKEN`; `AdminInitiateAuth` (password and refresh flows; SigV4); `RevokeToken` (unsigned public IdP; `ClientId` + refresh `Token`) |
| MFA (TOTP) | `AssociateSoftwareToken`, `VerifySoftwareToken`, `RespondToAuthChallenge` (`SOFTWARE_TOKEN_MFA`, `PASSWORD_VERIFIER`) — unsigned public IdP. After Verify, password or SRP auth returns `ChallengeName=SOFTWARE_TOKEN_MFA` + `Session` (no tokens) until a valid TOTP is submitted |
| Triggers | `LambdaConfig` lab subset store ARNs (`PreSignUp`, `PostConfirmation`, `PreAuthentication`, `PostAuthentication`, `PreTokenGeneration`, `CustomMessage`, `UserMigration`, `DefineAuthChallenge`, `CreateAuthChallenge`, `VerifyAuthChallengeResponse`) and lab top-level `RoleArn` on Create/Update/Describe; PassRole for `cognito-idp.amazonaws.com` when `RoleArn` is set (`aws:SourceArn` = pool ARN). Sync Invoke on lifecycle events below |
| JWKS | `GET /cognito-idp/{region}/{userPoolId}/.well-known/jwks.json` (no SigV4) |

### Issuer

Stable lab issuer (document for Gateway JWT and AppSync):

`http://127.0.0.1:4566/cognito-idp/<region>/<userPoolId>`

Tokens are RS256 with `kid`. ID token uses `aud` = client id and `token_use` = `id`. Access token uses `client_id` and `token_use` = `access`. Passwords are bcrypt hashed at rest; SRP salt and verifier are stored for `USER_SRP_AUTH`. Signing keys and TOTP secrets are sealed under the store master key. Refresh tokens are stored as SHA-256 hashes only (raw tokens are never logged). Lab refresh always rotates: a successful `REFRESH_TOKEN_AUTH` / `REFRESH_TOKEN` invalidates the presented refresh token and returns a new one with new access/id tokens.

### SRP lab rules

- `InitiateAuth` `USER_SRP_AUTH` requires `USERNAME` and hex `SRP_A` (RFC 5054 3072-bit group; Cognito password hashing / HKDF).
- Challenge `PASSWORD_VERIFIER` returns `SALT`, `SRP_B`, `SECRET_BLOCK`, `USERNAME`, `USER_ID_FOR_SRP` plus `Session`.
- `RespondToAuthChallenge` `PASSWORD_VERIFIER` requires `Session` and claim fields (`PASSWORD_CLAIM_SIGNATURE`, `PASSWORD_CLAIM_SECRET_BLOCK`, `TIMESTAMP`, `USERNAME`). Wrong password signature yields `NotAuthorizedException`.
- When TOTP MFA is enabled, a successful password verifier returns `SOFTWARE_TOKEN_MFA` instead of tokens.

### Trigger RoleArn and Invoke lab rules

- AWS Cognito invokes triggers via Lambda resource policies; the lab additionally accepts a top-level pool `RoleArn` for configure-time PassRole parity with other services.
- Setting `RoleArn` on Create/UpdateUserPool requires `iam:PassRole` and a trust that Allows `sts:AssumeRole` for `cognito-idp.amazonaws.com` (optional `aws:SourceArn` / `aws:SourceAccount` on trust).
- `UpdateUserPool` replace semantics: omit `LambdaConfig` / `RoleArn` to clear those lab fields.
- When a trigger ARN is configured, the lab Invokes it synchronously (RequestResponse) on the matching lifecycle event:
  - `PreSignUp` → `SignUp` (before the user row is created)
  - `CustomMessage` → `SignUp` after the user row is created (`CustomMessage_SignUp`), `AdminCreateUser` after the user row is created (`CustomMessage_AdminCreateUser`), `ForgotPassword` (`CustomMessage_ForgotPassword`), `ResendConfirmationCode` (`CustomMessage_ResendCode`), `UpdateUserAttributes` when `email` or `phone_number` is present (`CustomMessage_UpdateUserAttribute`), and `GetUserAttributeVerificationCode` (`CustomMessage_VerifyUserAttribute`); lab stub `codeParameter` `{####}`; response `smsMessage` / `emailMessage` / `emailSubject` are validated to include the codeParameter, rendered with the issued confirmation code and `{username}`, and stored for tests — no SES send
  - `PostConfirmation` → `ConfirmSignUp` (after the user is marked `CONFIRMED`)
  - `PreAuthentication` → password / SRP auth start
  - `UserMigration` → `USER_PASSWORD_AUTH` when the username is missing (`UserMigration_Authentication`); if the Lambda response includes non-empty `response.userAttributes`, the lab creates a `CONFIRMED` user with the auth password and continues; otherwise `UserNotFoundException`. Fail closed on Invoke error
  - `PreTokenGeneration` → before access/id tokens are minted (including refresh; trigger source `TokenGeneration_RefreshTokens` on refresh)
  - `PostAuthentication` → after a successful password / SRP / MFA auth that returns tokens (not on refresh)
  - Custom auth (`CUSTOM_AUTH`): sync Invoke of `DefineAuthChallenge` → optional `CreateAuthChallenge` → `CUSTOM_CHALLENGE` response; `RespondToAuthChallenge` Invokes `VerifyAuthChallengeResponse` then Define again until `issueTokens` or `failAuthentication`. When `InitiateAuth` includes `SRP_A` (optional `CHALLENGE_NAME=SRP_A`), Define receives session `[{challengeName:SRP_A,challengeResult:true}]`; Define may return `PASSWORD_VERIFIER` and the lab reuses the same SRP challenge params as `USER_SRP_AUTH`; after a successful `RespondToAuthChallenge` `PASSWORD_VERIFIER`, Define runs again (may return `CUSTOM_CHALLENGE`)
- Fail closed: missing function, Invoke error, or unparseable PreToken/UserMigration/custom-auth/CustomMessage payload returns `UnexpectedLambdaException` / `InvalidLambdaResponseException` (or `UserNotFoundException` when migration returns no attributes) and the Cognito API fails.
- PreTokenGeneration V1 claim overrides: apply `response.claimsOverrideDetails.claimsToAddOrOverride` and `claimsToSuppress` to the **ID** token only. Reserved claims (`iss`, `aud`, `client_id`, `exp`, `iat`, `auth_time`, `token_use`, `sub`) are ignored for add/suppress. V2/V3 `claimsAndScopeOverrideDetails`, access-token custom claims, and `groupOverrideDetails` are not applied.
- CustomMessage lab helper: `GetLastCognitoCustomMessage` (store) returns the last rendered SMS/email body for a user (tests only; no public API).
- Confirmation codes (secure by default): `SignUp` / `ResendConfirmationCode` / `ForgotPassword` / attribute verify (`UpdateUserAttributes` for `email`/`phone_number`, `GetUserAttributeVerificationCode`) issue a high-entropy code (8+ hex chars), store it with a one-hour TTL, and require a constant-time match on `ConfirmSignUp` / `ConfirmForgotPassword` / `VerifyUserAttribute`. Codes are single-use (cleared on success). Wrong, missing, expired, or reused codes return `CodeMismatchException`.
- `NOCTAXRIS_COGNITO_INSECURE_CODES=1` restores the prior lab stubs: fixed code `123456` for ForgotPassword and attribute verify, and any non-empty `ConfirmSignUp` code. See [configuration.md](../configuration.md).
- `UpdateUserAttributes` / `GetUserAttributeVerificationCode` / `VerifyUserAttribute` are AccessToken public IdP APIs. Lab attributes live in SQLite; `email` / `phone_number` start unverified until `VerifyUserAttribute` succeeds with the issued code.

### MFA lab rules

- TOTP is RFC 6238 (SHA1, 30s period, 6 digits) with ±1 step skew.
- Enroll with access token from a password auth that has not yet enabled MFA: `AssociateSoftwareToken` → `VerifySoftwareToken` (`Session` + `UserCode`).
- Wrong TOTP on verify or challenge response returns `CodeMismatchException`.
- Raw TOTP secrets, session strings, and refresh tokens are not logged.

### Authz notes

Identity `EvaluateFull` on management `cognito-idp:*`. `InitiateAuth`, `ConfirmForgotPassword`, `UpdateUserAttributes`, `GetUserAttributeVerificationCode`, `VerifyUserAttribute`, `RevokeToken`, `AssociateSoftwareToken`, `VerifySoftwareToken`, and `RespondToAuthChallenge` do not require SigV4 (public IdP). JWKS is public on loopback. PassRole applies only when configuring `RoleArn`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws cognito-idp create-user-pool --pool-name lab --endpoint-url "$EP"
aws cognito-idp create-user-pool-client --user-pool-id "$POOL" --client-name app --endpoint-url "$EP"
aws cognito-idp admin-create-user --user-pool-id "$POOL" --username alice --temporary-password 'Secret1!' --endpoint-url "$EP"
AUTH=$(aws cognito-idp initiate-auth --client-id "$CLIENT" --auth-flow USER_PASSWORD_AUTH \
  --auth-parameters USERNAME=alice,PASSWORD='Secret1!' --endpoint-url "$EP" --output json)
ACCESS=$(echo "$AUTH" | python3 -c 'import sys,json; print(json.load(sys.stdin)["AuthenticationResult"]["AccessToken"])')
ASSOC=$(aws cognito-idp associate-software-token --access-token "$ACCESS" --endpoint-url "$EP" --output json)
SECRET=$(echo "$ASSOC" | python3 -c 'import sys,json; print(json.load(sys.stdin)["SecretCode"])')
SESSION=$(echo "$ASSOC" | python3 -c 'import sys,json; print(json.load(sys.stdin)["Session"])')
# Produce a 6-digit TOTP from $SECRET (authenticator app or RFC 6238 helper), then:
aws cognito-idp verify-software-token --session "$SESSION" --user-code "$TOTP" --endpoint-url "$EP"
CHAL=$(aws cognito-idp initiate-auth --client-id "$CLIENT" --auth-flow USER_PASSWORD_AUTH \
  --auth-parameters USERNAME=alice,PASSWORD='Secret1!' --endpoint-url "$EP" --output json)
# ChallengeName SOFTWARE_TOKEN_MFA; respond with current TOTP:
aws cognito-idp respond-to-auth-challenge --client-id "$CLIENT" --challenge-name SOFTWARE_TOKEN_MFA \
  --session "$(echo "$CHAL" | python3 -c 'import sys,json; print(json.load(sys.stdin)["Session"])')" \
  --challenge-responses USERNAME=alice,SOFTWARE_TOKEN_MFA_CODE="$TOTP" --endpoint-url "$EP"
# Capture RefreshToken from AuthenticationResult, then:
aws cognito-idp initiate-auth --client-id "$CLIENT" --auth-flow REFRESH_TOKEN_AUTH \
  --auth-parameters REFRESH_TOKEN="$REFRESH" --endpoint-url "$EP"
aws cognito-idp revoke-token --client-id "$CLIENT" --token "$REFRESH" --endpoint-url "$EP"
curl -s "http://127.0.0.1:4566/cognito-idp/us-east-1/$POOL/.well-known/jwks.json"
```

`USER_SRP_AUTH` needs an SRP client (SDK helper or amazon-cognito-identity-js shape): `InitiateAuth` with `SRP_A`, then `RespondToAuthChallenge` `PASSWORD_VERIFIER`. Store unit tests cover the full round-trip; AWS CLI alone does not compute SRP.

Document skip when Docker is unavailable (unit tests still cover issue/verify/refresh/revoke/MFA/SRP/PassRole).

## Not yet / deferred

- PreTokenGeneration V2/V3 `claimsAndScopeOverrideDetails` (access-token claims/scopes) and `groupOverrideDetails`
- UserMigration on `USER_SRP_AUTH` (AWS requires password auth so the migrate Lambda can verify credentials; SRP obscures the password)
- `CustomMessage_Authentication` (SMS MFA out of lab scope)
- `SECRET_HASH` for app clients with a client secret
- `NEW_PASSWORD_REQUIRED` and device SRP challenges

## Out of lab scope

- Cognito Identity Pools (out of lab scope; User Pools + JWKS cover Gateway/AppSync JWT labs)
- Hosted UI / full SAML or OIDC federation into Cognito (out of lab scope; no second host port for Hosted UI)
- SMS / email MFA, `MFA_SETUP` challenge orchestration beyond access-token enroll, Adaptive authentication (out of lab scope; TOTP covers MFA labs)
- Advanced security mode / UI customization (out of lab scope)
