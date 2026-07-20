# AppConfig

**Status:** shipped (lab core)

Applications, environments, hosted configuration profiles, GetConfiguration, and a cheap AppConfigData session path. Identity authz only.

## Implemented

| Area | Actions |
|------|---------|
| Control | `CreateApplication`, `CreateEnvironment`, `CreateConfigurationProfile` |
| Hosted | `CreateHostedConfigurationVersion` |
| Read | `GetConfiguration` |
| AppConfigData | `StartConfigurationSession`, `GetLatestConfiguration` |

Hosted content is stored as opaque bytes with a content type. Deployments and strategies are skipped. GetConfiguration returns the latest hosted version for the profile.

### Authz notes

Identity `EvaluateFull` on `appconfig:*` and `appconfigdata:*`. Org SCP/RCP filters apply. No resource policy path.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
APP=$(aws appconfig create-application --name lab-app --endpoint-url "$EP" --query Id --output text)
ENV=$(aws appconfig create-environment --application-id "$APP" --name dev --endpoint-url "$EP" --query Id --output text)
PROFILE=$(aws appconfig create-configuration-profile \
  --application-id "$APP" --name flags --location-uri hosted \
  --endpoint-url "$EP" --query Id --output text)

echo '{"feature":true}' | base64 > /tmp/cfg.b64
aws appconfig create-hosted-configuration-version \
  --application-id "$APP" \
  --configuration-profile-id "$PROFILE" \
  --content fileb:///tmp/cfg.b64 \
  --content-type application/json \
  --endpoint-url "$EP"

aws appconfig get-configuration \
  --application "$APP" \
  --environment "$ENV" \
  --configuration "$PROFILE" \
  --client-id lab \
  --endpoint-url "$EP"

TOKEN=$(aws appconfigdata start-configuration-session \
  --application-identifier "$APP" \
  --environment-identifier "$ENV" \
  --configuration-profile-identifier "$PROFILE" \
  --endpoint-url "$EP" \
  --query InitialConfigurationToken --output text)

aws appconfigdata get-latest-configuration \
  --configuration-token "$TOKEN" \
  --endpoint-url "$EP"
```

## Not yet / deferred

- Deployment strategies, validators, extensions
- Feature flags profile type depth
- Multi-environment deployment tracking
