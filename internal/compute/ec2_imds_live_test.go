package compute_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

func TestEnsureEC2IMDSMirrorLiveSkipsWithoutEngine(t *testing.T) {
	host := strings.TrimSpace(os.Getenv("NOCTAXRIS_DOCKER_HOST"))
	if host == "" {
		t.Skip("NOCTAXRIS_DOCKER_HOST unset; skip ec2 imds live test")
	}
	certPath := strings.TrimSpace(os.Getenv("NOCTAXRIS_DOCKER_CERT_PATH"))
	cli, err := compute.NewClient(host, certPath, "127.0.0.1:4566")
	if err != nil {
		t.Skipf("compute client unavailable: %v", err)
	}
	defer cli.Close()
	ctx := context.Background()

	started, err := cli.StartEC2Instance(ctx, compute.EC2RunOpts{
		ImageURI:   "public.ecr.aws/docker/library/alpine:3.20",
		InstanceID: "i-labuserdata01",
		AMIID:      "ami-alpine",
		UserData:   "#!/bin/sh\ntouch /tmp/noctaxris-userdata-ok\n",
		IamInstanceProfile: "LabEC2Role",
	})
	if err != nil {
		t.Fatalf("StartEC2Instance: %v", err)
	}
	t.Cleanup(func() {
		_ = cli.TerminateEC2Instance(context.Background(), started.ContainerID)
	})
	if started.PrivateIP == "" {
		t.Fatal("expected private IP on noctaxris-ec2")
	}
	if started.UserDataErr != nil {
		t.Fatalf("UserData: %v", started.UserDataErr)
	}
	if started.IMDSErr != nil {
		t.Fatalf("IMDS: %v", started.IMDSErr)
	}

	check, err := cli.Exec(ctx, compute.ExecOpts{
		ContainerID: started.ContainerID,
		Cmd: []string{"/bin/sh", "-c",
			"test -f /tmp/noctaxris-userdata-ok && " +
				"wget -qO- \"$AWS_EC2_METADATA_SERVICE_ENDPOINT/latest/meta-data/instance-id\"",
		},
	})
	if err != nil {
		t.Fatalf("exec check: %v", err)
	}
	if check.ExitCode != 0 {
		t.Fatalf("check exit %d stderr=%s stdout=%s", check.ExitCode, check.Stderr, check.Stdout)
	}
	if !strings.Contains(check.Stdout, "i-labuserdata01") {
		t.Fatalf("imds instance-id=%q", check.Stdout)
	}
}
