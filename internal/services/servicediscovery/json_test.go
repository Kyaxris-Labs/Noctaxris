package servicediscovery_test

import (
	"encoding/json"
	"testing"

	sdsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/servicediscovery"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestServiceDiscoveryJSON(t *testing.T) {
	ns := store.SDNamespace{
		ID: "ns-1", ARN: "arn:aws:servicediscovery:us-east-1:1:namespace/ns-1",
		Name: "lab.local", Type: "DNS_PRIVATE", Vpc: "vpc-1",
	}
	nsRaw, err := sdsvc.CreatePrivateDnsNamespaceJSON(ns)
	if err != nil {
		t.Fatal(err)
	}
	var nsOut map[string]any
	if err := json.Unmarshal(nsRaw, &nsOut); err != nil {
		t.Fatal(err)
	}
	if nsOut["OperationId"] != "op-ns-1" {
		t.Fatalf("namespace=%v", nsOut)
	}

	ns.Vpc = ""
	nsRaw, err = sdsvc.CreatePrivateDnsNamespaceJSON(ns)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(nsRaw, &nsOut); err != nil {
		t.Fatal(err)
	}
	namespace, _ := nsOut["Namespace"].(map[string]any)
	if _, ok := namespace["Vpc"]; ok {
		t.Fatalf("empty vpc omitted: %v", namespace)
	}

	svc := store.SDService{ID: "svc-1", ARN: "arn:svc", Name: "api", NamespaceID: "ns-1"}
	svcRaw, err := sdsvc.CreateServiceJSON(svc)
	if err != nil {
		t.Fatal(err)
	}
	var svcOut map[string]any
	if err := json.Unmarshal(svcRaw, &svcOut); err != nil {
		t.Fatal(err)
	}
	service, _ := svcOut["Service"].(map[string]any)
	if service["NamespaceId"] != "ns-1" {
		t.Fatalf("service=%v", svcOut)
	}

	inst := store.SDInstance{InstanceID: "i-1", Attributes: map[string]string{"AWS_INSTANCE_IPV4": "10.0.0.1"}}
	regRaw, err := sdsvc.RegisterInstanceJSON(inst)
	if err != nil {
		t.Fatal(err)
	}
	var regOut map[string]string
	if err := json.Unmarshal(regRaw, &regOut); err != nil {
		t.Fatal(err)
	}
	if regOut["OperationId"] != "op-i-1" {
		t.Fatalf("register=%v", regOut)
	}

	deregRaw, err := sdsvc.DeregisterInstanceJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(deregRaw, &regOut); err != nil {
		t.Fatal(err)
	}

	discRaw, err := sdsvc.DiscoverInstancesJSON([]store.SDInstance{inst})
	if err != nil {
		t.Fatal(err)
	}
	var discOut map[string]any
	if err := json.Unmarshal(discRaw, &discOut); err != nil {
		t.Fatal(err)
	}
	items, _ := discOut["Instances"].([]any)
	if len(items) != 1 {
		t.Fatalf("discover=%v", discOut)
	}
}
