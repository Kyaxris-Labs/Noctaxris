package macie2

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// EnableMacieJSON builds EnableMacie empty success body.
func EnableMacieJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// GetMacieSessionJSON builds GetMacieSession response.
func GetMacieSessionJSON(sess store.MacieSession) ([]byte, error) {
	return json.Marshal(map[string]any{
		"status":    sess.Status,
		"createdAt": time.UnixMilli(sess.CreatedAt).UTC().Format(time.RFC3339),
		"updatedAt": time.UnixMilli(sess.UpdatedAt).UTC().Format(time.RFC3339),
	})
}

// CreateClassificationJobJSON builds CreateClassificationJob response.
func CreateClassificationJobJSON(job store.MacieJob) ([]byte, error) {
	return json.Marshal(map[string]any{
		"jobId":  job.JobID,
		"jobArn": job.JobARN,
	})
}

// DescribeClassificationJobJSON builds DescribeClassificationJob response.
func DescribeClassificationJobJSON(job store.MacieJob) ([]byte, error) {
	if job.Raw != nil {
		return json.Marshal(job.Raw)
	}
	return json.Marshal(map[string]any{
		"jobId":     job.JobID,
		"jobArn":    job.JobARN,
		"name":      job.Name,
		"jobType":   job.JobType,
		"jobStatus": job.JobStatus,
	})
}

// ListClassificationJobsJSON builds ListClassificationJobs response.
func ListClassificationJobsJSON(jobs []store.MacieJob) ([]byte, error) {
	items := make([]map[string]any, 0, len(jobs))
	for _, j := range jobs {
		items = append(items, map[string]any{
			"jobId":     j.JobID,
			"jobArn":    j.JobARN,
			"name":      j.Name,
			"jobType":   j.JobType,
			"jobStatus": j.JobStatus,
		})
	}
	return json.Marshal(map[string]any{"items": items})
}

// ListFindingsJSON builds ListFindings response.
func ListFindingsJSON(ids []string) ([]byte, error) {
	if ids == nil {
		ids = []string{}
	}
	return json.Marshal(map[string]any{"findingIds": ids})
}

// GetFindingsJSON builds GetFindings response (camelCase per Macie2 API).
func GetFindingsJSON(findings []store.MacieFinding) ([]byte, error) {
	if findings == nil {
		findings = []store.MacieFinding{}
	}
	return json.Marshal(map[string]any{"findings": findings})
}

// InjectFindingsJSON builds lab InjectFindings response.
func InjectFindingsJSON(ids []string) ([]byte, error) {
	if ids == nil {
		ids = []string{}
	}
	return json.Marshal(map[string]any{"findingIds": ids})
}
