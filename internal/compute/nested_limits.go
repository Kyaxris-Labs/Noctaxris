package compute

import (
	"github.com/docker/docker/api/types/container"
)

// nestedTaskSecurity applies CapDrop ALL and optional memory limit for nested lab tasks.
func nestedTaskSecurity(memoryMB int) container.HostConfig {
	hc := container.HostConfig{
		CapDrop: []string{"ALL"},
	}
	if memoryMB > 0 {
		hc.Memory = int64(memoryMB) * 1024 * 1024
	}
	return hc
}
