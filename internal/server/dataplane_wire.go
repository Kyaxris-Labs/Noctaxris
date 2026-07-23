package server

import (
	"context"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

// tryStartNestedDataEngine starts a nested data engine via compute.StartDataPlane when DinD is configured.
// Without DockerHost, this is a no-op (control-plane may stay creating).
// When DockerHost is set and start fails, status is marked failed (fail closed).
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
		_ = markNestedDataFailed(s, accountID, dk, name)
		return err
	}
	waitErr := cli.WaitDataPlaneHealthy(ctx, inst.ContainerID)
	host := inst.Name
	if ep := strings.TrimSpace(inst.Endpoint); ep != "" {
		if i := strings.LastIndex(ep, ":"); i > 0 {
			host = ep[:i]
		}
	}
	return promoteNestedDataAfterWait(s, accountID, dk, name, inst.ContainerID, host, waitErr)
}

// promoteNestedDataAfterWait promotes ElastiCache/DocDB to available only when wait succeeded.
// Wait errors mark failed (do not claim available).
func promoteNestedDataAfterWait(
	s *Server, accountID string, dk compute.DataKind, name, containerID, host string, waitErr error,
) error {
	if waitErr != nil {
		_ = markNestedDataFailed(s, accountID, dk, name)
		return waitErr
	}
	switch dk {
	case compute.DataKindElastiCache:
		return s.store.SetElastiCacheContainerID(accountID, name, containerID, "available", host)
	case compute.DataKindDocDB:
		return s.store.SetDocDBContainerID(accountID, name, containerID, "available", host)
	default:
		return nil
	}
}

func markNestedDataFailed(s *Server, accountID string, dk compute.DataKind, name string) error {
	switch dk {
	case compute.DataKindElastiCache:
		return s.store.SetElastiCacheContainerID(accountID, name, "", "failed", "")
	case compute.DataKindDocDB:
		return s.store.SetDocDBContainerID(accountID, name, "", "failed", "")
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
