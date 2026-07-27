package compute_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

func TestValidateEC2RunOpts(t *testing.T) {
	t.Run("rejects empty ImageURI", func(t *testing.T) {
		err := compute.ValidateEC2RunOpts(compute.EC2RunOpts{})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("accepts alpine pin", func(t *testing.T) {
		err := compute.ValidateEC2RunOpts(compute.EC2RunOpts{
			ImageURI: "public.ecr.aws/docker/library/alpine:3.20",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts amazonlinux pin", func(t *testing.T) {
		err := compute.ValidateEC2RunOpts(compute.EC2RunOpts{
			ImageURI: "public.ecr.aws/amazonlinux/amazonlinux:2023",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts ubuntu pin", func(t *testing.T) {
		err := compute.ValidateEC2RunOpts(compute.EC2RunOpts{
			ImageURI: "public.ecr.aws/docker/library/ubuntu:22.04",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("rejects unknown image", func(t *testing.T) {
		err := compute.ValidateEC2RunOpts(compute.EC2RunOpts{ImageURI: "evil.example/pwn:latest"})
		if err == nil {
			t.Fatal("expected deny")
		}
	})
}
