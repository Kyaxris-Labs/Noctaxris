# SES

**Status:** shipped (lab core)

Local email catcher. Verify identities, send mail into the store, list identities, and read send statistics. No outbound SMTP.

## Implemented

| Area | Actions |
|------|---------|
| Identities (v1 Query) | `VerifyEmailIdentity` (lab auto-verify), `ListIdentities`, `SetIdentityNotificationTopic` (Bounce SNS topic only) |
| Identities (v2 REST) | `POST/GET/DELETE /v2/email/identities`, `GET /v2/email/identities/{emailIdentity}` (`CreateEmailIdentity`, `ListEmailIdentities`, `GetEmailIdentity`, `DeleteEmailIdentity`). Email identities auto-verify; domain identities stay pending with lab DKIM token stubs. |
| Send | `SendEmail`, `SendRawEmail` (v1 Query); `POST /v2/email/outbound-emails` with `Content.Simple` or `Content.Raw` (same `ses_messages` catcher). Destinations containing `bounce@` Publish a Bounce notification when Bounce SNS is configured |
| Account (v2) | `GET /v2/email/account` (`GetAccount` stub: send quota, suppression reasons, sending enabled) |
| Stats | `GetSendStatistics` stub (`DeliveryAttempts` equals caught message count) |

### Authz notes

Identity `EvaluateFull` on `ses:*` (v1 Query and v2 REST). List on `/v2/email/identities` requires `ses:ListEmailIdentities` (distinct from v1 `ses:ListIdentities`). Org SCP/RCP filters apply. Source address must be a verified identity or Send* returns MessageRejected.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws ses verify-email-identity --email-address lab@example.com --endpoint-url "$EP"

aws ses send-email \
  --from lab@example.com \
  --destination ToAddresses=to@example.com \
  --message "Subject={Data=hello,Charset=utf8},Body={Text={Data=caught,Charset=utf8}}" \
  --endpoint-url "$EP"

aws ses list-identities --endpoint-url "$EP"
aws ses get-send-statistics --endpoint-url "$EP"
```

SES v2 (AWS CLI v2 uses the REST protocol on the same endpoint):

```bash
aws sesv2 create-email-identity --email-identity lab@example.com --endpoint-url "$EP"
aws sesv2 list-email-identities --endpoint-url "$EP"
aws sesv2 send-email \
  --from-email-address lab@example.com \
  --destination ToAddresses=to@example.com \
  --content "Simple={Subject={Data=hello,Charset=utf8},Body={Text={Data=caught,Charset=utf8}}}" \
  --endpoint-url "$EP"
aws sesv2 get-account --endpoint-url "$EP"
```

## Not yet / deferred

- Real SMTP relay or receipt rules
- Configuration sets, templates, bulk/templated v2 send, Complaint/Delivery feedback depth (Bounce SNS stub ships)
- Domain DKIM Route53 verification theatre, suppression list CRUD, account sending pause PUT
- Full SES v2 surface (tags, identity policies, event destinations)
