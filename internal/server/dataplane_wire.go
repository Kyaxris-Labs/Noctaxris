package server

import (
	"context"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

// tryStartNestedDataEngine starts a nested data engine via compute.StartDataPlane when DinD is configured.
// Without DockerHost, this is a no-op (control-plane endpoint still recorded in store).
// env is optional engine bootstrap (for example Mongo init credentials). Never log env values.
func tryStartNestedDataEngine(s *Server, accountID, kind, name string, env map[string]string) error {
	if s == nil || strings.TrimSpace(accountID) == "" {
		return nil
	}
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		return nil
	}
	dk := compute.DataKind(strings.ToLower(kind))
	switch dk {
	case compute.DataKindElastiCache, compute.DataKindDocDB:
	default:
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	inst, err := cli.StartDataPlane(ctx, compute.DataPlaneOpts{
		Kind:  dk,
		Image: compute.DefaultDataPlaneImage(dk),
		Name:  "noctaxris-" + string(dk) + "-" + strings.ToLower(name),
		Env:   env,
	})
	if err != nil {
		return err
	}
	_ = cli.WaitDataPlaneHealthy(ctx, inst.ContainerID)
	host := inst.Name
	if ep := strings.TrimSpace(inst.Endpoint); ep != "" {
		if i := strings.LastIndex(ep, ":"); i > 0 {
			host = ep[:i]
		}
	}
	switch dk {
	case compute.DataKindElastiCache:
		return s.store.SetElastiCacheContainerID(accountID, name, inst.ContainerID, "available", host)
	case compute.DataKindDocDB:
		return s.store.SetDocDBContainerID(accountID, name, inst.ContainerID, "available", host)
	default:
		return nil
	}
}

func tryStopNestedDataEngine(s *Server, containerID string) error {
	if s == nil || strings.TrimSpace(containerID) == "" {
		return nil
	}
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return cli.StopDataPlane(ctx, containerID)
}
