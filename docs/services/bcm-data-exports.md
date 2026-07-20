# BCM Data Exports

**Status:** shipped (lab core)

CreateExport, ListExports, GetExport, and DeleteExport lite. Writes a CSV or JSON sample Cost and Usage shaped file under the data root. Identity authz. No scheduled CUR delivery pipeline.

## Implemented

| Area | Actions |
|------|---------|
| Exports | `CreateExport`, `GetExport`, `ListExports`, `DeleteExport` |

### Authz notes

Identity `EvaluateFull` on `bcm-data-exports:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws bcm-data-exports create-export --cli-input-json file://export.json --endpoint-url "$EP"
aws bcm-data-exports list-exports --endpoint-url "$EP"
```

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Real Cost and Usage Report delivery to S3 on a schedule
- Scheduler glue for recurring exports
- Parquet and compression variants
