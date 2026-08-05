package compute

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

func TestDirTreeTarBoundaries(t *testing.T) {
	raw, err := dirTreeTarForAbsolute("/")
	if err != nil || raw != nil {
		t.Fatalf("root: raw=%v err=%v", raw, err)
	}
	raw, err = dirTreeTarForAbsolute("  ")
	if err != nil || raw != nil {
		t.Fatalf("blank: raw=%v err=%v", raw, err)
	}
	if _, err := dirTreeTarForAbsolute("relative"); err == nil {
		t.Fatal("relative path must fail")
	}
	raw, err = dirTreeTarForAbsolute("/a//b/")
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(bytes.NewReader(raw))
	var names []string
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, hdr.Name)
	}
	if len(names) != 2 || names[0] != "a/" || names[1] != "a/b/" {
		t.Fatalf("names=%v", names)
	}
}

func TestValidateCodeBuildContainerPathTraversalCleaned(t *testing.T) {
	if err := ValidateCodeBuildContainerPath("/codebuild/src/../../etc/passwd"); err == nil {
		t.Fatal("expected reject")
	}
	if err := ValidateCodeBuildContainerPath("   "); err == nil {
		t.Fatal("whitespace")
	}
	if err := ValidateCodeBuildContainerPath("."); err == nil {
		t.Fatal("dot")
	}
}

