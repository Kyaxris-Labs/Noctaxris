package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	ossvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/opensearch"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// tryStartNestedDataEngine starts a nested data engine via compute.StartDataPlane when DinD is configured.
// ElastiCache/DocDB: without DockerHost this is a no-op (control-plane may stay creating).
// MQ/OpenSearch: without DockerHost or on start/wait failure, status is fail-closed (CREATION_FAILED / CreateFailed).
// env is optional engine bootstrap. Never log env values.
func tryStartNestedDataEngine(s *Server, accountID, kind, name string, env map[string]string) error {
	if s == nil || strings.TrimSpace(accountID) == "" {
		return nil
	}
	dk := compute.DataKind(strings.ToLower(kind))
	switch dk {
	case compute.DataKindElastiCache, compute.DataKindDocDB, compute.DataKindMQ, compute.DataKindOpenSearch:
	default:
		return nil
	}
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		switch dk {
		case compute.DataKindMQ, compute.DataKindOpenSearch:
			_ = markNestedDataFailed(s, accountID, dk, name, "compute client unavailable")
		}
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
		_ = markNestedDataFailed(s, accountID, dk, name, err.Error())
		return err
	}
	waitErr := cli.WaitDataPlaneHealthy(ctx, inst.ContainerID)
	host := inst.Name
	if ep := strings.TrimSpace(inst.Endpoint); ep != "" {
		if i := strings.LastIndex(ep, ":"); i > 0 {
			host = ep[:i]
		}
	}
	if waitErr != nil && dk == compute.DataKindOpenSearch {
		evidence := waitErr.Error()
		if logs, logErr := cli.DataPlaneLogs(ctx, inst.ContainerID); logErr == nil && strings.TrimSpace(logs) != "" {
			evidence = evidence + "\n" + logs
		}
		_ = markNestedDataFailed(s, accountID, dk, name, evidence)
		return waitErr
	}
	return promoteNestedDataAfterWait(s, accountID, dk, name, inst.ContainerID, host, waitErr)
}

// promoteNestedDataAfterWait promotes nested data to ready only when wait succeeded.
// Wait errors mark failed (do not claim available/RUNNING/Active).
func promoteNestedDataAfterWait(
	s *Server, accountID string, dk compute.DataKind, name, containerID, host string, waitErr error,
) error {
	if waitErr != nil {
		_ = markNestedDataFailed(s, accountID, dk, name, waitErr.Error())
		return waitErr
	}
	switch dk {
	case compute.DataKindElastiCache:
		return s.store.SetElastiCacheContainerID(accountID, name, containerID, "available", host)
	case compute.DataKindDocDB:
		return s.store.SetDocDBContainerID(accountID, name, containerID, "available", host)
	case compute.DataKindMQ:
		ep := fmt.Sprintf("amqp://%s:%d", host, store.MQNestedPort)
		return s.store.SetMQContainerID(accountID, name, containerID, store.MQBrokerStateRunning, ep)
	case compute.DataKindOpenSearch:
		ep := fmt.Sprintf("%s:%d", host, store.OpenSearchNestedPort)
		return s.store.SetOpenSearchContainerID(accountID, name, containerID, store.OpenSearchDomainStatusActive, ep, "")
	default:
		return nil
	}
}

func markNestedDataFailed(s *Server, accountID string, dk compute.DataKind, name, evidence string) error {
	switch dk {
	case compute.DataKindElastiCache:
		return s.store.SetElastiCacheContainerID(accountID, name, "", "failed", "")
	case compute.DataKindDocDB:
		return s.store.SetDocDBContainerID(accountID, name, "", "failed", "")
	case compute.DataKindMQ:
		stub := fmt.Sprintf("stub://127.0.0.1/mq/%s", strings.TrimSpace(name))
		return s.store.SetMQContainerID(accountID, name, "", store.MQBrokerStateCreationFailed, stub)
	case compute.DataKindOpenSearch:
		reason := ossvc.ClassifyOpenSearchNestedFailure(evidence)
		return s.store.SetOpenSearchContainerID(accountID, name, "", store.OpenSearchDomainStatusCreateFailed, "", reason)
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
