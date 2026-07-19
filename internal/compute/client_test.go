package compute_test

import (
	"context"
	"os"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

func TestNewClientEmptyHost(t *testing.T) {
	_, err := compute.NewClient("")
	if err == nil {
		t.Fatal("expected error for empty docker host")
	}
}

func TestValidateRunOpts(t *testing.T) {
	t.Run("rejects empty CodeHostPath", func(t *testing.T) {
		err := compute.ValidateRunOpts(compute.RunOpts{
			Handler: "main.handler",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rejects bad Handler", func(t *testing.T) {
		err := compute.ValidateRunOpts(compute.RunOpts{
			CodeHostPath: "/var/lib/noctaxris/fn",
			Handler:      "nope",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rejects relative CodeHostPath", func(t *testing.T) {
		err := compute.ValidateRunOpts(compute.RunOpts{
			CodeHostPath: "relative/path",
			Handler:      "main.handler",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("accepts minimal opts", func(t *testing.T) {
		err := compute.ValidateRunOpts(compute.RunOpts{
			CodeHostPath: "/var/lib/noctaxris/fn",
			Handler:      "main.handler",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestPingSkipsWithoutEngine(t *testing.T) {
	host := os.Getenv("NOCTAXRIS_DOCKER_HOST")
	if host == "" {
		t.Skip("NOCTAXRIS_DOCKER_HOST unset")
	}
	cli, err := compute.NewClient(host)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	if err := cli.Ping(context.Background()); err != nil {
		t.Skipf("engine not reachable: %v", err)
	}
}
