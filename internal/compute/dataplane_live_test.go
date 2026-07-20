package compute

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Live nested data-plane smoke. Skips cleanly when DinD is not configured.
// Set NOCTAXRIS_DOCKER_HOST (and TLS cert path when required) to run against noctaxris-engine.
func TestStartDataPlaneLiveSkipsWithoutEngine(t *testing.T) {
	host := strings.TrimSpace(os.Getenv("NOCTAXRIS_DOCKER_HOST"))
	if host == "" {
		t.Skip("NOCTAXRIS_DOCKER_HOST unset; skip nested data-plane live test")
	}
	certPath := strings.TrimSpace(os.Getenv("NOCTAXRIS_DOCKER_CERT_PATH"))
	cli, err := NewClient(host, certPath)
	if err != nil {
		t.Skipf("compute client unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	inst, err := cli.StartDataPlane(ctx, DataPlaneOpts{
		Kind:  DataKindElastiCache,
		Image: DefaultDataPlaneImage(DataKindElastiCache),
		Name:  "noctaxris-live-ec-test",
	})
	if err != nil {
		t.Fatalf("StartDataPlane: %v", err)
	}
	defer func() { _ = cli.StopDataPlane(context.Background(), inst.ContainerID) }()
	if inst.Endpoint == "" {
		t.Fatal("expected nested-network endpoint")
	}
	if strings.Contains(inst.Endpoint, "0.0.0.0") || strings.Contains(inst.Endpoint, "127.0.0.1") {
		t.Fatalf("endpoint looks host-published: %q", inst.Endpoint)
	}
	_ = cli.WaitDataPlaneHealthy(ctx, inst.ContainerID)
}
