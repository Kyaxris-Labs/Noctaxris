# Cost and Usage Reports

**Status:** shipped (lab lite)

Report definition CRUD for the legacy CUR API. Optional lab artifact written to the configured S3 bucket on Put/Modify. CSV emits without DuckDB. `Format=Parquet` (and `Format=FOCUS` with `Compression=Parquet`) stages FOCUS NDJSON then writes binary Parquet via nested DuckDB (`noctaxris-lab-duck` / `NOCTAXRIS_DUCKDB_URL`).

**Protocol:** JSON 1.1  
**Header:** `X-Amz-Target: AWSOrigamiServiceGatewayService.<Action>`  
**Endpoint prefix:** `cur`

## Implemented

| Area | Actions |
|------|---------|
| Definitions | `PutReportDefinition`, `ModifyReportDefinition`, `DescribeReportDefinitions`, `DeleteReportDefinition` |
| Tags | `TagResource` / `UntagResource` / `ListTagsForResource` stub empty OK |
| Emission | Best-effort PutObject: legacy or FOCUS CSV under `s3://S3Bucket/S3Prefix/ReportName/<runId>.csv`; Parquet under `.../<runId>.parquet` via DuckDB |
| FOCUS lite | In-process `ResourceUsageEnumerator` SPI (built-in S3 bucket + Lambda function counters); `ProjectFOCUSRows` → FOCUS 1.2 / CUR 2.0 columns |

### Validation

- `ReportName`: alphanumerics + `-_`, max 256
- `TimeUnit`: `HOURLY` / `DAILY` / `MONTHLY`
- `Format`: `textORcsv`, `Parquet`, or `FOCUS`
- `Compression`: `ZIP` / `GZIP` with `textORcsv`; `Parquet` with `Format=Parquet`; `ZIP` / `GZIP` / `Parquet` with `Format=FOCUS`
- `AdditionalSchemaElements`: `RESOURCES`, `SPLIT_COST_ALLOCATION_DATA`, `MANUAL_DISCOUNT_COMPATIBILITY`, `FOCUS`
- Required: `ReportName`, `TimeUnit`, `Format`, `Compression`, `S3Bucket`, `S3Region`
- Max 5 reports per account; duplicate names return `DuplicateReportNameException`

### Authz notes

Identity `EvaluateFull` on `cur:*`.

### Emission

Default: emit on every successful `PutReportDefinition` / `ModifyReportDefinition` when the destination bucket exists. Emission failures set `ReportStatus.LastStatus=ERROR` and do not roll back the definition.

FOCUS projection runs when `Format` is `Parquet` or `FOCUS`, or when `AdditionalSchemaElements` includes `FOCUS`. Enumerators count lab S3 buckets and Lambda functions in the account into `UsageLine` rows (catalog count line plus one row per resource).

| Path | Behavior |
|------|----------|
| `textORcsv` (no FOCUS) | Tiny legacy CUR-shaped CSV; no DuckDB |
| `textORcsv` + `FOCUS` schema, or `Format=FOCUS` + ZIP/GZIP | FOCUS CSV from live enumerators; no DuckDB |
| `Parquet`, or `Format=FOCUS` + `Compression=Parquet` | Stages FOCUS NDJSON under `noctaxris-cur-staging/<ReportName>/`, runs DuckDB `COPY ... (FORMAT PARQUET)` to `S3Prefix/ReportName/<runId>.parquet`, then deletes the staging object. Fail-closed to `ERROR` when DuckDB is unavailable (never SUCCESS with a JSON stand-in) |

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

Store unit tests cover FOCUS CSV (no DinD) and Parquet emit with a mocked Duck runner. Soft-skip live nested DuckDB when the engine or `floci/floci-duck` image is unavailable. Live Compose smoke skipped when Docker is unavailable. Prefer SDK triples under `tests/sdk/`.

## Not yet / deferred

- Daily scheduled emission executor
- Broader service enumerators beyond S3 buckets and Lambda functions
- Bucket-policy preflight on the destination bucket beyond S3's own checks
