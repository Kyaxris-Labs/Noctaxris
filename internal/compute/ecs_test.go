package compute_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

func TestValidateECSRunOpts(t *testing.T) {
	t.Run("rejects empty ImageURI", func(t *testing.T) {
		err := compute.ValidateECSRunOpts(compute.ECSRunOpts{})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("accepts minimal opts", func(t *testing.T) {
		err := compute.ValidateECSRunOpts(compute.ECSRunOpts{ImageURI: "alpine:3.20"})
		if err != nil {
			t.Fatal(err)
		}
	})
}
