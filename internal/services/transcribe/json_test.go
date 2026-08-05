package transcribe_test

import (
	"encoding/json"
	"testing"

	transcribesvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/transcribe"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestTranscribeJSON(t *testing.T) {
	j := store.TranscribeJob{
		JobName: "job-1", JobStatus: "COMPLETED", LanguageCode: "en-US",
		MediaURI: "s3://bucket/a.wav", TranscriptURI: "s3://bucket/out.json",
		CreatedAt: 1_700_000_000_000, CompletedAt: 1_700_000_100_000,
		FailureReason: "",
	}
	startRaw, err := transcribesvc.StartTranscriptionJobJSON(j)
	if err != nil {
		t.Fatal(err)
	}
	var startOut map[string]any
	if err := json.Unmarshal(startRaw, &startOut); err != nil {
		t.Fatal(err)
	}
	job, _ := startOut["TranscriptionJob"].(map[string]any)
	if job["Transcript"] == nil {
		t.Fatalf("start=%v", startOut)
	}

	j.FailureReason = "bad media"
	getRaw, err := transcribesvc.GetTranscriptionJobJSON(j)
	if err != nil {
		t.Fatal(err)
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRaw, &getOut); err != nil {
		t.Fatal(err)
	}
	job, _ = getOut["TranscriptionJob"].(map[string]any)
	if job["FailureReason"] != "bad media" {
		t.Fatalf("get=%v", getOut)
	}

	minimal := store.TranscribeJob{JobName: "j2", CreatedAt: 1_700_000_000_000}
	getRaw, err = transcribesvc.GetTranscriptionJobJSON(minimal)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(getRaw, &getOut); err != nil {
		t.Fatal(err)
	}
	job, _ = getOut["TranscriptionJob"].(map[string]any)
	if _, ok := job["Transcript"]; ok {
		t.Fatalf("minimal should omit transcript: %v", job)
	}

	listRaw, err := transcribesvc.ListTranscriptionJobsJSON([]store.TranscribeJob{j, minimal})
	if err != nil {
		t.Fatal(err)
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRaw, &listOut); err != nil {
		t.Fatal(err)
	}
	sums, _ := listOut["TranscriptionJobSummaries"].([]any)
	if len(sums) != 2 {
		t.Fatalf("list=%v", listOut)
	}
}
