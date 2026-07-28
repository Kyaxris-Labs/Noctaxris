# Cost and Usage Reports

**Status:** shipped (lab lite)

Report definition CRUD for the legacy CUR API. Optional tiny lab artifact written to the configured S3 bucket on Put/Modify. CSV emits without DuckDB. `Format=Parquet` stages UsageLine NDJSON then writes binary Parquet via nested DuckDB (`noctaxris-lab-duck` / `NOCTAXRIS_DUCKDB_URL`).

**Protocol:** JSON 1.1  
**Header:** `X-Amz-Target: AWSOrigamiServiceGatewayService.<Action>`  
**Endpoint prefix:** `cur`

## Implemented

| Area | Actions |
|------|---------|
| Definitions | `PutReportDefinition`, `ModifyReportDefinition`, `DescribeReportDefinitions`, `DeleteReportDefinition` |
| Tags | `TagResource` / `UntagResource` / `ListTagsForResource` stub empty OK |
| Emission | Best-effort PutObject: CSV under `s3://S3Bucket/S3Prefix/ReportName/<runId>.csv`; Parquet under `.../<runId>.parquet` via DuckDB |

### Validation

- `ReportName`: alphanumerics + `-_`, max 256
- `TimeUnit`: `HOURLY` / `DAILY` / `MONTHLY`
- `Format`: `textORcsv` or `Parquet`
- `Compression`: `ZIP` / `GZIP` with `textORcsv`; `Parquet` with `Format=Parquet`
- Required: `ReportName`, `TimeUnit`, `Format`, `Compression`, `S3Bucket`, `S3Region`
- Max 5 reports per account; duplicate names return `DuplicateReportNameException`

### Authz notes

Identity `EvaluateFull` on `cur:*`.

### Emission

Default: emit on every successful `PutReportDefinition` / `ModifyReportDefinition` when the destination bucket exists. Emission failures set `ReportStatus.LastStatus=ERROR` and do not roll back the definition.

| Path | Behavior |
|------|----------|
| `textORcsv` | Writes a tiny CSV object; no DuckDB |
| `Parquet` | Stages NDJSON under `noctaxris-cur-staging/<ReportName>/`, runs DuckDB `COPY ... (FORMAT PARQUET)` to `S3Prefix/ReportName/<runId>.parquet`, then deletes the staging object. Fail-closed to `ERROR` when DuckDB is unavailable (never SUCCESS with a JSON stand-in) |

| Variable | Default | Description |
|----------|---------|-------------|
| `NOCTAXRIS_CUR_EMIT` | on | Set `0` / `off` / `false` to skip S3 artifact writes |
| `NOCTAXRIS_DUCKDB_URL` | empty | Pre-configured DuckDB HTTP base URL (skips nested `noctaxris-lab-duck` ensure); shared with Athena |
| `NOCTAXRIS_DUCKDB_IMAGE` | `floci/floci-duck:latest` | Allowlisted nested DuckDB HTTP shim image |
| `NOCTAXRIS_DUCKDB_S3_ENDPOINT` | `http://host.docker.internal:4566` | Lab S3/API URL as seen from the DuckDB container |

Nested DuckDB stays on `noctaxris-data` with no host port publish. The API reaches `/query` via DinD exec (same path as Athena).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws s3 mb s3://my-billing --endpoint-url "$EP"
# Lab protocol is JSON (AWSOrigamiServiceGatewayService.* X-Amz-Target).
# SDK suites use signed JSON; AWS CLI cur may require matching SigV4 service cur.
```

Live Compose smoke skipped when Docker is unavailable. Prefer SDK triples under `tests/sdk/`. Soft-skip live nested DuckDB when the engine or `floci/floci-duck` image is unavailable; store/server unit tests cover Parquet fail-closed and mocked Duck HTTP/exec hooks.

## Not yet / deferred

- Daily scheduled emission executor
- FOCUS row projection from live service usage enumerators
- Bucket-policy preflight on the destination bucket beyond S3's own checks
