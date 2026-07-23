package servicediscovery

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreatePrivateDnsNamespaceJSON builds CreatePrivateDnsNamespace / CreateHttpNamespace response.
func CreatePrivateDnsNamespaceJSON(n store.SDNamespace) ([]byte, error) {
	ns := map[string]any{
		"Id":   n.ID,
		"Arn":  n.ARN,
		"Name": n.Name,
		"Type": n.Type,
	}
	if n.Vpc != "" {
		ns["Vpc"] = n.Vpc
	}
	return json.Marshal(map[string]any{
		"OperationId": "op-" + n.ID,
		"Namespace":   ns,
	})
}

// CreateServiceJSON builds CreateService response.
func CreateServiceJSON(svc store.SDService) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Service": map[string]any{
			"Id":          svc.ID,
			"Arn":         svc.ARN,
			"Name":        svc.Name,
			"NamespaceId": svc.NamespaceID,
		},
	})
}

// RegisterInstanceJSON builds RegisterInstance response.
func RegisterInstanceJSON(inst store.SDInstance) ([]byte, error) {
	return json.Marshal(map[string]any{
		"OperationId": "op-" + inst.InstanceID,
	})
}

// DeregisterInstanceJSON builds DeregisterInstance response.
func DeregisterInstanceJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"OperationId": "op-deregister"})
}

// DiscoverInstancesJSON builds DiscoverInstances response.
func DiscoverInstancesJSON(instances []store.SDInstance) ([]byte, error) {
	items := make([]map[string]any, 0, len(instances))
	for _, inst := range instances {
		items = append(items, map[string]any{
			"InstanceId": inst.InstanceID,
			"Attributes": inst.Attributes,
		})
	}
	return json.Marshal(map[string]any{"Instances": items})
}
