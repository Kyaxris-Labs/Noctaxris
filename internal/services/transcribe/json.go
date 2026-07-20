package transcribe

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func jobMap(j store.TranscribeJob) map[string]any {
	out := map[string]any{
		"TranscriptionJobName": j.JobName,
		"TranscriptionJobStatus": j.JobStatus,
		"LanguageCode":         j.LanguageCode,
		"Media":                map[string]any{"MediaFileUri": j.MediaURI},
		"CreationTime":         time.UnixMilli(j.CreatedAt).UTC().Format(time.RFC3339),
	}
	if j.TranscriptURI != "" {
		out["Transcript"] = map[string]any{"TranscriptFileUri": j.TranscriptURI}
	}
	if j.CompletedAt > 0 {
		out["CompletionTime"] = time.UnixMilli(j.CompletedAt).UTC().Format(time.RFC3339)
	}
	if j.FailureReason != "" {
		out["FailureReason"] = j.FailureReason
	}
	return out
}

// StartTranscriptionJobJSON builds a StartTranscriptionJob success body.
func StartTranscriptionJobJSON(j store.TranscribeJob) ([]byte, error) {
	return json.Marshal(map[string]any{"TranscriptionJob": jobMap(j)})
}

// GetTranscriptionJobJSON builds a GetTranscriptionJob success body.
func GetTranscriptionJobJSON(j store.TranscribeJob) ([]byte, error) {
	return json.Marshal(map[string]any{"TranscriptionJob": jobMap(j)})
}

// ListTranscriptionJobsJSON builds a ListTranscriptionJobs success body.
func ListTranscriptionJobsJSON(jobs []store.TranscribeJob) ([]byte, error) {
	summaries := make([]map[string]any, 0, len(jobs))
	for _, j := range jobs {
		summaries = append(summaries, map[string]any{
			"TranscriptionJobName":   j.JobName,
			"TranscriptionJobStatus": j.JobStatus,
			"LanguageCode":           j.LanguageCode,
			"CreationTime":           time.UnixMilli(j.CreatedAt).UTC().Format(time.RFC3339),
		})
	}
	return json.Marshal(map[string]any{"TranscriptionJobSummaries": summaries})
}
