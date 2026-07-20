# Cognito User Pools (cognito-idp)

**Status:** shipped (lab core)

User Pool and app client CRUD, AdminCreateUser / SignUp lite, InitiateAuth `USER_PASSWORD_AUTH`, RS256 ID and access tokens, JWKS on the existing `:4566` listener.

## Implemented

| Area | Actions |
|------|---------|
| Pool | `CreateUserPool`, `DescribeUserPool`, `ListUserPools`, `DeleteUserPool` |
| Client | `CreateUserPoolClient`, `DescribeUserPoolClient`, `ListUserPoolClients`, `DeleteUserPoolClient` |
| Users | `AdminCreateUser`, `SignUp`, `ConfirmSignUp` |
| Auth | `InitiateAuth`, `AdminInitiateAuth` (`USER_PASSWORD_AUTH` / `ADMIN_USER_PASSWORD_AUTH`) |
| JWKS | `GET /cognito-idp/{region}/{userPoolId}/.well-known/jwks.json` (no SigV4) |

### Issuer

Stable lab issuer (document for Gateway JWT and AppSync):

`http://127.0.0.1:4566/cognito-idp/<region>/<userPoolId>`

Tokens are RS256 with `kid`. ID token uses `aud` = client id and `token_use` = `id`. Access token uses `client_id` and `token_use` = `access`. Passwords are bcrypt hashed at rest. Signing keys are sealed under the store master key.

### Authz notes

Identity `EvaluateFull` on management `cognito-idp:*`. JWKS is public on loopback.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws cognito-idp create-user-pool --pool-name lab --endpoint-url "$EP"
aws cognito-idp create-user-pool-client --user-pool-id "$POOL" --client-name app --endpoint-url "$EP"
aws cognito-idp admin-create-user --user-pool-id "$POOL" --username alice --temporary-password 'Secret1!' --endpoint-url "$EP"
aws cognito-idp initiate-auth --client-id "$CLIENT" --auth-flow USER_PASSWORD_AUTH \
  --auth-parameters USERNAME=alice,PASSWORD='Secret1!' --endpoint-url "$EP"
curl -s "http://127.0.0.1:4566/cognito-idp/us-east-1/$POOL/.well-known/jwks.json"
```

Document skip when Docker is unavailable (unit tests still cover issue/verify).

## Not yet / deferred

- Cognito Identity Pools
- Hosted UI / full SAML or OIDC federation into Cognito
- Full SRP_A flow
- MFA beyond optional future TOTP stub
- Refresh token rotation depth and revoke APIs
- Cognito configure RoleArn fields (SMS / Lambda triggers) and PassRole for `cognito-idp.amazonaws.com`
- Advanced security mode / UI customization
