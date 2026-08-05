package compute

import (
	"context"
	"fmt"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

const (
	// EC2NetworkName is the DinD-internal network for lab EC2 instance containers.
	EC2NetworkName = "noctaxris-ec2"

	// LabelEC2 marks nested EC2 lab containers.
	LabelEC2 = "noctaxris.ec2"
)

// EC2 keep-alive command so any base image stays running regardless of default CMD.
var ec2KeepAliveCmd = []string{"tail", "-f", "/dev/null"}

// EC2RunOpts configures a lab EC2 instance container inside DinD.
type EC2RunOpts struct {
	ImageURI    string
	Env         map[string]string
	EndpointURL string
	// ListenAddr pins DinD lab registry pulls.
	ListenAddr string
	MemoryMB   int
	// LabRegistryPull requests an authenticated pull via PullLabRegistryImage.
	LabRegistryPull  bool
	RegistryUsername string
	RegistryPassword string
	// InstanceID / AMIID / LocalIPv4Hint feed IMDS lite registration after start.
	InstanceID string
	AMIID      string
	// IamInstanceProfile is the lab role name exposed under iam/security-credentials/ when set.
	IamInstanceProfile string
	// UserData is raw or base64 UserData; executed once via sh after start (best-effort).
	UserData string
}

// EC2StartResult is returned by StartEC2Instance.
type EC2StartResult struct {
	ContainerID string
	PrivateIP   string
	// UserDataErr is set when UserData ran but failed; the instance still started.
	UserDataErr error
	// IMDSErr is set when IMDS attach/register failed; the instance still started.
	IMDSErr error
}

// ValidateEC2RunOpts checks required fields without talking to Docker.
func ValidateEC2RunOpts(opts EC2RunOpts) error {
	if strings.TrimSpace(opts.ImageURI) == "" {
		return fmt.Errorf("compute: ImageURI is required")
	}
	if err := AllowImagePull(opts.ImageURI, opts.ListenAddr); err != nil {
		return err
	}
	return RequireLabRegistryCreds(opts.LabRegistryPull, LabRegistryPullCreds{
		Username: opts.RegistryUsername,
		Password: opts.RegistryPassword,
	})
}

// EnsureEC2Network creates (or reuses) an Internal Docker network for EC2 instances.
func (c *Client) EnsureEC2Network(ctx context.Context) (string, error) {
	return c.ensureInternalNetwork(ctx, EC2NetworkName)
}

func ec2TaskHostConfig(memoryMB int, extraHosts []string) *container.HostConfig {
	sec := nestedTaskSecurity(memoryMB)
	hosts := ecsHostGatewayExtraHosts()
	if len(extraHosts) > 0 {
		hosts = append(append([]string{}, hosts...), extraHosts...)
	}
	return &container.HostConfig{
		AutoRemove:  false,
		NetworkMode: container.NetworkMode(EC2NetworkName),
		ExtraHosts:  hosts,
		Privileged:  false,
		CapDrop:     append([]string(nil), sec.CapDrop...),
		SecurityOpt: append([]string(nil), sec.SecurityOpt...),
		Resources:   nestedTaskResources(sec),
	}
}

// StartEC2Instance creates and starts a keep-alive EC2 lab container.
// UserData and IMDS failures are recorded on the result and do not fail the start.
func (c *Client) StartEC2Instance(ctx context.Context, opts EC2RunOpts) (EC2StartResult, error) {
	if err := ValidateEC2RunOpts(opts); err != nil {
		return EC2StartResult{}, err
	}
	endpoint := strings.TrimSpace(opts.EndpointURL)
	if endpoint == "" {
		endpoint = defaultEndpointURL
	}
	if _, err := c.EnsureEC2Network(ctx); err != nil {
		return EC2StartResult{}, err
	}
	if opts.LabRegistryPull {
		if err := c.PullLabRegistryImage(ctx, opts.ImageURI, opts.RegistryUsername, opts.RegistryPassword); err != nil {
			return EC2StartResult{}, fmt.Errorf("compute: pull lab registry image %s: %w", opts.ImageURI, err)
		}
	} else if err := c.pullImage(ctx, opts.ImageURI); err != nil {
		return EC2StartResult{}, fmt.Errorf("compute: pull image %s: %w", opts.ImageURI, err)
	}

	var imdsErr error
	imdsHosts, imdsEnv, err := c.attachEC2IMDSHosts(ctx)
	if err != nil {
		imdsErr = err
		imdsHosts, imdsEnv = nil, nil
	}

	env := []string{
		"AWS_DEFAULT_REGION=us-east-1",
		"AWS_ENDPOINT_URL=" + endpoint,
		"AWS_ENDPOINT_URL_EC2=" + endpoint,
		"AWS_ENDPOINT_URL_S3=" + endpoint,
		"AWS_ENDPOINT_URL_STS=" + endpoint,
		"AWS_ENDPOINT_URL_IAM=" + endpoint,
	}
	for k, v := range imdsEnv {
		env = append(env, k+"="+v)
	}
	for k, v := range opts.Env {
		if k == "" {
			continue
		}
		env = append(env, k+"="+v)
	}

	name := "noctaxris-ec2-" + uuid.NewString()
	cfg := &container.Config{
		Image: opts.ImageURI,
		Env:   env,
		Cmd:   append([]string(nil), ec2KeepAliveCmd...),
		Labels: map[string]string{
			LabelManaged: "1",
			LabelEC2:     "instance",
		},
	}
	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			EC2NetworkName: {},
		},
	}
	create, err := c.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           cfg,
		HostConfig:       ec2TaskHostConfig(opts.MemoryMB, imdsHosts),
		NetworkingConfig: netCfg,
		Name:             name,
	})
	if err != nil {
		return EC2StartResult{}, fmt.Errorf("compute: ec2 container create: %w", err)
	}
	cid := create.ID
	if _, err := c.cli.ContainerStart(ctx, cid, client.ContainerStartOptions{}); err != nil {
		_, _ = c.cli.ContainerRemove(context.Background(), cid, client.ContainerRemoveOptions{Force: true})
		return EC2StartResult{}, fmt.Errorf("compute: ec2 container start: %w", err)
	}

	result := EC2StartResult{ContainerID: cid, IMDSErr: imdsErr}
	privIP, ipErr := c.ContainerNetworkIP(ctx, cid, EC2NetworkName)
	if ipErr == nil {
		result.PrivateIP = privIP
	}

	if imdsErr == nil && result.PrivateIP != "" {
		meta := EC2IMDSMeta{
			InstanceID: strings.TrimSpace(opts.InstanceID),
			LocalIPv4:  result.PrivateIP,
			AMIID:      strings.TrimSpace(opts.AMIID),
			IAMRole:    strings.TrimSpace(opts.IamInstanceProfile),
		}
		if meta.InstanceID == "" {
			meta.InstanceID = cid
		}
		if regErr := c.registerEC2IMDSMeta(ctx, result.PrivateIP, meta); regErr != nil {
			result.IMDSErr = regErr
		}
	}

	if script := DecodeUserData(opts.UserData); script != "" {
		if udErr := c.execEC2UserData(ctx, cid, script); udErr != nil {
			result.UserDataErr = udErr
		}
	}
	return result, nil
}

