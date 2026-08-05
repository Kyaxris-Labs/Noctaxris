package msk_test

import (
	"encoding/json"
	"testing"

	msksvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/msk"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMSKClusterJSON(t *testing.T) {
	c := store.MSKCluster{
		ClusterARN: "arn:aws:kafka:us-east-1:000000000001:cluster/lab/abc",
		ClusterName: "lab", State: store.MSKClusterStateActive,
		KafkaVersion: "3.5.1", NumberOfBrokerNodes: 2,
		ContainerID: "ctr-1", BootstrapBrokers: "broker:9092",
	}
	raw, err := msksvc.CreateClusterJSON(c)
	if err != nil {
		t.Fatal(err)
	}
	var create map[string]any
	if err := json.Unmarshal(raw, &create); err != nil {
		t.Fatal(err)
	}
	if create["ClusterName"] != "lab" {
		t.Fatalf("create=%v", create)
	}

	descRaw, err := msksvc.DescribeClusterJSON(c)
	if err != nil {
		t.Fatal(err)
	}
	var desc map[string]any
	_ = json.Unmarshal(descRaw, &desc)
	info, _ := desc["ClusterInfo"].(map[string]any)
	if info["NumberOfBrokerNodes"] != float64(2) {
		t.Fatalf("describe=%v", desc)
	}

	listRaw, err := msksvc.ListClustersJSON([]store.MSKCluster{c})
	if err != nil {
		t.Fatal(err)
	}
	var list map[string]any
	_ = json.Unmarshal(listRaw, &list)
	arr, _ := list["ClusterInfoList"].([]any)
	if len(arr) != 1 {
		t.Fatalf("list=%v", list)
	}

	bootRaw, err := msksvc.GetBootstrapBrokersJSON(c)
	if err != nil {
		t.Fatal(err)
	}
	var boot map[string]any
	_ = json.Unmarshal(bootRaw, &boot)
	if boot["BootstrapBrokerString"] != "broker:9092" {
		t.Fatalf("boot=%v", boot)
	}
	inactive := c
	inactive.ContainerID = ""
	bootRaw2, _ := msksvc.GetBootstrapBrokersJSON(inactive)
	_ = json.Unmarshal(bootRaw2, &boot)
	if boot["BootstrapBrokerString"] != "" {
		t.Fatalf("inactive boot=%v", boot)
	}

	delRaw, err := msksvc.DeleteClusterJSON(c.ClusterARN)
	if err != nil {
		t.Fatal(err)
	}
	var del map[string]any
	_ = json.Unmarshal(delRaw, &del)
	if del["State"] != store.MSKClusterStateDeleting {
		t.Fatalf("del=%v", del)
	}

	_, _ = msksvc.ListClustersJSON(nil)
}
