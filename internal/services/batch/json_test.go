package batch_test

import (
	"encoding/json"
	"testing"

	batchsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/batch"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestBatchJSON(t *testing.T) {
	ce := store.BatchComputeEnvironment{Name: "ce", ARN: "arn:ce", Type: "MANAGED", State: "ENABLED", Status: "VALID", ServiceRole: "arn:role"}
	raw, err := batchsvc.CreateComputeEnvironmentJSON(ce)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	jq := store.BatchJobQueue{Name: "q", ARN: "arn:q", State: "ENABLED", Status: "VALID", Priority: 1, CEOrders: `[{"order":1,"computeEnvironment":"ce"}]`}
	raw, _ = batchsvc.CreateJobQueueJSON(jq)

	jd := store.BatchJobDefinition{
		Name: "jd", ARN: "arn:jd", Revision: 1, Type: "container", Status: "ACTIVE",
		Image: "img", Command: []string{"echo"}, JobRoleARN: "jr", ExecutionRoleARN: "er",
		EnvJSON: `{"FOO":"bar"}`,
	}
	raw, _ = batchsvc.RegisterJobDefinitionJSON(jd)
	jd.EnvJSON = `{bad`
	raw, _ = batchsvc.DescribeJobDefinitionsJSON([]store.BatchJobDefinition{jd})

	job := store.BatchJob{
		JobID: "j1", JobName: "n", JobQueue: "q", JobDefARN: jd.ARN, Status: "RUNNING",
		CreatedAt: "1", StartedAt: "2", StoppedAt: "", ContainerID: "ctr",
	}
	raw, _ = batchsvc.SubmitJobJSON(job)

	raw, _ = batchsvc.DescribeComputeEnvironmentsJSON([]store.BatchComputeEnvironment{ce})
	raw, _ = batchsvc.DescribeJobQueuesJSON([]store.BatchJobQueue{jq})
	jq.CEOrders = `{invalid`
	raw, _ = batchsvc.DescribeJobQueuesJSON([]store.BatchJobQueue{jq})
	raw, _ = batchsvc.DescribeJobsJSON([]store.BatchJob{job})
	_ = json.Unmarshal(raw, &out)
}
