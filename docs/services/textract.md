# Textract

**Status:** shipped (lab core, canned stub)

`DetectDocumentText` and `AnalyzeDocument` accept `Document.Bytes` or `Document.S3Object` and return a deterministic PAGE/LINE/WORD Block list. No real OCR.

## Implemented

| Area | Actions |
|------|---------|
| Sync detect | `DetectDocumentText` |
| Sync analyze | `AnalyzeDocument` (FeatureTypes recorded, same canned Blocks) |
| Input | Inline Bytes or lab S3 object (must exist for S3Object) |
| Authz | Identity `EvaluateFull` on `textract:*` |

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws textract detect-document-text \
  --document Bytes=aGVsbG8= \
  --endpoint-url "$EP"
```

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Async StartDocumentAnalysis / GetDocumentAnalysis
- Queries, Forms, and Tables deep fidelity
- Real OCR engines
