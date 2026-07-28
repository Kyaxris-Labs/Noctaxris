package emr

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// RunJobFlowJSON builds a RunJobFlow success body.
func RunJobFlowJSON(c store.EMRCluster) ([]byte, error) {
	return json.Marshal(map[string]any{
		"JobFlowId":  c.ClusterID,
		"ClusterArn": c.ClusterARN,
	})
}

// DescribeClusterJSON builds a DescribeCluster success body.
func DescribeClusterJSON(c store.EMRCluster) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Cluster": map[string]any{
			"Id":           c.ClusterID,
			"Name":         c.ClusterName,
			"ClusterArn":   c.ClusterARN,
			"Status":       map[string]any{"State": c.Status},
			"ReleaseLabel": c.ReleaseLabel,
			"LogUri":       c.LogURI,
			"StatusTimeline": map[string]any{
				"CreationDateTime": time.UnixMilli(c.CreatedAt).UTC().Format(time.RFC3339),
			},
		},
	})
}

// ListClustersJSON builds a ListClusters success body.
func ListClustersJSON(clusters []store.EMRCluster) ([]byte, error) {
	summaries := make([]map[string]any, 0, len(clusters))
	for _, c := range clusters {
		summaries = append(summaries, map[string]any{
			"Id":         c.ClusterID,
			"Name":       c.ClusterName,
			"ClusterArn": c.ClusterARN,
			"Status":     map[string]any{"State": c.Status},
		})
	}
	return json.Marshal(map[string]any{"Clusters": summaries})
}

// AddJobFlowStepsJSON builds an AddJobFlowSteps success body.
func AddJobFlowStepsJSON(stepIDs []string) ([]byte, error) {
	return json.Marshal(map[string]any{"StepIds": stepIDs})
}

// DescribeStepJSON builds a DescribeStep success body.
func DescribeStepJSON(st store.EMRStep) ([]byte, error) {
	return json.Marshal(map[string]any{"Step": stepToMap(st)})
}

// ListStepsJSON builds a ListSteps success body.
func ListStepsJSON(steps []store.EMRStep) ([]byte, error) {
	items := make([]map[string]any, 0, len(steps))
	for _, st := range steps {
		items = append(items, stepToMap(st))
	}
	return json.Marshal(map[string]any{"Steps": items})
}

func stepToMap(st store.EMRStep) map[string]any {
	args := st.Args
	if args == nil {
		args = []string{}
	}
	props := st.Properties
	if props == nil {
		props = map[string]string{}
	}
	config := map[string]any{
		"Args":       args,
		"Properties": props,
	}
	if st.Jar != "" {
		config["Jar"] = st.Jar
	}
	if st.MainClass != "" {
		config["MainClass"] = st.MainClass
	}
	status := map[string]any{
		"State": st.State,
		"Timeline": timelineMap(st.CreatedAt, st.StartAt, st.EndAt),
	}
	node := map[string]any{
		"Id":              st.StepID,
		"Name":            st.Name,
		"Config":          config,
		"ActionOnFailure": st.ActionOnFailure,
		"Status":          status,
	}
	if st.ExecutionRoleArn != "" {
		node["ExecutionRoleArn"] = st.ExecutionRoleArn
	}
	return node
}

func timelineMap(createdAt, startAt, endAt int64) map[string]any {
	out := map[string]any{}
	if createdAt > 0 {
		out["CreationDateTime"] = time.UnixMilli(createdAt).UTC().Format(time.RFC3339)
	}
	if startAt > 0 {
		out["StartDateTime"] = time.UnixMilli(startAt).UTC().Format(time.RFC3339)
	}
	if endAt > 0 {
		out["EndDateTime"] = time.UnixMilli(endAt).UTC().Format(time.RFC3339)
	}
	return out
}
