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

// stopDataPlaneHook is set by MSK unit tests to observe StopDataPlane calls without DinD.
var stopDataPlaneHook func(containerID string)

// tryStartNestedDataEngine starts a nested data engine via compute.StartDataPlane when DinD is configured.
// ElastiCache/MemoryDB/DocDB/Neptune: without DockerHost this is a no-op (control-plane may stay creating).
// MQ/OpenSearch/MSK: without DockerHost or on start/wait failure, status is fail-closed.
// env is optional engine bootstrap. Never log env values.
// imageOverride, when non-empty, selects the container image (used for MQ EngineType RabbitMQ vs ActiveMQ).
func tryStartNestedDataEngine(s *Server, accountID, kind, name string, env map[string]string, imageOverride ...string) error {
	image := ""
	if len(imageOverride) > 0 {
		image = imageOverride[0]
	}
	return tryStartNestedDataEngineWithOpts(s, accountID, kind, name, env, image, nil, "", 0)
}

// tryStartNestedDataEngineWithOpts is like tryStartNestedDataEngine with optional Cmd, container name, and port.
// containerPort zero selects the default for Kind.
func tryStartNestedDataEngineWithOpts(
	s *Server, accountID, kind, name string, env map[string]string, image string, cmd []string, containerName string, containerPort int,
) error {
	if s == nil || strings.TrimSpace(accountID) == "" {
		return nil
	}
	dk := compute.DataKind(strings.ToLower(kind))
	switch dk {
	case compute.DataKindElastiCache, compute.DataKindMemoryDB, compute.DataKindDocDB,
		compute.DataKindMQ, compute.DataKindOpenSearch, compute.DataKindNeptune, compute.DataKindMSK:
	default:
		return nil
	}
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		switch dk {
		case compute.DataKindMQ, compute.DataKindOpenSearch, compute.DataKindMSK:
			_ = markNestedDataFailed(s, accountID, dk, name, "compute client unavailable")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if strings.TrimSpace(image) == "" {
		image = compute.DefaultDataPlaneImage(dk)
	}
	cname := strings.TrimSpace(containerName)
	if cname == "" {
		cname = "noctaxris-" + string(dk) + "-" + strings.ToLower(name)
	}
	inst, err := cli.StartDataPlane(ctx, compute.DataPlaneOpts{
		Kind:          dk,
		Image:         image,
		Name:          cname,
		Env:           env,
		Cmd:           cmd,
		ContainerPort: containerPort,
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

// tryStartNestedMQ starts nested RabbitMQ or ActiveMQ with the engine-specific image and env.
func tryStartNestedMQ(s *Server, accountID, brokerID, engineType string) error {
	return tryStartNestedDataEngine(
		s, accountID, "mq", brokerID,
		mqNestedBootstrapEnv(engineType),
		compute.DefaultDataPlaneImageForMQ(engineType),
	)
}

// tryStartNestedNeptune starts nested Gremlin Server or Neo4j for a Neptune cluster.
func tryStartNestedNeptune(s *Server, accountID, clusterID, graphEngine string) error {
	return tryStartNestedDataEngineWithOpts(
		s, accountID, "neptune", clusterID,
		compute.NeptuneNestedBootstrapEnv(graphEngine),
		compute.DefaultDataPlaneImageForNeptune(graphEngine),
		nil, "",
		compute.DefaultDataPlanePortForNeptune(graphEngine),
	)
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
	case compute.DataKindMemoryDB:
		// Keep CreateCluster DNS-style Address (*.memorydb.noctaxris.internal); host is DinD-only.
		return s.store.SetMemoryDBContainerID(accountID, name, containerID, "available", "")
	case compute.DataKindDocDB:
		return s.store.SetDocDBContainerID(accountID, name, containerID, "available", host)
	case compute.DataKindNeptune:
		// Keep CreateDBCluster DNS-style Address (*.neptune.noctaxris.internal); host is DinD-only.
		return s.store.SetNeptuneContainerID(accountID, name, containerID, "available", "")
	case compute.DataKindMQ:
		ep := fmt.Sprintf("amqp://%s:%d", host, store.MQNestedPort)
		return s.store.SetMQContainerID(accountID, name, containerID, store.MQBrokerStateRunning, ep)
	case compute.DataKindOpenSearch:
		ep := fmt.Sprintf("%s:%d", host, store.OpenSearchNestedPort)
		return s.store.SetOpenSearchContainerID(accountID, name, containerID, store.OpenSearchDomainStatusActive, ep, "")
	case compute.DataKindMSK:
		ep := fmt.Sprintf("%s:%d", host, store.MSKNestedPort)
		return s.store.SetMSKContainerID(accountID, name, containerID, store.MSKClusterStateActive, ep)
	default:
		return nil
	}
}

func markNestedDataFailed(s *Server, accountID string, dk compute.DataKind, name, evidence string) error {
	switch dk {
	case compute.DataKindElastiCache:
		return s.store.SetElastiCacheContainerID(accountID, name, "", "failed", "")
	case compute.DataKindMemoryDB:
		return s.store.SetMemoryDBContainerID(accountID, name, "", "failed", "")
	case compute.DataKindDocDB:
		return s.store.SetDocDBContainerID(accountID, name, "", "failed", "")
	case compute.DataKindNeptune:
		return s.store.SetNeptuneContainerID(accountID, name, "", "failed", "")
	case compute.DataKindMQ:
		stub := fmt.Sprintf("stub://127.0.0.1/mq/%s", strings.TrimSpace(name))
		return s.store.SetMQContainerID(accountID, name, "", store.MQBrokerStateCreationFailed, stub)
	case compute.DataKindOpenSearch:
		reason := ossvc.ClassifyOpenSearchNestedFailure(evidence)
		return s.store.SetOpenSearchContainerID(accountID, name, "", store.OpenSearchDomainStatusCreateFailed, "", reason)
	case compute.DataKindMSK:
		return s.store.SetMSKContainerID(accountID, name, "", store.MSKClusterStateFailed, "")
	default:
		return nil
	}
}

// SetStopDataPlaneHookForTest records nested StopDataPlane container IDs when set (unit tests).
func SetStopDataPlaneHookForTest(hook func(containerID string)) {
	stopDataPlaneHook = hook
}

func tryStopNestedDataEngine(s *Server, containerID string) error {
	if stopDataPlaneHook != nil {
		stopDataPlaneHook(containerID)
		return nil
	}
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
