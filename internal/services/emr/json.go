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
	cluster := map[string]any{
		"Id":           c.ClusterID,
		"Name":         c.ClusterName,
		"ClusterArn":   c.ClusterARN,
		"Status":       map[string]any{"State": c.Status},
		"ReleaseLabel": c.ReleaseLabel,
		"LogUri":       c.LogURI,
		"StatusTimeline": map[string]any{
			"CreationDateTime": time.UnixMilli(c.CreatedAt).UTC().Format(time.RFC3339),
		},
	}
	if len(c.Tags) > 0 {
		cluster["Tags"] = tagsToList(c.Tags)
	}
	return json.Marshal(map[string]any{"Cluster": cluster})
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

// CancelStepsJSON builds a CancelSteps success body.
func CancelStepsJSON(infos []store.EMRCancelStepInfo) ([]byte, error) {
	items := make([]map[string]any, 0, len(infos))
	for _, info := range infos {
		item := map[string]any{
			"StepId": info.StepID,
			"Status": info.Status,
		}
		if info.Reason != "" {
			item["Reason"] = info.Reason
		}
		items = append(items, item)
	}
	return json.Marshal(map[string]any{"CancelStepsInfoList": items})
}

// ListInstanceGroupsJSON builds a ListInstanceGroups success body.
func ListInstanceGroupsJSON(groups []store.EMRInstanceGroup) ([]byte, error) {
	items := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		item := map[string]any{
			"Id":                     g.ID,
			"InstanceGroupType":      g.InstanceGroupType,
			"InstanceType":           g.InstanceType,
			"RequestedInstanceCount": g.RequestedInstanceCount,
			"RunningInstanceCount":   g.RunningInstanceCount,
			"Status":                 map[string]any{"State": g.State},
		}
		if g.Name != "" {
			item["Name"] = g.Name
		}
		if g.Market != "" {
			item["Market"] = g.Market
		}
		if g.BidPrice != "" {
			item["BidPrice"] = g.BidPrice
		}
		items = append(items, item)
	}
	return json.Marshal(map[string]any{"InstanceGroups": items})
}

// ListInstanceFleetsJSON builds a ListInstanceFleets success body.
func ListInstanceFleetsJSON(fleets []store.EMRInstanceFleet) ([]byte, error) {
	items := make([]map[string]any, 0, len(fleets))
	for _, f := range fleets {
		item := map[string]any{
			"Id":                          f.ID,
			"InstanceFleetType":           f.InstanceFleetType,
			"TargetOnDemandCapacity":      f.TargetOnDemandCapacity,
			"TargetSpotCapacity":          f.TargetSpotCapacity,
			"ProvisionedOnDemandCapacity": f.ProvisionedOnDemandCapacity,
			"ProvisionedSpotCapacity":     f.ProvisionedSpotCapacity,
			"Status":                      map[string]any{"State": f.State},
		}
		if f.Name != "" {
			item["Name"] = f.Name
		}
		items = append(items, item)
	}
	return json.Marshal(map[string]any{"InstanceFleets": items})
}

// CreateSecurityConfigurationJSON builds a CreateSecurityConfiguration success body.
func CreateSecurityConfigurationJSON(sc store.EMRSecurityConfiguration) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Name":             sc.Name,
		"CreationDateTime": time.UnixMilli(sc.CreatedAt).UTC().Unix(),
	})
}

// DescribeSecurityConfigurationJSON builds a DescribeSecurityConfiguration success body.
func DescribeSecurityConfigurationJSON(sc store.EMRSecurityConfiguration) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Name":                  sc.Name,
		"SecurityConfiguration": sc.SecurityConfiguration,
		"CreationDateTime":      time.UnixMilli(sc.CreatedAt).UTC().Unix(),
	})
}

// ListSecurityConfigurationsJSON builds a ListSecurityConfigurations success body.
func ListSecurityConfigurationsJSON(configs []store.EMRSecurityConfiguration) ([]byte, error) {
	items := make([]map[string]any, 0, len(configs))
	for _, sc := range configs {
		items = append(items, map[string]any{
			"Name":             sc.Name,
			"CreationDateTime": time.UnixMilli(sc.CreatedAt).UTC().Unix(),
		})
	}
	return json.Marshal(map[string]any{"SecurityConfigurations": items})
}

func tagsToList(tags map[string]string) []map[string]string {
	out := make([]map[string]string, 0, len(tags))
	for k, v := range tags {
		out = append(out, map[string]string{"Key": k, "Value": v})
	}
	return out
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
		"State":    st.State,
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
