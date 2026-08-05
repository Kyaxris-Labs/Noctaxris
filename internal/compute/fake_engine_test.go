package compute

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// fakeEngine is a minimal Docker Engine HTTP API for unit tests (no DinD).
type fakeEngine struct {
	mu         sync.Mutex
	networks   map[string]map[string]any // id -> inspect
	netByName  map[string]string
	containers map[string]map[string]any // id or name -> inspect
	execs      map[string]execRec
	seq        atomic.Uint64
	execStdout string
	execCode   int
	logsStdout string
	pullFail   bool
}

type execRec struct {
	ID          string
	Container   string
	ExitCode    int
	Stdout      string
	Cmd         []string
	AttachStdin bool
	AttachDone  bool
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{
		networks:   map[string]map[string]any{},
		netByName:  map[string]string{},
		containers: map[string]map[string]any{},
		execs:      map[string]execRec{},
		execCode:   0,
		logsStdout: `{"ok":true}`,
		execStdout: "ok",
	}
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func stripAPIVersion(p string) string {
	if strings.HasPrefix(p, "/v") {
		if i := strings.Index(p[1:], "/"); i >= 0 {
			return p[1+i:]
		}
	}
	return p
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeDockerErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}

func muxStream(stream byte, payload []byte) []byte {
	hdr := make([]byte, 8)
	hdr[0] = stream
	binary.BigEndian.PutUint32(hdr[4:], uint32(len(payload)))
	return append(hdr, payload...)
}

func (f *fakeEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := stripAPIVersion(r.URL.Path)
	switch {
	case r.Method == http.MethodGet && (path == "/_ping" || path == "/version"):
		if path == "/_ping" {
			w.Header().Set("API-Version", "1.45")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ApiVersion": "1.45", "Version": "fake"})
		return
	case r.Method == http.MethodGet && path == "/networks":
		f.mu.Lock()
		defer f.mu.Unlock()
		out := make([]map[string]any, 0, len(f.networks))
		for id, n := range f.networks {
			out = append(out, map[string]any{"Id": id, "Name": n["Name"]})
		}
		writeJSON(w, http.StatusOK, out)
		return
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/networks/"):
		id := strings.TrimPrefix(path, "/networks/")
		f.mu.Lock()
		defer f.mu.Unlock()
		n, ok := f.networks[id]
		if !ok {
			if nid, ok2 := f.netByName[id]; ok2 {
				n, ok = f.networks[nid]
			}
		}
		if !ok {
			writeDockerErr(w, http.StatusNotFound, "network not found")
			return
		}
		writeJSON(w, http.StatusOK, n)
		return
	case r.Method == http.MethodPost && path == "/networks/create":
		var body struct {
			Name     string            `json:"Name"`
			Driver   string            `json:"Driver"`
			Internal bool              `json:"Internal"`
			Options  map[string]string `json:"Options"`
			Labels   map[string]string `json:"Labels"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := "net-" + itoa(f.seq.Add(1))
		insp := map[string]any{
			"Id":         id,
			"Name":       body.Name,
			"Driver":     body.Driver,
			"Internal":   body.Internal,
			"Options":    body.Options,
			"Labels":     body.Labels,
			"Containers": map[string]any{},
		}
		if insp["Options"] == nil {
			insp["Options"] = map[string]string{}
		}
		f.mu.Lock()
		f.networks[id] = insp
		f.netByName[body.Name] = id
		f.mu.Unlock()
		writeJSON(w, http.StatusCreated, map[string]any{"Id": id, "Warning": ""})
		return
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/networks/"):
		id := strings.TrimPrefix(path, "/networks/")
		f.mu.Lock()
		if n, ok := f.networks[id]; ok {
			if name, _ := n["Name"].(string); name != "" {
				delete(f.netByName, name)
			}
			delete(f.networks, id)
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	case r.Method == http.MethodPost && path == "/images/create":
		if f.pullFail {
			writeDockerErr(w, http.StatusInternalServerError, "pull failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"Pull complete"}` + "\n"))
		return
	case r.Method == http.MethodPost && path == "/images/load":
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"stream":"Loaded image" }` + "\n"))
		return
	case r.Method == http.MethodPost && strings.Contains(path, "/tag"):
		w.WriteHeader(http.StatusCreated)
		return
	case r.Method == http.MethodPost && path == "/containers/create":
		var body struct {
			Image  string            `json:"Image"`
			Labels map[string]string `json:"Labels"`
			Env    []string          `json:"Env"`
			Cmd    []string          `json:"Cmd"`
			HostConfig struct {
				NetworkMode string `json:"NetworkMode"`
			} `json:"HostConfig"`
			NetworkingConfig struct {
				EndpointsConfig map[string]any `json:"EndpointsConfig"`
			} `json:"NetworkingConfig"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		name := r.URL.Query().Get("name")
		id := "ctr-" + itoa(f.seq.Add(1))
		if name == "" {
			name = id
		}
		netName := body.HostConfig.NetworkMode
		if netName == "" {
			for n := range body.NetworkingConfig.EndpointsConfig {
				netName = n
				break
			}
		}
		if netName == "" {
			netName = FunctionNetworkName
			if strings.Contains(name, "data") || (body.Labels != nil && body.Labels[LabelDataKind] != "") {
				netName = DataPlaneNetworkName
			}
			if strings.Contains(name, "ecs") || name == ECSIMDSContainerName {
				netName = ECSNetworkName
			}
			if strings.Contains(name, "ec2") || name == EC2IMDSContainerName {
				netName = EC2NetworkName
			}
		}
		insp := map[string]any{
			"Id":   id,
			"Name": "/" + name,
			"State": map[string]any{
				"Running":  false,
				"Status":   "created",
				"ExitCode": 0,
				"Error":    "",
			},
			"Config": map[string]any{
				"Image":  body.Image,
				"Labels": body.Labels,
				"Env":    body.Env,
				"Cmd":    body.Cmd,
			},
			"NetworkSettings": map[string]any{
				"Networks": map[string]any{
					netName: map[string]any{"IPAddress": "10.0.0." + itoa(f.seq.Load()%200+2)},
				},
			},
		}
		f.mu.Lock()
		f.containers[id] = insp
		f.containers[name] = insp
		f.mu.Unlock()
		writeJSON(w, http.StatusCreated, map[string]any{"Id": id, "Warnings": []string{}})
		return
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/start"):
		id := containerIDFromPath(path, "/start")
		f.mu.Lock()
		if insp, ok := f.findContainerLocked(id); ok {
			state := insp["State"].(map[string]any)
			state["Running"] = true
			state["Status"] = "running"
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/stop"):
		id := containerIDFromPath(path, "/stop")
		f.mu.Lock()
		if insp, ok := f.findContainerLocked(id); ok {
			state := insp["State"].(map[string]any)
			state["Running"] = false
			state["Status"] = "exited"
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/containers/"):
		id := strings.TrimPrefix(path, "/containers/")
		if i := strings.Index(id, "/"); i >= 0 {
			id = id[:i]
		}
		f.mu.Lock()
		if insp, ok := f.findContainerLocked(id); ok {
			name := strings.TrimPrefix(insp["Name"].(string), "/")
			delete(f.containers, insp["Id"].(string))
			delete(f.containers, name)
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/json") && strings.HasPrefix(path, "/containers/"):
		id := containerIDFromPath(path, "/json")
		f.mu.Lock()
		insp, ok := f.findContainerLocked(id)
		f.mu.Unlock()
		if !ok {
			writeDockerErr(w, http.StatusNotFound, "No such container: "+id)
			return
		}
		writeJSON(w, http.StatusOK, insp)
		return
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/wait"):
		id := containerIDFromPath(path, "/wait")
		code := 0
		f.mu.Lock()
		if insp, ok := f.findContainerLocked(id); ok {
			state := insp["State"].(map[string]any)
			state["Running"] = false
			state["Status"] = "exited"
			switch v := state["ExitCode"].(type) {
			case int:
				code = v
			case float64:
				code = int(v)
			}
		}
		f.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"StatusCode": code})
		return
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/logs"):
		payload := muxStream(1, []byte(f.logsStdout+"\n"))
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
		return
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/exec") && strings.HasPrefix(path, "/containers/"):
		id := containerIDFromPath(path, "/exec")
		var body struct {
			Cmd         []string `json:"Cmd"`
			AttachStdin bool     `json:"AttachStdin"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		eid := "exec-" + itoa(f.seq.Add(1))
		f.mu.Lock()
		f.execs[eid] = execRec{
			ID:          eid,
			Container:   id,
			ExitCode:    f.execCode,
			Stdout:      f.execStdout,
			Cmd:         body.Cmd,
			AttachStdin: body.AttachStdin,
		}
		f.mu.Unlock()
		writeJSON(w, http.StatusCreated, map[string]string{"Id": eid})
		return
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/exec/") && strings.HasSuffix(path, "/start"):
		eid := strings.TrimSuffix(strings.TrimPrefix(path, "/exec/"), "/start")
		f.mu.Lock()
		rec, ok := f.execs[eid]
		f.mu.Unlock()
		if !ok {
			writeDockerErr(w, http.StatusNotFound, "No such exec")
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			writeDockerErr(w, http.StatusInternalServerError, "hijack unsupported")
			return
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = bufrw.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = bufrw.Flush()
		if rec.AttachStdin {
			// Block until client CloseWrite so stdin arrives before we emit stdout.
			_, _ = io.Copy(io.Discard, io.LimitReader(conn, 1<<20))
		}
		payload := muxStream(1, []byte(rec.Stdout))
		_, _ = conn.Write(payload)
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
		return
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/exec/") && strings.HasSuffix(path, "/json"):
		eid := strings.TrimSuffix(strings.TrimPrefix(path, "/exec/"), "/json")
		f.mu.Lock()
		rec, ok := f.execs[eid]
		f.mu.Unlock()
		if !ok {
			writeDockerErr(w, http.StatusNotFound, "No such exec")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ExitCode": rec.ExitCode, "Running": false})
		return
	case (r.Method == http.MethodPut || r.Method == http.MethodHead || r.Method == http.MethodGet) && strings.Contains(path, "/archive"):
		statJSON, _ := json.Marshal(container.PathStat{
			Name:  "probe.txt",
			Size:  5,
			Mode:  0o644,
			Mtime: time.Now().UTC(),
		})
		w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(statJSON))
		if r.Method == http.MethodPut {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/x-tar")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-tar"))
		return
	default:
		writeDockerErr(w, http.StatusNotFound, "fake engine: unhandled "+r.Method+" "+path)
	}
}

func containerIDFromPath(path, suffix string) string {
	path = strings.TrimSuffix(path, suffix)
	path = strings.TrimPrefix(path, "/containers/")
	return path
}

func (f *fakeEngine) findContainerLocked(idOrName string) (map[string]any, bool) {
	if insp, ok := f.containers[idOrName]; ok {
		return insp, true
	}
	return nil, false
}

func (f *fakeEngine) seedInternalNetwork(name string) string {
	id := "net-" + itoa(f.seq.Add(1))
	insp := map[string]any{
		"Id":         id,
		"Name":       name,
		"Driver":     "bridge",
		"Internal":   true,
		"Options":    map[string]string{},
		"Labels":     map[string]string{LabelManaged: "true"},
		"Containers": map[string]any{},
	}
	f.mu.Lock()
	f.networks[id] = insp
	f.netByName[name] = id
	f.mu.Unlock()
	return id
}

func (f *fakeEngine) seedFunctionNetwork(name string, okPosture bool, withContainers bool) string {
	id := "net-" + itoa(f.seq.Add(1))
	opts := map[string]string{}
	internal := false
	if okPosture {
		opts[bridgeNoMasquerade] = "false"
	} else {
		internal = true
	}
	containers := map[string]any{}
	if withContainers {
		containers["ctr-busy"] = map[string]any{"Name": "busy"}
	}
	insp := map[string]any{
		"Id":         id,
		"Name":       name,
		"Driver":     "bridge",
		"Internal":   internal,
		"Options":    opts,
		"Labels":     map[string]string{LabelManaged: "true"},
		"Containers": containers,
	}
	f.mu.Lock()
	f.networks[id] = insp
	f.netByName[name] = id
	f.mu.Unlock()
	return id
}

func (f *fakeEngine) seedRunningContainer(name, networkName, image string, labels map[string]string) string {
	id := "ctr-" + itoa(f.seq.Add(1))
	if labels == nil {
		labels = map[string]string{}
	}
	insp := map[string]any{
		"Id":   id,
		"Name": "/" + name,
		"State": map[string]any{
			"Running":  true,
			"Status":   "running",
			"ExitCode": 0,
			"Error":    "",
		},
		"Config": map[string]any{
			"Image":  image,
			"Labels": labels,
			"Env":    []string{},
			"Cmd":    []string{},
		},
		"NetworkSettings": map[string]any{
			"Networks": map[string]any{
				networkName: map[string]any{"IPAddress": "10.0.0.9"},
			},
		},
	}
	f.mu.Lock()
	f.containers[id] = insp
	f.containers[name] = insp
	f.mu.Unlock()
	return id
}

func newTestComputeClient(t *testing.T, eng *fakeEngine) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(eng)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	dockerCLI, err := client.New(
		client.WithHost("tcp://"+u.Host),
		client.WithHTTPClient(srv.Client()),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dockerCLI.Close() })
	return &Client{cli: dockerCLI, listenAddr: "127.0.0.1:4566"}, srv
}

func TestFakeEnginePingCloseAndNetworks(t *testing.T) {
	eng := newFakeEngine()
	cli, _ := newTestComputeClient(t, eng)
	ctx := context.Background()

	if err := cli.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if err := cli.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Close again via nil-safe path.
	var nilCLI *Client
	if err := nilCLI.Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}

	cli, _ = newTestComputeClient(t, eng)
	if _, err := cli.EnsureNetwork(ctx); err != nil {
		t.Fatalf("EnsureNetwork: %v", err)
	}
	// Reuse existing function network with good posture.
	if _, err := cli.EnsureNetwork(ctx); err != nil {
		t.Fatalf("EnsureNetwork reuse: %v", err)
	}
	if _, err := cli.EnsureECSNetwork(ctx); err != nil {
		t.Fatalf("EnsureECSNetwork: %v", err)
	}
	if _, err := cli.EnsureDataPlaneNetwork(ctx); err != nil {
		t.Fatalf("EnsureDataPlaneNetwork: %v", err)
	}
	if _, err := cli.EnsureEC2Network(ctx); err != nil {
		t.Fatalf("EnsureEC2Network: %v", err)
	}
}

func TestFakeEngineNetworkPolicyRefuse(t *testing.T) {
	eng := newFakeEngine()
	eng.seedInternalNetwork("legacy-open")
	// Force non-Internal for refuse path on ensureInternalNetwork.
	eng.mu.Lock()
	for id, n := range eng.networks {
		if n["Name"] == "legacy-open" {
			n["Internal"] = false
			eng.networks[id] = n
		}
	}
	eng.mu.Unlock()

	cli, _ := newTestComputeClient(t, eng)
	_, err := cli.ensureInternalNetwork(context.Background(), "legacy-open")
	if err == nil || !strings.Contains(err.Error(), "not Internal") {
		t.Fatalf("expected refuse non-Internal, got %v", err)
	}

	eng2 := newFakeEngine()
	eng2.seedFunctionNetwork(FunctionNetworkName, false, true)
	cli2, _ := newTestComputeClient(t, eng2)
	_, err = cli2.EnsureNetwork(context.Background())
	if err == nil || !strings.Contains(err.Error(), "active containers") {
		t.Fatalf("expected refuse busy incompatible function net, got %v", err)
	}

	eng3 := newFakeEngine()
	eng3.seedFunctionNetwork(FunctionNetworkName, false, false)
	cli3, _ := newTestComputeClient(t, eng3)
	if _, err := cli3.EnsureNetwork(context.Background()); err != nil {
		t.Fatalf("replace unused incompatible function net: %v", err)
	}
}

func TestFakeEngineNilClientGuards(t *testing.T) {
	ctx := context.Background()
	var c *Client
	if err := c.CopyToContainer(ctx, "x", CodeBuildWorkspaceDir, bytes.NewReader(nil)); err == nil {
		t.Fatal("CopyToContainer nil")
	}
	if _, err := c.CopyFromContainer(ctx, "x", CodeBuildWorkspaceDir); err == nil {
		t.Fatal("CopyFromContainer nil")
	}
	if err := c.LoadImageFromTar(ctx, bytes.NewReader(nil)); err == nil {
		t.Fatal("LoadImageFromTar nil")
	}
	if err := c.TagImage(ctx, "a", "b"); err == nil {
		t.Fatal("TagImage nil")
	}
	if err := c.PullLabRegistryImage(ctx, "alpine:3.20", "AWS", "tok"); err == nil {
		t.Fatal("PullLabRegistryImage nil")
	}
	if _, err := c.EnsureDataPlaneByName(ctx, DataPlaneOpts{Kind: DataKindRDS, Image: "postgres:16-alpine", Name: "x"}); err == nil {
		t.Fatal("EnsureDataPlaneByName nil")
	}
	if _, err := c.DataPlaneLogs(ctx, "x"); err == nil {
		t.Fatal("DataPlaneLogs nil")
	}
	if _, err := c.ContainerNetworkIP(ctx, "x", EC2NetworkName); err == nil {
		t.Fatal("ContainerNetworkIP nil")
	}
	if _, err := c.ensureECSIMDSMirror(ctx); err == nil {
		t.Fatal("ensureECSIMDSMirror nil")
	}
	if _, err := c.ensureEC2IMDSMirror(ctx); err == nil {
		t.Fatal("ensureEC2IMDSMirror nil")
	}
	empty := &Client{}
	if err := empty.CopyToContainer(ctx, "cid", CodeBuildWorkspaceDir, bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("CopyToContainer empty cli")
	}
	if err := empty.pullImage(ctx, "not-allowlisted.example/x:1"); err == nil {
		t.Fatal("pullImage deny")
	}
}

func TestFakeEngineDataPlaneLifecycle(t *testing.T) {
	eng := newFakeEngine()
	cli, _ := newTestComputeClient(t, eng)
	ctx := context.Background()

	inst, err := cli.StartDataPlane(ctx, DataPlaneOpts{
		Kind:  DataKindRDS,
		Image: "postgres:16-alpine",
		Env:   map[string]string{"": "skip", "POSTGRES_PASSWORD": "lab"},
		Labels: map[string]string{
			"":           "skip",
			LabelManaged: "attacker",
			"custom":     "ok",
		},
		Binds:      []string{"/tmp/a:/a:ro"},
		ExtraHosts: []string{"host.docker.internal:host-gateway"},
	})
	if err != nil {
		t.Fatalf("StartDataPlane: %v", err)
	}
	if !inst.Running || inst.ContainerID == "" || !strings.Contains(inst.Endpoint, ":") {
		t.Fatalf("inst=%+v", inst)
	}

	got, err := cli.InspectDataPlane(ctx, inst.ContainerID)
	if err != nil {
		t.Fatalf("InspectDataPlane: %v", err)
	}
	if got.Kind != DataKindRDS || !got.Running {
		t.Fatalf("inspect=%+v", got)
	}

	logs, err := cli.DataPlaneLogs(ctx, inst.ContainerID)
	if err != nil {
		t.Fatalf("DataPlaneLogs: %v", err)
	}
	if !strings.Contains(logs, "ok") {
		t.Fatalf("logs=%q", logs)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := cli.WaitDataPlaneHealthy(waitCtx, inst.ContainerID); err != nil {
		t.Fatalf("WaitDataPlaneHealthy: %v", err)
	}

	if err := cli.StopDataPlane(ctx, inst.ContainerID); err != nil {
		t.Fatalf("StopDataPlane: %v", err)
	}

	// Empty ID validation.
	if err := cli.StopDataPlane(ctx, "  "); err == nil {
		t.Fatal("StopDataPlane empty id")
	}
	if _, err := cli.InspectDataPlane(ctx, ""); err == nil {
		t.Fatal("InspectDataPlane empty id")
	}
	if err := cli.WaitDataPlaneHealthy(ctx, ""); err == nil {
		t.Fatal("WaitDataPlaneHealthy empty id")
	}
	if _, err := cli.DataPlaneLogs(ctx, ""); err == nil {
		t.Fatal("DataPlaneLogs empty id")
	}
}

func TestFakeEngineEnsureDataPlaneByName(t *testing.T) {
	eng := newFakeEngine()
	cli, _ := newTestComputeClient(t, eng)
	ctx := context.Background()

	_, err := cli.EnsureDataPlaneByName(ctx, DataPlaneOpts{
		Kind: DataKindMQTT, Image: "eclipse-mosquitto:2.0.20",
	})
	if err == nil || !strings.Contains(err.Error(), "Name is required") {
		t.Fatalf("name required: %v", err)
	}

	opts := DataPlaneOpts{
		Kind:  DataKindMQTT,
		Image: "eclipse-mosquitto:2.0.20",
		Name:  LabMQTTContainerName,
		Cmd:   MosquittoStartCmd("/etc/noctaxris/mqtt/mosquitto.conf"),
	}
	first, err := cli.EnsureDataPlaneByName(ctx, opts)
	if err != nil {
		t.Fatalf("ensure create: %v", err)
	}
	second, err := cli.EnsureDataPlaneByName(ctx, opts)
	if err != nil {
		t.Fatalf("ensure reuse: %v", err)
	}
	if first.ContainerID != second.ContainerID {
		t.Fatalf("reuse id %q vs %q", first.ContainerID, second.ContainerID)
	}

	// Binds change forces recreate.
	opts.Binds = []string{"/tmp/mqtt:/etc/noctaxris/mqtt:ro"}
	third, err := cli.EnsureDataPlaneByName(ctx, opts)
	if err != nil {
		t.Fatalf("ensure recreate: %v", err)
	}
	if third.ContainerID == first.ContainerID {
		t.Fatal("expected new container after binds change")
	}

	// Stopped named container should restart.
	eng.mu.Lock()
	insp := eng.containers[LabMQTTContainerName]
	state := insp["State"].(map[string]any)
	state["Running"] = false
	state["Status"] = "exited"
	eng.mu.Unlock()
	restarted, err := cli.EnsureDataPlaneByName(ctx, opts)
	if err != nil {
		t.Fatalf("ensure restart: %v", err)
	}
	if !restarted.Running {
		t.Fatal("expected running after restart")
	}
}

func TestFakeEngineWaitDataPlaneExited(t *testing.T) {
	eng := newFakeEngine()
	id := eng.seedRunningContainer("dead-data", DataPlaneNetworkName, "postgres:16-alpine", map[string]string{
		LabelDataKind: string(DataKindRDS),
	})
	eng.mu.Lock()
	state := eng.containers[id]["State"].(map[string]any)
	state["Running"] = false
	state["Status"] = "exited"
	state["ExitCode"] = 1
	state["Error"] = "bootstrap failed"
	eng.mu.Unlock()

	cli, _ := newTestComputeClient(t, eng)
	err := cli.WaitDataPlaneHealthy(context.Background(), id)
	if err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("err=%v", err)
	}
}

func TestFakeEngineECSTaskCopyAndWait(t *testing.T) {
	eng := newFakeEngine()
	cli, _ := newTestComputeClient(t, eng)
	ctx := context.Background()

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

	cid, err := cli.RunECSTask(ctx, ECSRunOpts{
		ImageURI:         "alpine:3.20",
		Command:          []string{"/bin/sh", "-c", "true"},
		Env:              map[string]string{"": "skip", "FOO": "bar"},
		PreStartCopyDest: CodeBuildWorkspaceDir,
		PreStartCopyTar:  bytes.NewReader(tarBuf.Bytes()),
	})
	if err != nil {
		t.Fatalf("RunECSTask: %v", err)
	}
	running, err := cli.ContainerRunning(ctx, cid)
	if err != nil || !running {
		t.Fatalf("ContainerRunning: running=%v err=%v", running, err)
	}
	code, err := cli.WaitECSTaskExit(ctx, cid)
	if err != nil || code != 0 {
		t.Fatalf("WaitECSTaskExit code=%d err=%v", code, err)
	}
	if err := cli.StopECSTask(ctx, cid); err != nil {
		t.Fatalf("StopECSTask: %v", err)
	}

	_, err = cli.RunECSTask(ctx, ECSRunOpts{
		ImageURI:         "alpine:3.20",
		PreStartCopyDest: CodeBuildWorkspaceDir,
	})
	if err == nil || !strings.Contains(err.Error(), "PreStartCopyDest") {
		t.Fatalf("dest without tar: %v", err)
	}

	if err := cli.StopECSTask(ctx, ""); err == nil {
		t.Fatal("StopECSTask empty")
	}
	if _, err := cli.WaitECSTaskExit(ctx, ""); err == nil {
		t.Fatal("WaitECSTaskExit empty")
	}
	if _, err := cli.ContainerRunning(ctx, ""); err == nil {
		t.Fatal("ContainerRunning empty")
	}
}

func TestFakeEngineCopyValidation(t *testing.T) {
	eng := newFakeEngine()
	cli, _ := newTestComputeClient(t, eng)
	ctx := context.Background()
	cid := eng.seedRunningContainer("copy-ctr", ECSNetworkName, "alpine:3.20", nil)

	if err := cli.CopyToContainer(ctx, "", CodeBuildWorkspaceDir, bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("empty cid")
	}
	if err := cli.CopyToContainer(ctx, cid, "/etc/passwd", bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("path escape")
	}
	if err := cli.CopyToContainer(ctx, cid, CodeBuildWorkspaceDir, nil); err == nil {
		t.Fatal("nil tar")
	}
	if err := cli.CopyToContainer(ctx, cid, CodeBuildWorkspaceDir, bytes.NewReader([]byte("x"))); err != nil {
		t.Fatalf("CopyToContainer: %v", err)
	}
	rc, err := cli.CopyFromContainer(ctx, cid, CodeBuildWorkspaceDir+"/probe.txt")
	if err != nil {
		t.Fatalf("CopyFromContainer: %v", err)
	}
	_ = rc.Close()
	if _, err := cli.CopyFromContainer(ctx, "", CodeBuildWorkspaceDir); err == nil {
		t.Fatal("empty cid from")
	}
}

func TestFakeEngineRegistryAndEnsureImage(t *testing.T) {
	eng := newFakeEngine()
	cli, _ := newTestComputeClient(t, eng)
	ctx := context.Background()

	if err := cli.LoadImageFromTar(ctx, bytes.NewReader([]byte("tar"))); err != nil {
		t.Fatalf("LoadImageFromTar: %v", err)
	}
	if err := cli.TagImage(ctx, "alpine:3.20", "lab/alpine:3.20"); err != nil {
		t.Fatalf("TagImage: %v", err)
	}
	ref := DinDPullHost("127.0.0.1:4566") + "/000000000001/repo:tag"
	if err := cli.PullLabRegistryImage(ctx, ref, "AWS", "token"); err != nil {
		t.Fatalf("PullLabRegistryImage: %v", err)
	}
	if err := cli.PullLabRegistryImage(ctx, "evil.example/x:1", "AWS", "token"); err == nil {
		t.Fatal("expected allowlist deny")
	}

	img, err := cli.EnsureImage(ctx, "python3.12")
	if err != nil || img == "" {
		t.Fatalf("EnsureImage: img=%q err=%v", img, err)
	}
	eng.pullFail = true
	if _, err := cli.EnsureImage(ctx, "python3.12"); err == nil {
		t.Fatal("expected pull failure")
	}
}

func TestFakeEngineExecAndSQL(t *testing.T) {
	eng := newFakeEngine()
	eng.execStdout = "id\tname\n1\to\n"
	eng.execCode = 0
	cli, _ := newTestComputeClient(t, eng)
	ctx := context.Background()
	cid := eng.seedRunningContainer("sql-ctr", DataPlaneNetworkName, "mysql:8.0", nil)

	res, err := cli.Exec(ctx, ExecOpts{ContainerID: cid, Cmd: []string{"true"}})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d", res.ExitCode)
	}
	res, err = cli.Exec(ctx, ExecOpts{ContainerID: cid, Cmd: []string{"cat"}, Stdin: "hi"})
	if err != nil {
		t.Fatalf("Exec stdin: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("stdin exit=%d", res.ExitCode)
	}
	if _, err := cli.Exec(ctx, ExecOpts{ContainerID: "", Cmd: []string{"true"}}); err == nil {
		t.Fatal("empty cid")
	}
	if _, err := cli.Exec(ctx, ExecOpts{ContainerID: cid}); err == nil {
		t.Fatal("empty cmd")
	}

	my, err := cli.ExecMySQLSQL(ctx, MySQLSQLOpts{
		ContainerID: cid,
		SQL:         "SELECT 1",
		Password:    "pw",
	})
	if err != nil {
		t.Fatalf("ExecMySQLSQL select: %v", err)
	}
	if !my.Select || len(my.Columns) == 0 {
		t.Fatalf("mysql select=%+v", my)
	}
	eng.execStdout = "Query OK, 2 rows affected"
	up, err := cli.ExecMySQLSQL(ctx, MySQLSQLOpts{ContainerID: cid, SQL: "UPDATE t SET a=1"})
	if err != nil {
		t.Fatalf("ExecMySQLSQL update: %v", err)
	}
	if up.Select || up.NumberOfRecordsUpdated != 2 {
		t.Fatalf("mysql update=%+v", up)
	}
	if _, err := cli.ExecMySQLSQL(ctx, MySQLSQLOpts{ContainerID: cid, SQL: ""}); err == nil {
		t.Fatal("empty sql")
	}
	eng.execCode = 1
	eng.execStdout = ""
	if _, err := cli.ExecMySQLSQL(ctx, MySQLSQLOpts{ContainerID: cid, SQL: "SELECT 1"}); err == nil {
		t.Fatal("mysql exit fail")
	}

	eng.execCode = 0
	eng.execStdout = "stub,n\nok,1\n"
	pg, err := cli.ExecPostgresSQL(ctx, PostgresSQLOpts{
		ContainerID: cid,
		SQL:         "SELECT 1",
		Password:    "pw",
	})
	if err != nil {
		t.Fatalf("ExecPostgresSQL: %v", err)
	}
	if !pg.Select {
		t.Fatalf("pg=%+v", pg)
	}
	eng.execStdout = "UPDATE 4"
	pgUp, err := cli.ExecPostgresSQL(ctx, PostgresSQLOpts{ContainerID: cid, SQL: "UPDATE t SET a=1"})
	if err != nil {
		t.Fatalf("ExecPostgresSQL update: %v", err)
	}
	if pgUp.NumberOfRecordsUpdated != 4 {
		t.Fatalf("pg update=%+v", pgUp)
	}
}

func TestFakeEngineDuckDBPaths(t *testing.T) {
	eng := newFakeEngine()
	eng.execStdout = `{"status":"success","rows":[{"x":1}]}`
	cli, _ := newTestComputeClient(t, eng)
	ctx := context.Background()

	inst, err := cli.EnsureDuckDB(ctx)
	if err != nil {
		t.Fatalf("EnsureDuckDB: %v", err)
	}
	if inst.Name != LabDuckContainerName {
		t.Fatalf("name=%q", inst.Name)
	}

	okCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := cli.WaitDuckHealthy(okCtx, inst.ContainerID, time.Second); err != nil {
		t.Fatalf("WaitDuckHealthy: %v", err)
	}
	if _, err := cli.ExecDuckQuery(ctx, "", NewDuckQueryRequest("SELECT 1", "", "")); err == nil {
		t.Fatal("empty container")
	}
	if _, err := cli.ExecDuckQuery(ctx, inst.ContainerID, NewDuckQueryRequest("", "", "")); err == nil {
		t.Fatal("empty sql")
	}
	out, err := cli.ExecDuckQuery(ctx, inst.ContainerID, NewDuckQueryRequest("SELECT 1", "", "test"))
	if err != nil {
		t.Fatalf("ExecDuckQuery: %v", err)
	}
	if len(out.Rows) != 1 {
		t.Fatalf("out=%+v", out)
	}
	eng.execCode = 1
	if _, err := cli.ExecDuckQuery(ctx, inst.ContainerID, NewDuckQueryRequest("SELECT 1", "", "")); err == nil {
		t.Fatal("duck exit fail")
	}
}

func TestFakeEngineEC2Lifecycle(t *testing.T) {
	eng := newFakeEngine()
	cli, _ := newTestComputeClient(t, eng)
	ctx := context.Background()

	res, err := cli.StartEC2Instance(ctx, EC2RunOpts{
		ImageURI:           "alpine:3.20",
		InstanceID:         "i-lab1",
		AMIID:              "ami-lab",
		IamInstanceProfile: "role",
		UserData:           "#!/bin/sh\necho hi\n",
		Env:                map[string]string{"": "skip", "MARK": "1"},
	})
	if err != nil {
		t.Fatalf("StartEC2Instance: %v", err)
	}
	if res.ContainerID == "" || res.PrivateIP == "" {
		t.Fatalf("res=%+v", res)
	}
	if err := cli.StopEC2Instance(ctx, res.ContainerID); err != nil {
		t.Fatalf("StopEC2Instance: %v", err)
	}
	if err := cli.StartStoppedEC2Instance(ctx, res.ContainerID); err != nil {
		t.Fatalf("StartStoppedEC2Instance: %v", err)
	}
	if err := cli.TerminateEC2Instance(ctx, res.ContainerID); err != nil {
		t.Fatalf("TerminateEC2Instance: %v", err)
	}
	if err := cli.StopEC2Instance(ctx, ""); err == nil {
		t.Fatal("empty stop")
	}
	if err := cli.StartStoppedEC2Instance(ctx, ""); err == nil {
		t.Fatal("empty start")
	}
	if err := cli.TerminateEC2Instance(ctx, ""); err == nil {
		t.Fatal("empty terminate")
	}
}

func TestFakeEngineECSIMDSAttach(t *testing.T) {
	eng := newFakeEngine()
	cli, _ := newTestComputeClient(t, eng)
	ctx := context.Background()

	hosts, uriEnv, err := cli.attachECSIMDSEnv(ctx, map[string]string{
		"AWS_ACCESS_KEY_ID":     "AKIATEST",
		"AWS_SECRET_ACCESS_KEY": "secret",
		"AWS_SESSION_TOKEN":     "tok",
	})
	if err != nil {
		t.Fatalf("attachECSIMDSEnv: %v", err)
	}
	if len(hosts) == 0 || uriEnv["AWS_CONTAINER_CREDENTIALS_FULL_URI"] == "" {
		t.Fatalf("hosts=%v env=%v", hosts, uriEnv)
	}
	// Second call should reuse running sidecar.
	hosts2, _, err := cli.attachECSIMDSEnv(ctx, map[string]string{
		"AWS_ACCESS_KEY_ID":     "AKIATEST2",
		"AWS_SECRET_ACCESS_KEY": "secret2",
	})
	if err != nil {
		t.Fatalf("reuse: %v", err)
	}
	if len(hosts2) == 0 {
		t.Fatal("expected hosts on reuse")
	}

	cid, err := cli.RunECSTask(ctx, ECSRunOpts{
		ImageURI: "alpine:3.20",
		Env: map[string]string{
			"AWS_ACCESS_KEY_ID":     "AKIATEST",
			"AWS_SECRET_ACCESS_KEY": "secret",
		},
	})
	if err != nil {
		t.Fatalf("RunECSTask with imds: %v", err)
	}
	_ = cli.StopECSTask(ctx, cid)
}

func TestFakeEngineRunInvokeAndImageInvoke(t *testing.T) {
	eng := newFakeEngine()
	eng.logsStdout = `{"ok":true}`
	cli, _ := newTestComputeClient(t, eng)
	ctx := context.Background()

	codeDir := filepath.Join(t.TempDir(), "task")
	// ValidateRunOpts requires a POSIX absolute path; on Windows MkdirAll still works under the drive root.
	posixCode := "/noctaxris-test-task-" + filepath.Base(t.TempDir())
	if err := os.MkdirAll(posixCode, 0o755); err != nil {
		t.Skipf("cannot create POSIX-style path %s: %v", posixCode, err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(posixCode) })
	_ = codeDir

	out, err := cli.RunInvoke(ctx, RunOpts{
		CodeHostPath: posixCode,
		Runtime:      "python3.12",
		Handler:      "app.handler",
		EventJSON:    `{"x":1}`,
		TimeoutSec:   5,
		Env:          map[string]string{"": "skip", "A": "1"},
	})
	if err != nil {
		t.Fatalf("RunInvoke: %v", err)
	}
	if !bytes.Contains(out.Payload, []byte("ok")) {
		t.Fatalf("payload=%s logs=%s", out.Payload, out.Logs)
	}

	eventDir := "/noctaxris-test-event-" + filepath.Base(t.TempDir())
	if err := os.MkdirAll(eventDir, 0o755); err != nil {
		t.Skipf("event dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(eventDir) })
	imgOut, err := cli.RunImageInvoke(ctx, ImageRunOpts{
		ImageURI:      "public.ecr.aws/lambda/python:3.12",
		Handler:       "app.handler",
		EventHostPath: eventDir,
		EventJSON:     `{}`,
		TimeoutSec:    5,
	})
	if err != nil {
		t.Fatalf("RunImageInvoke: %v", err)
	}
	if !bytes.Contains(imgOut.Payload, []byte("ok")) {
		t.Fatalf("img payload=%s", imgOut.Payload)
	}
}
