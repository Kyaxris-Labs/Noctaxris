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
		"isStaticIp":       inst.IsStaticIP,
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

func operationMap(resourceName, resourceType, az, region, opType string) map[string]any {
	now := float64(time.Now().UTC().Unix())
	return map[string]any{
		"id":             resourceName + "-" + opType,
		"resourceName":   resourceName,
		"resourceType":   resourceType,
		"createdAt":      now,
		"location": map[string]string{
			"availabilityZone": az,
			"regionName":       region,
		},
		"isTerminal":      true,
		"operationType":   opType,
		"status":          "Succeeded",
		"statusChangedAt": now,
	}
}

func instanceOperationMap(inst store.LightsailInstance, opType string) map[string]any {
	return operationMap(inst.Name, "Instance", inst.AvailabilityZone, inst.Region, opType)
}

func diskMap(d store.LightsailDisk) map[string]any {
	m := map[string]any{
		"name":         d.Name,
		"arn":          d.ARN,
		"supportCode":  "noctaxris/" + d.Name,
		"createdAt":    float64(d.CreatedAt) / 1000.0,
		"location": map[string]string{
			"availabilityZone": d.AvailabilityZone,
			"regionName":       d.Region,
		},
		"resourceType":    "Disk",
		"sizeInGb":        d.SizeInGb,
		"iops":            d.Iops,
		"path":            d.Path,
		"state":           d.State,
		"isAttached":      d.IsAttached,
		"isSystemDisk":    false,
		"attachmentState": "detached",
	}
	if d.IsAttached {
		m["attachedTo"] = d.AttachedTo
		m["attachmentState"] = "attached"
	}
	return m
}

func staticIPMap(ip store.LightsailStaticIP) map[string]any {
	m := map[string]any{
		"name":        ip.Name,
		"arn":         ip.ARN,
		"supportCode": "noctaxris/" + ip.Name,
		"createdAt":   float64(ip.CreatedAt) / 1000.0,
		"location": map[string]string{
			"availabilityZone": ip.Region + "a",
			"regionName":       ip.Region,
		},
		"resourceType": "StaticIp",
		"ipAddress":    ip.IPAddress,
		"isAttached":   ip.IsAttached,
	}
	if ip.IsAttached {
		m["attachedTo"] = ip.AttachedTo
	}
	return m
}

func keyPairMap(kp store.LightsailKeyPair) map[string]any {
	return map[string]any{
		"name":        kp.Name,
		"arn":         kp.ARN,
		"supportCode": "noctaxris/" + kp.Name,
		"createdAt":   float64(kp.CreatedAt) / 1000.0,
		"location": map[string]string{
			"availabilityZone": kp.Region + "a",
			"regionName":       kp.Region,
		},
		"resourceType": "KeyPair",
		"fingerprint":  kp.Fingerprint,
	}
}

func portStateMap(p store.LightsailPortState) map[string]any {
	return map[string]any{
		"fromPort": p.FromPort,
		"toPort":   p.ToPort,
		"protocol": p.Protocol,
		"cidrs":    p.Cidrs,
		"state":    p.State,
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
		ops = append(ops, instanceOperationMap(inst, "CreateInstance"))
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
	return json.Marshal(map[string]any{"operations": []map[string]any{instanceOperationMap(inst, opType)}})
}

// DiskOperationJSON builds disk mutation responses.
func DiskOperationJSON(d store.LightsailDisk, opType string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"operations": []map[string]any{operationMap(d.Name, "Disk", d.AvailabilityZone, d.Region, opType)},
	})
}

// GetDiskJSON builds GetDisk response.
func GetDiskJSON(d store.LightsailDisk) ([]byte, error) {
	return json.Marshal(map[string]any{"disk": diskMap(d)})
}

// GetDisksJSON builds GetDisks response.
func GetDisksJSON(disks []store.LightsailDisk) ([]byte, error) {
	items := make([]map[string]any, 0, len(disks))
	for _, d := range disks {
		items = append(items, diskMap(d))
	}
	return json.Marshal(map[string]any{"disks": items})
}

// StaticIPOperationJSON builds static IP mutation responses.
func StaticIPOperationJSON(ip store.LightsailStaticIP, opType string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"operations": []map[string]any{operationMap(ip.Name, "StaticIp", ip.Region+"a", ip.Region, opType)},
	})
}

// GetStaticIPJSON builds GetStaticIp response.
func GetStaticIPJSON(ip store.LightsailStaticIP) ([]byte, error) {
	return json.Marshal(map[string]any{"staticIp": staticIPMap(ip)})
}

// GetStaticIPsJSON builds GetStaticIps response.
func GetStaticIPsJSON(ips []store.LightsailStaticIP) ([]byte, error) {
	items := make([]map[string]any, 0, len(ips))
	for _, ip := range ips {
		items = append(items, staticIPMap(ip))
	}
	return json.Marshal(map[string]any{"staticIps": items})
}

// CreateKeyPairJSON builds CreateKeyPair response.
func CreateKeyPairJSON(kp store.LightsailKeyPair, privateKeyBase64 string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"keyPair":          keyPairMap(kp),
		"publicKeyBase64":  kp.PublicKeyBase64,
		"privateKeyBase64": privateKeyBase64,
		"operation":        operationMap(kp.Name, "KeyPair", kp.Region+"a", kp.Region, "CreateKeyPair"),
	})
}

// GetKeyPairJSON builds GetKeyPair response.
func GetKeyPairJSON(kp store.LightsailKeyPair) ([]byte, error) {
	return json.Marshal(map[string]any{"keyPair": keyPairMap(kp)})
}

// GetKeyPairsJSON builds GetKeyPairs response.
func GetKeyPairsJSON(pairs []store.LightsailKeyPair) ([]byte, error) {
	items := make([]map[string]any, 0, len(pairs))
	for _, kp := range pairs {
		items = append(items, keyPairMap(kp))
	}
	return json.Marshal(map[string]any{"keyPairs": items})
}

// KeyPairOperationJSON builds DeleteKeyPair response.
func KeyPairOperationJSON(kp store.LightsailKeyPair, opType string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"operation": operationMap(kp.Name, "KeyPair", kp.Region+"a", kp.Region, opType),
	})
}

// PortOperationJSON builds Open/CloseInstancePublicPorts response.
func PortOperationJSON(inst store.LightsailInstance, opType string) ([]byte, error) {
	return json.Marshal(map[string]any{"operation": instanceOperationMap(inst, opType)})
}

// GetInstancePortStatesJSON builds GetInstancePortStates response.
func GetInstancePortStatesJSON(ports []store.LightsailPortState) ([]byte, error) {
	items := make([]map[string]any, 0, len(ports))
	for _, p := range ports {
		items = append(items, portStateMap(p))
	}
	return json.Marshal(map[string]any{"portStates": items})
}
