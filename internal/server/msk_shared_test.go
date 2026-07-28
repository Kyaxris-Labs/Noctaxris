package server_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/server"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMSKSharedCreateBootstrapBrokers(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.SharedKafka = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustMSKJSON(t, handler, "CreateCluster", map[string]any{
		"ClusterName":         "lab-shared-1",
		"KafkaVersion":        "3.6.0",
		"NumberOfBrokerNodes": 1,
	}, now)
	if create.Code != 200 {
		t.Fatalf("CreateCluster status=%d body=%q", create.Code, create.Body.String())
	}
	got, err := st.DescribeMSKClusterByName(testAccountID, "lab-shared-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.BootstrapBrokers != store.MSKSharedBootstrap {
		t.Fatalf("bootstrap=%q want %q", got.BootstrapBrokers, store.MSKSharedBootstrap)
	}
}

func TestMSKSharedSecondClusterLimitExceeded(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.SharedKafka = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	first := mustMSKJSON(t, handler, "CreateCluster", map[string]any{
		"ClusterName": "shared-one",
	}, now)
	if first.Code != 200 {
		t.Fatalf("first create status=%d body=%q", first.Code, first.Body.String())
	}
	// Without DinD the handler may mark FAILED; CREATING/ACTIVE hold the shared slot.
	if err := st.SetMSKContainerID(testAccountID, "shared-one", compute.LabKafkaContainerName, store.MSKClusterStateActive, store.MSKSharedBootstrap); err != nil {
		t.Fatal(err)
	}

	second := mustMSKJSON(t, handler, "CreateCluster", map[string]any{
		"ClusterName": "shared-two",
	}, now)
	if second.Code != 400 {
		t.Fatalf("second create status=%d want 400 body=%q", second.Code, second.Body.String())
	}
	body := second.Body.String()
	if !strings.Contains(body, `"__type":"LimitExceededException"`) {
		t.Fatalf("want LimitExceededException body=%q", body)
	}
	if !strings.Contains(body, store.MSKMsgSharedOneClusterLimit) {
		t.Fatalf("want limit message body=%q", body)
	}
}

func TestMSKSharedDeleteSkipsStopHook(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.SharedKafka = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustMSKJSON(t, handler, "CreateCluster", map[string]any{
		"ClusterName": "shared-del",
	}, now)
	if create.Code != 200 {
		t.Fatalf("create status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	arn, _ := out["ClusterArn"].(string)
	labID := compute.LabKafkaContainerName
	if err := st.SetMSKContainerID(testAccountID, "shared-del", labID, store.MSKClusterStateActive, store.MSKSharedBootstrap); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var stopped []string
	server.SetStopDataPlaneHookForTest(func(containerID string) {
		mu.Lock()
		stopped = append(stopped, containerID)
		mu.Unlock()
	})
	t.Cleanup(func() { server.SetStopDataPlaneHookForTest(nil) })

	del := mustMSKJSON(t, handler, "DeleteCluster", map[string]any{"ClusterArn": arn}, now)
	if del.Code != 200 {
		t.Fatalf("delete status=%d body=%q", del.Code, del.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(stopped) != 0 {
		t.Fatalf("shared delete must not stop data plane, got %v", stopped)
	}
}

func TestMSKSharedRemapRefuseHandler(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.SharedKafka = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateMSKCluster(testAccountID, "us-east-1", "legacy", "3.6.0", 1, false); err != nil {
		t.Fatal(err)
	}

	create := mustMSKJSON(t, handler, "CreateCluster", map[string]any{
		"ClusterName": "shared-blocked",
	}, now)
	if create.Code != 400 {
		t.Fatalf("create status=%d want 400 body=%q", create.Code, create.Body.String())
	}
	body := create.Body.String()
	if !strings.Contains(body, `"__type":"BadRequestException"`) {
		t.Fatalf("want BadRequestException body=%q", body)
	}
	if !strings.Contains(body, store.MSKMsgSharedRemapRefuse) {
		t.Fatalf("want remap message body=%q", body)
	}
}
