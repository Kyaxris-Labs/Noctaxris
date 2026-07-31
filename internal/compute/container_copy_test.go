package compute_test

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

func TestValidateCodeBuildContainerPath(t *testing.T) {
	ok := []string{
		compute.CodeBuildWorkspaceDir,
		compute.CodeBuildWorkspaceDir + "/README.md",
		compute.CodeBuildWorkspaceDir + "/nested/dir",
	}
	for _, p := range ok {
		if err := compute.ValidateCodeBuildContainerPath(p); err != nil {
			t.Fatalf("path %q: %v", p, err)
		}
	}
	bad := []string{
		"",
		"/var/task",
		"/codebuild",
		"/codebuild/src/../etc/passwd",
		"relative/path",
	}
	for _, p := range bad {
		if err := compute.ValidateCodeBuildContainerPath(p); err == nil {
			t.Fatalf("expected error for %q", p)
		}
	}
}

func TestWorkspaceTarHasFileEntries(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "README.md", Size: 3, Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hi\n")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if !compute.WorkspaceTarHasFileEntries(buf.Bytes()) {
		t.Fatal("expected file entries")
	}
	if compute.WorkspaceTarHasFileEntries(nil) {
		t.Fatal("empty tar should be false")
	}
}

func TestCopyToFromContainerLive(t *testing.T) {
	host := strings.TrimSpace(os.Getenv("NOCTAXRIS_DOCKER_HOST"))
	if host == "" {
		t.Skip("NOCTAXRIS_DOCKER_HOST unset; skip nested copy live test")
	}
	certPath := strings.TrimSpace(os.Getenv("NOCTAXRIS_DOCKER_CERT_PATH"))
	cli, err := compute.NewClient(host, certPath, "127.0.0.1:4566")
	if err != nil {
		t.Skipf("compute client unavailable: %v", err)
	}
	defer cli.Close()
	ctx := context.Background()
	if err := cli.Ping(ctx); err != nil {
		t.Skipf("nested engine ping: %v", err)
	}

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	if err := tw.WriteHeader(&tar.Header{Name: "probe.txt", Size: 5, Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("probe")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	cid, err := cli.RunECSTask(ctx, compute.ECSRunOpts{
		ImageURI:         "alpine:3.20",
		Command:          []string{"/bin/sh", "-c", "sleep 30"},
		EndpointURL:      "http://host.docker.internal:4566",
		PreStartCopyDest: compute.CodeBuildWorkspaceDir,
		PreStartCopyTar:  bytes.NewReader(tarBuf.Bytes()),
	})
	if err != nil {
		t.Fatalf("RunECSTask: %v", err)
	}
	defer func() { _ = cli.StopECSTask(context.Background(), cid) }()

	rc, err := cli.CopyFromContainer(ctx, cid, compute.CodeBuildWorkspaceDir)
	if err != nil {
		t.Fatalf("CopyFromContainer: %v", err)
	}
	defer rc.Close()
	out, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !compute.WorkspaceTarHasFileEntries(out) {
		t.Fatalf("harvest tar empty: %d bytes", len(out))
	}
	if !bytes.Contains(out, []byte("probe")) {
		t.Fatalf("expected probe.txt in tar (%d bytes)", len(out))
	}
}