// execEC2UserData runs decoded UserData once via sh inside the instance container.
func (c *Client) execEC2UserData(ctx context.Context, containerID, script string) error {
	res, err := c.Exec(ctx, ExecOpts{
		ContainerID: containerID,
		Cmd: []string{
			"/bin/sh", "-c",
			`printf '%s' "$NOCTAXRIS_USERDATA" > /tmp/noctaxris-userdata.sh && /bin/sh /tmp/noctaxris-userdata.sh`,
		},
		Env: []string{"NOCTAXRIS_USERDATA=" + script},
	})
	if err != nil {
		return fmt.Errorf("userdata exec: %w", err)
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = fmt.Sprintf("exit %d", res.ExitCode)
		}
		return fmt.Errorf("userdata exit %d: %s", res.ExitCode, msg)
	}
	return nil
}

// StopEC2Instance stops a running EC2 lab container (kept for StartInstances).
func (c *Client) StopEC2Instance(ctx context.Context, containerID string) error {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("compute: container ID is required")
	}
	timeout := 30
	if _, err := c.cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("compute: ec2 container stop: %w", err)
	}
	return nil
}

// StartStoppedEC2Instance starts a previously stopped EC2 lab container.
func (c *Client) StartStoppedEC2Instance(ctx context.Context, containerID string) error {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("compute: container ID is required")
	}
	if _, err := c.cli.ContainerStart(ctx, containerID, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("compute: ec2 container start: %w", err)
	}
	return nil
}

// TerminateEC2Instance force-removes an EC2 lab container.
func (c *Client) TerminateEC2Instance(ctx context.Context, containerID string) error {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("compute: container ID is required")
	}
	if _, err := c.cli.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true}); err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("compute: ec2 container remove: %w", err)
	}
	return nil
}
