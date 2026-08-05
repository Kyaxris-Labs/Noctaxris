package mq_test

import (
	"encoding/json"
	"testing"

	mqsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/mq"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMQJSON(t *testing.T) {
	b := store.MQBroker{
		BrokerID: "b-1", BrokerARN: "arn:aws:mq:us-east-1:1:broker:b-1",
		BrokerName: "lab", BrokerState: store.MQBrokerStateRunning,
		EngineType: "ActiveMQ", EngineVersion: "5.17",
		DeploymentMode: "SINGLE_INSTANCE", HostInstanceType: "mq.t3.micro",
		StubEndpoint: "https://console.local", ContainerID: "cid-1",
	}
	createRaw, err := mqsvc.CreateBrokerJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	var createOut map[string]string
	if err := json.Unmarshal(createRaw, &createOut); err != nil {
		t.Fatal(err)
	}
	if createOut["BrokerId"] != "b-1" {
		t.Fatalf("create=%v", createOut)
	}

	descRaw, err := mqsvc.DescribeBrokerJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	var descOut map[string]any
	if err := json.Unmarshal(descRaw, &descOut); err != nil {
		t.Fatal(err)
	}
	instances, _ := descOut["BrokerInstances"].([]any)
	inst, _ := instances[0].(map[string]any)
	if inst["IpAddress"] != "" {
		t.Fatalf("running nested should clear ip: %v", inst)
	}

	b.ContainerID = ""
	b.BrokerState = store.MQBrokerStateCreationInProgress
	descRaw, err = mqsvc.DescribeBrokerJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(descRaw, &descOut); err != nil {
		t.Fatal(err)
	}
	instances, _ = descOut["BrokerInstances"].([]any)
	inst, _ = instances[0].(map[string]any)
	if inst["IpAddress"] != "127.0.0.1" {
		t.Fatalf("default ip=%v", inst["IpAddress"])
	}

	listRaw, err := mqsvc.ListBrokersJSON([]store.MQBroker{b})
	if err != nil {
		t.Fatal(err)
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRaw, &listOut); err != nil {
		t.Fatal(err)
	}
	sums, _ := listOut["BrokerSummaries"].([]any)
	if len(sums) != 1 {
		t.Fatalf("list=%v", listOut)
	}
}