func TestWorkspaceTarHasFileEntriesCorruptAndDot(t *testing.T) {
	if WorkspaceTarHasFileEntries([]byte("not-a-tar")) {
		t.Fatal("corrupt")
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if WorkspaceTarHasFileEntries(buf.Bytes()) {
		t.Fatal("root-only tar")
	}
}

func TestZipRuntimeEnvAndCloneMap(t *testing.T) {
	py := zipRuntimeEnv(store.LambdaRuntimePython312)
	if len(py) != 1 || py[0] != "PYTHONPATH=/var/task" {
		t.Fatalf("py=%v", py)
	}
	node := zipRuntimeEnv(store.LambdaRuntimeNodejs24x)
	if len(node) != 1 || !strings.Contains(node[0], "NODE_PATH=") {
		t.Fatalf("node=%v", node)
	}
	java := zipRuntimeEnv(store.LambdaRuntimeJava21)
	if len(java) != 1 || java[0] != "LAMBDA_TASK_ROOT=/var/task" {
		t.Fatalf("java=%v", java)
	}
	if zipRuntimeEnv("ruby3.2") != nil {
		t.Fatal("unknown runtime")
	}
	src := map[string]string{"a": "1"}
	cloned := cloneStringMap(src)
	cloned["a"] = "x"
	if src["a"] != "1" {
		t.Fatal("clone must copy")
	}
	if cloneStringMap(nil) == nil {
		t.Fatal("nil in still allocates")
	}
}

func TestResolveImageOneShotEchoBranches(t *testing.T) {
	node, ok := resolveImageOneShot("node:24-slim", "")
	if !ok || node.Exe != "node" || !strings.Contains(node.Script, "ok") {
		t.Fatalf("node echo=%+v ok=%v", node, ok)
	}
	java, ok := resolveImageOneShot("eclipse-temurin:21-jdk", "")
	if !ok || java.Exe != "/bin/sh" || !strings.Contains(java.Script, "ok") {
		t.Fatalf("java echo=%+v ok=%v", java, ok)
	}
	_, ok = resolveImageOneShot("alpine:3.20", "")
	if ok {
		t.Fatal("generic image")
	}
	if _, err := zipOneShotCommand("dotnet8"); err == nil {
		t.Fatal("unsupported zip runtime")
	}
}

func TestHostConfigSecurityOKNegatives(t *testing.T) {
	if hostConfigSecurityOK(nil) {
		t.Fatal("nil")
	}
	if hostConfigSecurityOK(&container.HostConfig{Privileged: true, CapDrop: []string{"ALL"}}) {
		t.Fatal("privileged")
	}
	if hostConfigSecurityOK(&container.HostConfig{CapAdd: []string{"NET_ADMIN"}, CapDrop: []string{"ALL"}}) {
		t.Fatal("capadd")
	}
	if hostConfigSecurityOK(&container.HostConfig{CapDrop: []string{"ALL", "KILL"}}) {
		t.Fatal("extra drop")
	}
	if hostConfigSecurityOK(&container.HostConfig{CapDrop: []string{"NET_RAW"}}) {
		t.Fatal("wrong drop")
	}
	if !hostConfigSecurityOK(&container.HostConfig{CapDrop: []string{"ALL"}}) {
		t.Fatal("ok")
	}
}

func TestFunctionNetworkPostureExtraCases(t *testing.T) {
	if !functionNetworkPostureOK(network.Inspect{
		Internal: false,
		Driver:   "BRIDGE",
		Options:  map[string]string{bridgeNoMasquerade: "0"},
	}) {
		t.Fatal("masq 0")
	}
	if functionNetworkPostureOK(network.Inspect{
		Internal: false,
		Driver:   "overlay",
		Options:  map[string]string{bridgeNoMasquerade: "false"},
	}) {
		t.Fatal("overlay")
	}
}

func TestAllowImagePullSecurityEdges(t *testing.T) {
	if err := AllowImagePull("", ""); err == nil {
		t.Fatal("empty")
	}
	if err := AllowImagePull(store.LabRegistryHost+"/not12digits/repo:tag", ""); err == nil {
		t.Fatal("bad account")
	}
	if err := AllowImagePull(store.LabRegistryHost+"/000000000001/", ""); err == nil {
		t.Fatal("empty repo")
	}
	if err := AllowImagePull(store.LabRegistryHost+"/000000000001/a/b:tag", ""); err == nil {
		t.Fatal("nested repo")
	}
	if err := AllowImagePull(store.LabRegistryHost+"/000000000001/../evil:tag", ""); err == nil {
		t.Fatal("traversal")
	}
	if err := AllowImagePull("public.ecr.aws/lambda/", ""); err == nil {
		t.Fatal("empty lambda path")
	}
	t.Setenv(EnvImagePullAllowlist, " , myprefix, localhost:5000/")
	if err := AllowImagePull("myprefix/tool:1", ""); err != nil {
		t.Fatalf("local prefix without digest: %v", err)
	}
	if err := AllowImagePull("localhost:5000/tool:1", ""); err == nil {
		t.Fatal("host prefix needs digest")
	}
	if err := AllowImagePull("localhost:5000/tool@sha256:"+strings.Repeat("b", 64), ""); err != nil {
		t.Fatal(err)
	}
}

func TestNestedDataEndpointAndRedpandaBoundaries(t *testing.T) {
	if got := NestedDataEndpoint("  ", 0); got != "unknown:5432" {
		t.Fatalf("got=%q", got)
	}
	if DefaultDataPlaneImage("") != "" || DefaultDataPlanePort("") != 0 {
		t.Fatal("empty kind defaults")
	}
	cmd := RedpandaStartCmd("  ")
	if !strings.Contains(strings.Join(cmd, " "), "noctaxris-msk:9092") {
		t.Fatalf("default advertise: %v", cmd)
	}
	mqtt := MosquittoStartCmd(" /custom.conf ")
	if mqtt[2] != "/custom.conf" {
		t.Fatalf("mqtt=%v", mqtt)
	}
}

func TestMergeLayerDirsEdges(t *testing.T) {
	if err := MergeLayerDirs(nil, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dest := t.TempDir()
	filePath := filepath.Join(root, "x.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MergeLayerDirs([]string{"", filePath}, dest); err == nil {
		t.Fatal("file layer must fail")
	}
	if err := MergeLayerDirs([]string{filepath.Join(root, "missing")}, dest); err == nil {
		t.Fatal("missing layer")
	}
}

func TestRegisterIMDSValidation(t *testing.T) {
	eng := newFakeEngine()
	cli, _ := newTestComputeClient(t, eng)
	ctx := t.Context()
	if err := cli.registerECSIMDSCredentials(ctx, "", []byte("{}")); err == nil {
		t.Fatal("empty cred id")
	}
	if err := cli.registerECSIMDSCredentials(ctx, "id", nil); err == nil {
		t.Fatal("empty body")
	}
	if err := cli.registerEC2IMDSMeta(ctx, "", EC2IMDSMeta{}); err == nil {
		t.Fatal("empty ip")
	}
	if _, err := cli.ContainerNetworkIP(ctx, "", EC2NetworkName); err == nil {
		t.Fatal("empty cid")
	}
}
