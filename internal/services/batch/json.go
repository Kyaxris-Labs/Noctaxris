package batch

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func CreateComputeEnvironmentJSON(ce store.BatchComputeEnvironment) ([]byte, error) {
	return json.Marshal(map[string]any{
		"computeEnvironmentName": ce.Name,
		"computeEnvironmentArn":  ce.ARN,
	})
}

func CreateJobQueueJSON(jq store.BatchJobQueue) ([]byte, error) {
	return json.Marshal(map[string]any{
		"jobQueueName": jq.Name,
		"jobQueueArn":  jq.ARN,
	})
}

func RegisterJobDefinitionJSON(jd store.BatchJobDefinition) ([]byte, error) {
	return json.Marshal(map[string]any{
		"jobDefinitionName": jd.Name,
		"jobDefinitionArn":  jd.ARN,
		"revision":          jd.Revision,
	})
}

func SubmitJobJSON(job store.BatchJob) ([]byte, error) {
	return json.Marshal(map[string]any{
		"jobName": job.JobName,
		"jobId":   job.JobID,
		"jobArn":  "arn:aws:batch:us-east-1:local:job/" + job.JobID,
	})
}

func DescribeComputeEnvironmentsJSON(ces []store.BatchComputeEnvironment) ([]byte, error) {
	items := make([]map[string]any, 0, len(ces))
	for _, ce := range ces {
		items = append(items, map[string]any{
			"computeEnvironmentName": ce.Name,
			"computeEnvironmentArn":  ce.ARN,
			"type":                   ce.Type,
			"state":                  ce.State,
			"status":                 ce.Status,
			"serviceRole":            ce.ServiceRole,
		})
	}
	return json.Marshal(map[string]any{"computeEnvironments": items})
}

func DescribeJobQueuesJSON(queues []store.BatchJobQueue) ([]byte, error) {
	items := make([]map[string]any, 0, len(queues))
	for _, jq := range queues {
		var order []any
		_ = json.Unmarshal([]byte(jq.CEOrders), &order)
		items = append(items, map[string]any{
			"jobQueueName":            jq.Name,
			"jobQueueArn":             jq.ARN,
			"state":                   jq.State,
			"status":                  jq.Status,
			"priority":                jq.Priority,
			"computeEnvironmentOrder": order,
		})
	}
	return json.Marshal(map[string]any{"jobQueues": items})
}

func DescribeJobDefinitionsJSON(defs []store.BatchJobDefinition) ([]byte, error) {
	items := make([]map[string]any, 0, len(defs))
	for _, jd := range defs {
		var env map[string]string
		_ = json.Unmarshal([]byte(jd.EnvJSON), &env)
		items = append(items, map[string]any{
			"jobDefinitionName": jd.Name,
			"jobDefinitionArn":  jd.ARN,
			"revision":          jd.Revision,
			"type":              jd.Type,
			"status":            jd.Status,
			"containerProperties": map[string]any{
				"image":              jd.Image,
				"command":            jd.Command,
				"jobRoleArn":         jd.JobRoleARN,
				"executionRoleArn":   jd.ExecutionRoleARN,
				"environment":        envToList(env),
			},
		})
	}
	return json.Marshal(map[string]any{"jobDefinitions": items})
}

func DescribeJobsJSON(jobs []store.BatchJob) ([]byte, error) {
	items := make([]map[string]any, 0, len(jobs))
	for _, j := range jobs {
		items = append(items, map[string]any{
			"jobId":                 j.JobID,
			"jobName":               j.JobName,
			"jobQueue":              j.JobQueue,
			"jobDefinition":         j.JobDefARN,
			"status":                j.Status,
			"createdAt":             j.CreatedAt,
			"startedAt":             j.StartedAt,
			"stoppedAt":             j.StoppedAt,
			"container": map[string]any{
				"containerInstanceArn": j.ContainerID,
			},
		})
	}
	return json.Marshal(map[string]any{"jobs": items})
}

func envToList(env map[string]string) []map[string]string {
	if len(env) == 0 {
		return []map[string]string{}
	}
	out := make([]map[string]string, 0, len(env))
	for k, v := range env {
		out = append(out, map[string]string{"name": k, "value": v})
	}
	return out
}
