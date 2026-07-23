package server

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestPromoteNestedDataAfterWaitFailure(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	srv := &Server{store: st}
	account := "000000000001"
	if _, err := st.CreateElastiCacheCluster(account, "us-east-1", "wait-fail-cache", "redis", "", "", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateDocDBCluster(account, "us-east-1", "wait-fail-docdb", "docdb", "", "labadmin", 0); err != nil {
		t.Fatal(err)
	}
	mq, err := st.CreateMQBroker(account, "us-east-1", "wait-fail-mq", "RABBITMQ", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateOpenSearchDomain(account, "us-east-1", "wait-fail-os", "OpenSearch_2.11"); err != nil {
		t.Fatal(err)
	}

	waitErr := errors.New("compute: data-plane healthy wait: timeout")
	if err := promoteNestedDataAfterWait(srv, account, compute.DataKindElastiCache, "wait-fail-cache", "ctr-ec", "host-ec", waitErr); err == nil {
		t.Fatal("expected wait error returned")
	}
	ec, err := st.DescribeElastiCacheCluster(account, "wait-fail-cache")
	if err != nil || ec.Status != "failed" {
		t.Fatalf("elasticache after wait fail: %+v err=%v", ec, err)
	}

	if err := promoteNestedDataAfterWait(srv, account, compute.DataKindDocDB, "wait-fail-docdb", "ctr-dd", "host-dd", waitErr); err == nil {
		t.Fatal("expected wait error returned")
	}
	dd, err := st.DescribeDocDBCluster(account, "wait-fail-docdb")
	if err != nil || dd.Status != "failed" {
		t.Fatalf("docdb after wait fail: %+v err=%v", dd, err)
	}

	if err := promoteNestedDataAfterWait(srv, account, compute.DataKindMQ, mq.BrokerID, "ctr-mq", "host-mq", waitErr); err == nil {
		t.Fatal("expected wait error returned")
	}
	mqFailed, err := st.DescribeMQBroker(account, mq.BrokerID)
	if err != nil || mqFailed.BrokerState != store.MQBrokerStateCreationFailed {
		t.Fatalf("mq after wait fail: %+v err=%v", mqFailed, err)
	}

	mapCountErr := errors.New("compute: data-plane container exited: max virtual memory areas vm.max_map_count [65530] is too low")
	if err := promoteNestedDataAfterWait(srv, account, compute.DataKindOpenSearch, "wait-fail-os", "ctr-os", "host-os", mapCountErr); err == nil {
		t.Fatal("expected wait error returned")
	}
	osFailed, err := st.DescribeOpenSearchDomain(account, "wait-fail-os")
	if err != nil || osFailed.DomainStatus != store.OpenSearchDomainStatusCreateFailed {
		t.Fatalf("opensearch after wait fail: %+v err=%v", osFailed, err)
	}
	if osFailed.FailureReason == "" || !strings.Contains(osFailed.FailureReason, "vm.max_map_count") {
		t.Fatalf("opensearch CreateFailed missing mmap FailureReason: %+v", osFailed)
	}

	if err := promoteNestedDataAfterWait(srv, account, compute.DataKindElastiCache, "wait-fail-cache", "ctr-ec2", "host-ec2", nil); err != nil {
		t.Fatal(err)
	}
	ecReady, err := st.DescribeElastiCacheCluster(account, "wait-fail-cache")
	if err != nil || ecReady.Status != "available" || ecReady.ContainerID != "ctr-ec2" {
		t.Fatalf("elasticache after wait ok: %+v err=%v", ecReady, err)
	}

	if err := promoteNestedDataAfterWait(srv, account, compute.DataKindMQ, mq.BrokerID, "ctr-mq2", "host-mq2", nil); err != nil {
		t.Fatal(err)
	}
	mqReady, err := st.DescribeMQBroker(account, mq.BrokerID)
	if err != nil || mqReady.BrokerState != store.MQBrokerStateRunning || mqReady.ContainerID != "ctr-mq2" {
		t.Fatalf("mq after wait ok: %+v err=%v", mqReady, err)
	}
	if err := promoteNestedDataAfterWait(srv, account, compute.DataKindOpenSearch, "wait-fail-os", "ctr-os2", "host-os2", nil); err != nil {
		t.Fatal(err)
	}
	osReady, err := st.DescribeOpenSearchDomain(account, "wait-fail-os")
	if err != nil || osReady.DomainStatus != store.OpenSearchDomainStatusActive || osReady.ContainerID != "ctr-os2" {
		t.Fatalf("opensearch after wait ok: %+v err=%v", osReady, err)
	}
}
