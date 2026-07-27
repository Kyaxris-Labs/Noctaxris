# Cost and Usage Reports

**Status:** shipped (lab lite)

Report definition CRUD for the legacy CUR API. Optional tiny CSV/JSON lab artifact written to the configured S3 bucket on Put/Modify. No DuckDB sidecar and no real Parquet encoder.

**Protocol:** JSON 1.1  
**Header:** `X-Amz-Target: AWSOrigamiServiceGatewayService.<Action>`  
**Endpoint prefix:** `cur`

## Implemented

| Area | Actions |
|------|---------|
| Definitions | `PutReportDefinition`, `ModifyReportDefinition`, `DescribeReportDefinitions`, `DeleteReportDefinition` |
| Tags | `TagResource` / `UntagResource` / `ListTagsForResource` stub empty OK |
| Emission | Best-effort PutObject to `s3://S3Bucket/S3Prefix/ReportName/<runId>.csv` or `.json` |

### Validation

- `ReportName`: alphanumerics + `-_`, max 256
- `TimeUnit`: `HOURLY` / `DAILY` / `MONTHLY`
- `Format`: `textORcsv` or `Parquet` (Parquet is emitted as lab JSON, not binary Parquet)
- `Compression`: `ZIP` / `GZIP` with `textORcsv`; `Parquet` with `Format=Parquet`
- Required: `ReportName`, `TimeUnit`, `Format`, `Compression`, `S3Bucket`, `S3Region`
- Max 5 reports per account; duplicate names return `DuplicateReportNameException`

### Authz notes

Identity `EvaluateFull` on `cur:*`.

### Emission

Default: emit on every successful `PutReportDefinition` / `ModifyReportDefinition` when the destination bucket exists. Emission failures set `ReportStatus.LastStatus=ERROR` and do not roll back the definition.

| Variable | Default | Description |
|----------|---------|-------------|
| `NOCTAXRIS_CUR_EMIT` | on | Set `0` / `off` / `false` to skip S3 artifact writes |

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws s3 mb s3://my-billing --endpoint-url "$EP"
# Lab protocol is JSON (AWSOrigamiServiceGatewayService.* X-Amz-Target).
# SDK suites use signed JSON; AWS CLI cur may require matching SigV4 service cur.
```

Live Compose smoke skipped when Docker is unavailable. Prefer SDK triples under `tests/sdk/`.

## Not yet / deferred

- DuckDB / Parquet sidecar emission
- Daily scheduled emission executor
- FOCUS row projection from live service usage enumerators
- Bucket-policy preflight on the destination bucket beyond S3's own checks
