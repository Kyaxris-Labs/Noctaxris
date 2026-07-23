package compute

import (
	"github.com/docker/docker/api/types/container"
)

// nestedTaskPidsLimit caps processes inside nested Lambda/ECS lab containers.
const nestedTaskPidsLimit = int64(1024)

// nestedTaskSecurity applies CapDrop ALL, no Privileged/CapAdd, no-new-privileges,
// an optional memory limit, and a pids cgroup bound for nested lab tasks.
func nestedTaskSecurity(memoryMB int) container.HostConfig {
	pids := nestedTaskPidsLimit
	hc := container.HostConfig{
		Privileged:  false,
		CapDrop:     []string{"ALL"},
		SecurityOpt: []string{"no-new-privileges:true"},
		Resources: container.Resources{
			PidsLimit: &pids,
		},
	}
	if memoryMB > 0 {
		hc.Memory = int64(memoryMB) * 1024 * 1024
		hc.Resources.Memory = hc.Memory
	}
	return hc
}

func nestedTaskResources(sec container.HostConfig) container.Resources {
	res := container.Resources{Memory: sec.Memory}
	if sec.Resources.PidsLimit != nil {
		pids := *sec.Resources.PidsLimit
		res.PidsLimit = &pids
	}
	return res
}

// zipInvokeHostConfig builds HostConfig for zip Lambda Invoke (code binds are :ro).
func zipInvokeHostConfig(binds []string, memoryMB int) *container.HostConfig {
	sec := nestedTaskSecurity(memoryMB)
	return &container.HostConfig{
		Binds:          binds,
		AutoRemove:     false,
		NetworkMode:    container.NetworkMode(FunctionNetworkName),
		ExtraHosts:     hostGatewayExtraHosts(),
		ReadonlyRootfs: false,
		Privileged:     false,
		CapDrop:        append([]string(nil), sec.CapDrop...),
		SecurityOpt:    append([]string(nil), sec.SecurityOpt...),
		Resources:      nestedTaskResources(sec),
	}
}

// imageInvokeHostConfig builds HostConfig for Image Lambda Invoke.
func imageInvokeHostConfig(binds []string, memoryMB int) *container.HostConfig {
	// Same security floor as zip; separate helper so HostConfig unit locks stay path-explicit.
	return zipInvokeHostConfig(binds, memoryMB)
}

// ecsTaskHostConfig builds HostConfig for ECS / CodeBuild / Batch nested tasks.
func ecsTaskHostConfig(memoryMB int) *container.HostConfig {
	sec := nestedTaskSecurity(memoryMB)
	return &container.HostConfig{
		AutoRemove:  false,
		NetworkMode: container.NetworkMode(ECSNetworkName),
		ExtraHosts:  ecsHostGatewayExtraHosts(),
		Privileged:  false,
		CapDrop:     append([]string(nil), sec.CapDrop...),
		SecurityOpt: append([]string(nil), sec.SecurityOpt...),
		Resources:   nestedTaskResources(sec),
	}
}

// hostConfigSecurityOK reports Privileged=false, empty CapAdd, CapDrop ALL.
func hostConfigSecurityOK(hc *container.HostConfig) bool {
	if hc == nil || hc.Privileged {
		return false
	}
	if len(hc.CapAdd) > 0 {
		return false
	}
	if len(hc.CapDrop) != 1 || hc.CapDrop[0] != "ALL" {
		return false
	}
	return true
}
