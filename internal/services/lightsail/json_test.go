package lightsail_test

import (
	"encoding/json"
	"testing"

	lightsailsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/lightsail"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLightsailJSON(t *testing.T) {
	if _, err := lightsailsvc.GetBlueprintsJSON(); err != nil {
		t.Fatal(err)
	}
	if _, err := lightsailsvc.GetBundlesJSON(); err != nil {
		t.Fatal(err)
	}
	inst := store.LightsailInstance{Name: "lab-vm", ARN: "arn:aws:lightsail:us-east-1:1:instance/lab-vm", StateName: "running"}
	if _, err := lightsailsvc.CreateInstancesJSON([]store.LightsailInstance{inst}); err != nil {
		t.Fatal(err)
	}
	raw, err := lightsailsvc.GetInstanceJSON(inst)
	if err != nil {
		t.Fatal(err)
	}
	var instOut map[string]any
	if err := json.Unmarshal(raw, &instOut); err != nil {
		t.Fatal(err)
	}
	if _, err := lightsailsvc.GetInstancesJSON([]store.LightsailInstance{inst}); err != nil {
		t.Fatal(err)
	}
	if _, err := lightsailsvc.InstanceOperationJSON(inst, "RebootInstances"); err != nil {
		t.Fatal(err)
	}
	disk := store.LightsailDisk{Name: "disk-1", ARN: "arn:aws:lightsail:us-east-1:1:disk/disk-1", SizeInGb: 8}
	if _, err := lightsailsvc.DiskOperationJSON(disk, "AttachDisk"); err != nil {
		t.Fatal(err)
	}
	if _, err := lightsailsvc.GetDiskJSON(disk); err != nil {
		t.Fatal(err)
	}
	if _, err := lightsailsvc.GetDisksJSON([]store.LightsailDisk{disk}); err != nil {
		t.Fatal(err)
	}
	ip := store.LightsailStaticIP{Name: "ip-1", IPAddress: "203.0.113.10"}
	if _, err := lightsailsvc.StaticIPOperationJSON(ip, "AllocateStaticIp"); err != nil {
		t.Fatal(err)
	}
	if _, err := lightsailsvc.GetStaticIPJSON(ip); err != nil {
		t.Fatal(err)
	}
	if _, err := lightsailsvc.GetStaticIPsJSON([]store.LightsailStaticIP{ip}); err != nil {
		t.Fatal(err)
	}
	kp := store.LightsailKeyPair{Name: "lab-key"}
	if _, err := lightsailsvc.CreateKeyPairJSON(kp, "cHJpdmF0ZUtleQ=="); err != nil {
		t.Fatal(err)
	}
	if _, err := lightsailsvc.GetKeyPairJSON(kp); err != nil {
		t.Fatal(err)
	}
	if _, err := lightsailsvc.GetKeyPairsJSON([]store.LightsailKeyPair{kp}); err != nil {
		t.Fatal(err)
	}
	if _, err := lightsailsvc.KeyPairOperationJSON(kp, "DeleteKeyPair"); err != nil {
		t.Fatal(err)
	}
	if _, err := lightsailsvc.PortOperationJSON(inst, "OpenInstancePublicPorts"); err != nil {
		t.Fatal(err)
	}
	port := store.LightsailPortState{FromPort: 443, ToPort: 443, Protocol: "tcp", State: "open"}
	if _, err := lightsailsvc.GetInstancePortStatesJSON([]store.LightsailPortState{port}); err != nil {
		t.Fatal(err)
	}
}
