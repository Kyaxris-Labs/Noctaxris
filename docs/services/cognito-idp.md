# Cognito User Pools (cognito-idp)

**Status:** shipped (lab core)

User Pool and app client CRUD, AdminCreateUser / SignUp lite, InitiateAuth `USER_PASSWORD_AUTH` and refresh flows, `RevokeToken`, RS256 ID and access tokens, JWKS on the existing `:4566` listener.

## Implemented

| Area | Actions |
|------|---------|
| Pool | `CreateUserPool`, `DescribeUserPool`, `ListUserPools`, `DeleteUserPool` |
| Client | `CreateUserPoolClient`, `DescribeUserPoolClient`, `ListUserPoolClients`, `DeleteUserPoolClient` |
| Users | `AdminCreateUser`, `SignUp`, `ConfirmSignUp` |
| Auth | `InitiateAuth` (unsigned public IdP API; AWS CLI shape without `Authorization`): `USER_PASSWORD_AUTH`, `REFRESH_TOKEN_AUTH` / `REFRESH_TOKEN`; `AdminInitiateAuth` (same password and refresh flows; SigV4); `RevokeToken` (unsigned public IdP; `ClientId` + refresh `Token`) |
| JWKS | `GET /cognito-idp/{region}/{userPoolId}/.well-known/jwks.json` (no SigV4) |

### Issuer

Stable lab issuer (document for Gateway JWT and AppSync):

`http://127.0.0.1:4566/cognito-idp/<region>/<userPoolId>`

Tokens are RS256 with `kid`. ID token uses `aud` = client id and `token_use` = `id`. Access token uses `client_id` and `token_use` = `access`. Passwords are bcrypt hashed at rest. Signing keys are sealed under the store master key. Refresh tokens are stored as SHA-256 hashes only (raw tokens are never logged). Lab refresh always rotates: a successful `REFRESH_TOKEN_AUTH` / `REFRESH_TOKEN` invalidates the presented refresh token and returns a new one with new access/id tokens.

### Authz notes

Identity `EvaluateFull` on management `cognito-idp:*`. `InitiateAuth` and `RevokeToken` do not require SigV4 (public IdP). JWKS is public on loopback.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws cognito-idp create-user-pool --pool-name lab --endpoint-url "$EP"
aws cognito-idp create-user-pool-client --user-pool-id "$POOL" --client-name app --endpoint-url "$EP"
aws cognito-idp admin-create-user --user-pool-id "$POOL" --username alice --temporary-password 'Secret1!' --endpoint-url "$EP"
aws cognito-idp initiate-auth --client-id "$CLIENT" --auth-flow USER_PASSWORD_AUTH \
  --auth-parameters USERNAME=alice,PASSWORD='Secret1!' --endpoint-url "$EP"
# Capture RefreshToken from AuthenticationResult, then:
aws cognito-idp initiate-auth --client-id "$CLIENT" --auth-flow REFRESH_TOKEN_AUTH \
  --auth-parameters REFRESH_TOKEN="$REFRESH" --endpoint-url "$EP"
aws cognito-idp revoke-token --client-id "$CLIENT" --token "$REFRESH" --endpoint-url "$EP"
curl -s "http://127.0.0.1:4566/cognito-idp/us-east-1/$POOL/.well-known/jwks.json"
```

Document skip when Docker is unavailable (unit tests still cover issue/verify/refresh/revoke).

## Not yet / deferred

- Cognito Identity Pools
- Hosted UI / full SAML or OIDC federation into Cognito
- Full SRP_A flow
- MFA beyond optional future TOTP stub
- Cognito configure RoleArn fields (SMS / Lambda triggers) and PassRole for `cognito-idp.amazonaws.com`
- Advanced security mode / UI customization
