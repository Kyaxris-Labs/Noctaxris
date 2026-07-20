package compute

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/image"
)

// LoadImageFromTar loads an image archive into the nested engine.
func (c *Client) LoadImageFromTar(ctx context.Context, tar io.Reader) error {
	if c == nil || c.cli == nil {
		return fmt.Errorf("compute: client is nil")
	}
	resp, err := c.cli.ImageLoad(ctx, tar)
	if err != nil {
		return fmt.Errorf("compute: image load: %w", err)
	}
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return fmt.Errorf("compute: image load response: %w", err)
	}
	return nil
}

// TagImage applies an additional tag to an image in the nested engine.
func (c *Client) TagImage(ctx context.Context, source, target string) error {
	if c == nil || c.cli == nil {
		return fmt.Errorf("compute: client is nil")
	}
	if err := c.cli.ImageTag(ctx, source, target); err != nil {
		return fmt.Errorf("compute: image tag %q -> %q: %w", source, target, err)
	}
	return nil
}

// PullLabRegistryImage pulls from the lab registry into the nested engine.
// ref should use a DinD-reachable host (for example host.docker.internal:4566/ACCOUNT/REPO:tag).
func (c *Client) PullLabRegistryImage(ctx context.Context, ref, username, password string) error {
	if c == nil || c.cli == nil {
		return fmt.Errorf("compute: client is nil")
	}
	if err := AllowImagePull(ref); err != nil {
		return err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	rc, err := c.cli.ImagePull(ctx, ref, image.PullOptions{RegistryAuth: auth})
	if err != nil {
		return fmt.Errorf("compute: pull %s: %w", ref, err)
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("compute: pull %s drain: %w", ref, err)
	}
	return nil
}
