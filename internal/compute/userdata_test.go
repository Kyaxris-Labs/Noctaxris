package compute

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestDecodeUserDataPlainScript(t *testing.T) {
	script := "#!/bin/sh\necho hi\n"
	if got := DecodeUserData(script); got != script {
		t.Fatalf("got %q", got)
	}
}

func TestDecodeUserDataBase64(t *testing.T) {
	script := "#!/bin/sh\necho lab\n"
	enc := base64.StdEncoding.EncodeToString([]byte(script))
	if got := DecodeUserData(enc); got != script {
		t.Fatalf("got %q want %q", got, script)
	}
}

func TestDecodeUserDataEmpty(t *testing.T) {
	if got := DecodeUserData("  "); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestDecodeUserDataPlainWithoutShebang(t *testing.T) {
	raw := "echo not-base64-and-multiline\nok\n"
	if got := DecodeUserData(raw); got != raw {
		t.Fatalf("got %q", got)
	}
}

func TestEC2IMDSEndpointEnv(t *testing.T) {
	env := EC2IMDSEndpointEnv()
	want := "http://169.254.169.254:9255"
	if env["AWS_EC2_METADATA_SERVICE_ENDPOINT"] != want {
		t.Fatalf("endpoint=%q want %q", env["AWS_EC2_METADATA_SERVICE_ENDPOINT"], want)
	}
}

func TestEC2TaskHostConfigMergesIMDSExtraHosts(t *testing.T) {
	t.Setenv(EnvInjectECSHostGateway, "")
	hc := ec2TaskHostConfig(128, []string{EC2IMDSLinkLocal + ":10.0.0.8"})
	if len(hc.ExtraHosts) != 1 || hc.ExtraHosts[0] != "169.254.169.254:10.0.0.8" {
		t.Fatalf("ExtraHosts=%#v", hc.ExtraHosts)
	}
	if hc.PortBindings != nil {
		t.Fatalf("PortBindings must be nil (no host publish)")
	}
	t.Setenv(EnvInjectECSHostGateway, "1")
	hc = ec2TaskHostConfig(128, []string{EC2IMDSLinkLocal + ":10.0.0.8"})
	if len(hc.ExtraHosts) != 2 {
		t.Fatalf("ExtraHosts=%#v want host-gateway + imds", hc.ExtraHosts)
	}
}

func TestAttachEC2IMDSHostsUnavailableClient(t *testing.T) {
	c := &Client{}
	_, _, err := c.attachEC2IMDSHosts(context.Background())
	if err == nil {
		t.Fatal("expected error without docker client")
	}
}
