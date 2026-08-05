package compute

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/moby/moby/client"
)

// CodeBuildWorkspaceDir is the nested CodeBuild source root inside ECS task containers.
const CodeBuildWorkspaceDir = "/codebuild/src"

// ValidateCodeBuildContainerPath ensures container archive paths stay under the CodeBuild lab root.
func ValidateCodeBuildContainerPath(containerPath string) error {
	p := path.Clean(strings.TrimSpace(containerPath))
	if p == "." || p == "" {
		return fmt.Errorf("compute: container path is required")
	}
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("compute: container path must be absolute")
	}
	root := CodeBuildWorkspaceDir
	if p != root && !strings.HasPrefix(p, root+"/") {
		return fmt.Errorf("compute: container path %q outside codebuild workspace", p)
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("compute: container path traversal rejected")
	}
	return nil
}

// dirTreeTarForAbsolute builds a POSIX tar of directory entries for absPath and its parents
// (paths relative to /, e.g. codebuild/ and codebuild/src/). Used so Docker CopyToContainer
// can create missing dest dirs before extracting source files.
func dirTreeTarForAbsolute(absPath string) ([]byte, error) {
	p := path.Clean(strings.TrimSpace(absPath))
	if p == "/" || p == "." || p == "" {
		return nil, nil
	}
	rel := strings.TrimPrefix(p, "/")
	if rel == "" || rel == p {
		return nil, fmt.Errorf("compute: absolute container path required")
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	accum := ""
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" {
			continue
		}
		if accum == "" {
			accum = seg
		} else {
			accum = accum + "/" + seg
		}
		if err := tw.WriteHeader(&tar.Header{
			Name:     accum + "/",
			Typeflag: tar.TypeDir,
			Mode:     0o755,
		}); err != nil {
			_ = tw.Close()
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CopyToContainer extracts tarStream into destPath inside a nested container (create or running).
// destPath must be under CodeBuildWorkspaceDir. tarStream must be a POSIX tar archive.
// Missing dest directories are created first (Docker requires the copy destination to exist).
func (c *Client) CopyToContainer(ctx context.Context, containerID, destPath string, tarStream io.Reader) error {
	if c == nil || c.cli == nil {
		return fmt.Errorf("compute: copy client unavailable")
	}
	cid := strings.TrimSpace(containerID)
	if cid == "" {
		return fmt.Errorf("compute: container ID is required")
	}
	if err := ValidateCodeBuildContainerPath(destPath); err != nil {
		return err
	}
	if tarStream == nil {
		return fmt.Errorf("compute: tar stream is required")
	}
	dst := path.Clean(destPath)
	mkdirTar, err := dirTreeTarForAbsolute(dst)
	if err != nil {
		return err
	}
	if len(mkdirTar) > 0 {
		if _, err := c.cli.CopyToContainer(ctx, cid, client.CopyToContainerOptions{
			DestinationPath: "/",
			Content:         bytes.NewReader(mkdirTar),
		}); err != nil {
			return fmt.Errorf("compute: ensure container dirs %s: %w", dst, err)
		}
	}
	if _, err := c.cli.CopyToContainer(ctx, cid, client.CopyToContainerOptions{
		DestinationPath: dst,
		Content:         tarStream,
	}); err != nil {
		return fmt.Errorf("compute: copy to container %s:%s: %w", cid, dst, err)
	}
	return nil
}

// CopyFromContainer returns a tar archive stream for srcPath inside a nested container.
// srcPath must be under CodeBuildWorkspaceDir. The caller must Close the ReadCloser.
func (c *Client) CopyFromContainer(ctx context.Context, containerID, srcPath string) (io.ReadCloser, error) {
	if c == nil || c.cli == nil {
		return nil, fmt.Errorf("compute: copy client unavailable")
	}
	cid := strings.TrimSpace(containerID)
	if cid == "" {
		return nil, fmt.Errorf("compute: container ID is required")
	}
	if err := ValidateCodeBuildContainerPath(srcPath); err != nil {
		return nil, err
	}
	src := path.Clean(srcPath)
	res, err := c.cli.CopyFromContainer(ctx, cid, client.CopyFromContainerOptions{SourcePath: src})
	if err != nil {
		return nil, fmt.Errorf("compute: copy from container %s:%s: %w", cid, src, err)
	}
	return res.Content, nil
}

// WorkspaceTarHasFileEntries reports whether tarData contains at least one non-root path entry.
func WorkspaceTarHasFileEntries(tarData []byte) bool {
	if len(tarData) == 0 {
		return false
	}
	tr := tar.NewReader(bytes.NewReader(tarData))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return false
		}
		if err != nil {
			return false
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		name = path.Clean(name)
		if name != "" && name != "." {
			return true
		}
	}
}
