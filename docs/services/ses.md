# SES

**Status:** shipped (lab core)

Local email catcher. Verify identities, send mail into the store, list identities, and read send statistics. No outbound SMTP.

## Implemented

| Area | Actions |
|------|---------|
| Identities | `VerifyEmailIdentity` (lab auto-verify), `ListIdentities` |
| Send | `SendEmail`, `SendRawEmail` (persist only) |
| Stats | `GetSendStatistics` stub (`DeliveryAttempts` equals caught message count) |

### Authz notes

Identity `EvaluateFull` on `ses:*`. Org SCP/RCP filters apply. Source address must be a verified identity or Send* returns MessageRejected.

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

## Not yet / deferred

- Real SMTP relay or receipt rules
- Configuration sets, templates, bounce/complaint feedback
- Domain identity DKIM depth
- SES v2 API surface
