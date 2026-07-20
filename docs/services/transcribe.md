# Transcribe

**Status:** shipped (lab core, canned stub)

`StartTranscriptionJob` requires a lab `s3://bucket/key` MediaFileUri that exists (HeadObject). The job completes immediately with a canned transcript JSON under the data root. `GetTranscriptionJob` and `ListTranscriptionJobs` read stored job metadata. No real ASR and no fetch of non-lab URIs.

## Implemented

| Area | Actions |
|------|---------|
| Jobs | `StartTranscriptionJob`, `GetTranscriptionJob`, `ListTranscriptionJobs` |
| Input | `Media.MediaFileUri` must be `s3://` and exist in lab S3 |
| Output | Canned transcript file under data root (`file://...`) |
| Authz | Identity `EvaluateFull` on `transcribe:*` |

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws s3api create-bucket --bucket lab-bucket --endpoint-url "$EP"
echo audio > /tmp/audio.wav
aws s3api put-object --bucket lab-bucket --key audio.wav --body /tmp/audio.wav --endpoint-url "$EP"
aws transcribe start-transcription-job \
  --transcription-job-name "noctaxris-job-$RANDOM" \
  --language-code en-US \
  --media MediaFileUri=s3://lab-bucket/audio.wav \
  --endpoint-url "$EP"

aws transcribe list-transcription-jobs --endpoint-url "$EP"
```

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Streaming transcription
- Real ASR / Call Analytics
- Writing transcript objects into lab S3 buckets
