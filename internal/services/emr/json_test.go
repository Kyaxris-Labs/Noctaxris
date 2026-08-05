package emr_test

import (
	"encoding/json"
	"testing"

	emrsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/emr"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestEMRJSON(t *testing.T) {
	cluster := store.EMRCluster{
		ClusterID: "j-ABC", ClusterName: "lab-cluster", Status: "WAITING",
		ReleaseLabel: "emr-6.15.0",
	}
	if _, err := emrsvc.RunJobFlowJSON(cluster); err != nil {
		t.Fatal(err)
	}
	desc, err := emrsvc.DescribeClusterJSON(cluster)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(desc, &body); err != nil {
		t.Fatal(err)
	}
	if _, err := emrsvc.ListClustersJSON([]store.EMRCluster{cluster}); err != nil {
		t.Fatal(err)
	}
	if _, err := emrsvc.AddJobFlowStepsJSON([]string{"s-1", "s-2"}); err != nil {
		t.Fatal(err)
	}
	step := store.EMRStep{StepID: "s-1", Name: "step", State: "COMPLETED", ClusterID: cluster.ClusterID}
	if _, err := emrsvc.DescribeStepJSON(step); err != nil {
		t.Fatal(err)
	}
	if _, err := emrsvc.ListStepsJSON([]store.EMRStep{step}); err != nil {
		t.Fatal(err)
	}
	if _, err := emrsvc.CancelStepsJSON([]store.EMRCancelStepInfo{{StepID: "s-1", Status: "CANCELLED"}}); err != nil {
		t.Fatal(err)
	}
	ig := store.EMRInstanceGroup{ID: "ig-1", Name: "master", Market: "ON_DEMAND", InstanceGroupType: "MASTER", InstanceType: "m5.xlarge", RequestedInstanceCount: 1}
	if _, err := emrsvc.ListInstanceGroupsJSON([]store.EMRInstanceGroup{ig}); err != nil {
		t.Fatal(err)
	}
	fleet := store.EMRInstanceFleet{ID: "if-1", Name: "core", TargetOnDemandCapacity: 2}
	if _, err := emrsvc.ListInstanceFleetsJSON([]store.EMRInstanceFleet{fleet}); err != nil {
		t.Fatal(err)
	}
	sc := store.EMRSecurityConfiguration{Name: "lab-sc", SecurityConfiguration: `{"EncryptionConfiguration":{}}`, CreatedAt: 1_700_000_000_000}
	if _, err := emrsvc.CreateSecurityConfigurationJSON(sc); err != nil {
		t.Fatal(err)
	}
	if _, err := emrsvc.DescribeSecurityConfigurationJSON(sc); err != nil {
		t.Fatal(err)
	}
	if _, err := emrsvc.ListSecurityConfigurationsJSON([]store.EMRSecurityConfiguration{sc}); err != nil {
		t.Fatal(err)
	}
}
