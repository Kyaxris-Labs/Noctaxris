package lightsail

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func instanceMap(inst store.LightsailInstance) map[string]any {
	return map[string]any{
		"name":             inst.Name,
		"arn":              inst.ARN,
		"supportCode":      "noctaxris/" + inst.Name,
		"createdAt":        float64(inst.CreatedAt) / 1000.0,
		"location": map[string]string{
			"availabilityZone": inst.AvailabilityZone,
			"regionName":       inst.Region,
		},
		"resourceType":     "Instance",
		"blueprintId":      inst.BlueprintID,
		"blueprintName":    inst.BlueprintID,
		"bundleId":         inst.BundleID,
		"isStaticIp":       false,
		"privateIpAddress": inst.PrivateIP,
		"publicIpAddress":  inst.PublicIP,
		"ipAddressType":    "ipv4",
		"state": map[string]any{
			"code": inst.StateCode,
			"name": inst.StateName,
		},
		"username": inst.Username,
	}
}

func operationMap(inst store.LightsailInstance, opType string) map[string]any {
	now := float64(time.Now().UTC().Unix())
	return map[string]any{
		"id":             inst.Name + "-" + opType,
		"resourceName":   inst.Name,
		"resourceType":   "Instance",
		"createdAt":      now,
		"location": map[string]string{
			"availabilityZone": inst.AvailabilityZone,
			"regionName":       inst.Region,
		},
		"isTerminal":      true,
		"operationType":   opType,
		"status":          "Succeeded",
		"statusChangedAt": now,
	}
}

// GetBlueprintsJSON builds GetBlueprints response.
func GetBlueprintsJSON() ([]byte, error) {
	items := make([]map[string]any, 0)
	for _, b := range store.LightsailBlueprints() {
		items = append(items, map[string]any{
			"blueprintId": b.BlueprintID,
			"name":        b.Name,
			"group":       b.Group,
			"type":        b.Type,
			"version":     b.Version,
			"platform":    b.Platform,
			"isActive":    b.IsActive,
		})
	}
	return json.Marshal(map[string]any{"blueprints": items})
}

// GetBundlesJSON builds GetBundles response.
func GetBundlesJSON() ([]byte, error) {
	items := make([]map[string]any, 0)
	for _, b := range store.LightsailBundles() {
		items = append(items, map[string]any{
			"bundleId":             b.BundleID,
			"name":                 b.Name,
			"price":                b.Price,
			"cpuCount":             b.CPUCount,
			"diskSizeInGb":         b.DiskSizeInGb,
			"memoryInGb":           b.MemoryInGb,
			"transferPerMonthInGb": b.TransferPerMonthInGb,
			"isActive":             b.IsActive,
		})
	}
	return json.Marshal(map[string]any{"bundles": items})
}

// CreateInstancesJSON builds CreateInstances response.
func CreateInstancesJSON(instances []store.LightsailInstance) ([]byte, error) {
	ops := make([]map[string]any, 0, len(instances))
	for _, inst := range instances {
		ops = append(ops, operationMap(inst, "CreateInstance"))
	}
	return json.Marshal(map[string]any{"operations": ops})
}

// GetInstanceJSON builds GetInstance response.
func GetInstanceJSON(inst store.LightsailInstance) ([]byte, error) {
	return json.Marshal(map[string]any{"instance": instanceMap(inst)})
}

// GetInstancesJSON builds GetInstances response.
func GetInstancesJSON(instances []store.LightsailInstance) ([]byte, error) {
	items := make([]map[string]any, 0, len(instances))
	for _, inst := range instances {
		items = append(items, instanceMap(inst))
	}
	return json.Marshal(map[string]any{"instances": items})
}

// InstanceOperationJSON builds Start/Stop/Reboot/Delete response.
func InstanceOperationJSON(inst store.LightsailInstance, opType string) ([]byte, error) {
	return json.Marshal(map[string]any{"operations": []map[string]any{operationMap(inst, opType)}})
}
