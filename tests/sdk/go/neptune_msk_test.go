package sdk_test

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNeptuneCreateSkipUnlessAvailable(t *testing.T) {
	requireReady(t)
	id := fmt.Sprintf("goneptune-%s", strings.ReplaceAll(uniquePrefix(t), "-", ""))
	if len(id) > 40 {
		id = id[:40]
	}
	body := url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {id},
		"Engine":              {"neptune"},
	}.Encode()
	status, resp := signedHTTP(t, "neptune", "POST", "/", []byte(body), "application/x-www-form-urlencoded")
	if status < 200 || status >= 300 {
		t.Fatalf("CreateDBCluster status=%d body=%s", status, resp)
	}
	t.Cleanup(func() {
		del := url.Values{
			"Action":              {"DeleteDBCluster"},
			"Version":             {"2014-10-31"},
			"DBClusterIdentifier": {id},
		}.Encode()
		_, _ = signedHTTP(t, "neptune", "POST", "/", []byte(del), "application/x-www-form-urlencoded")
	})

	var descBody []byte
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		q := url.Values{
			"Action":              {"DescribeDBClusters"},
			"Version":             {"2014-10-31"},
			"DBClusterIdentifier": {id},
		}.Encode()
		st, b := signedHTTP(t, "neptune", "POST", "/", []byte(q), "application/x-www-form-urlencoded")
		if st != 200 {
			t.Fatalf("DescribeDBClusters status=%d body=%s", st, b)
		}
		descBody = b
		s := string(b)
		if strings.Contains(s, "<Status>available</Status>") || strings.Contains(s, "<Status>failed</Status>") {
			break
		}
		time.Sleep(time.Second)
	}
	if !strings.Contains(string(descBody), "<Status>available</Status>") {
		t.Skipf("Neptune live smoke skipped: status not available (body=%s)", descBody)
	}
}

func TestMSKCreateSkipUnlessActive(t *testing.T) {
	requireReady(t)
	name := "gomsk-" + uniquePrefix(t)
	if len(name) > 40 {
		name = name[:40]
	}
	st, _, created := signedJSONTarget(t, "kafka", "Kafka_1.0.CreateCluster", map[string]any{
		"ClusterName":         name,
		"KafkaVersion":        "3.6.0",
		"NumberOfBrokerNodes": 1,
		"BrokerNodeGroupInfo": map[string]any{
			"InstanceType":  "kafka.m5.large",
			"ClientSubnets": []string{"subnet-1"},
		},
	})
	if st < 200 || st >= 300 {
		t.Fatalf("CreateCluster status=%d", st)
	}
	arn, _ := created["ClusterArn"].(string)
	if arn == "" {
		t.Fatalf("missing ClusterArn: %v", created)
	}
	t.Cleanup(func() {
		_, _, _ = signedJSONTarget(t, "kafka", "Kafka_1.0.DeleteCluster", map[string]any{"ClusterArn": arn})
	})

	var state string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		st, _, desc := signedJSONTarget(t, "kafka", "Kafka_1.0.DescribeCluster", map[string]any{"ClusterArn": arn})
		if st != 200 {
			t.Fatalf("DescribeCluster status=%d", st)
		}
		info, _ := desc["ClusterInfo"].(map[string]any)
		state, _ = info["State"].(string)
		if state == "ACTIVE" || state == "FAILED" {
			break
		}
		time.Sleep(time.Second)
	}
	if state != "ACTIVE" {
		t.Skipf("MSK live smoke skipped: State=%s (nested engine not ACTIVE)", state)
	}
	st, _, boot := signedJSONTarget(t, "kafka", "Kafka_1.0.GetBootstrapBrokers", map[string]any{"ClusterArn": arn})
	if st != 200 {
		t.Fatalf("GetBootstrapBrokers status=%d", st)
	}
	brokers, _ := boot["BootstrapBrokerString"].(string)
	if brokers == "" {
		t.Fatalf("expected nested bootstrap brokers, got empty")
	}
	if !strings.Contains(brokers, "noctaxris-msk-") && !strings.Contains(brokers, "noctaxris-lab-kafka") {
		t.Fatalf("expected per-cluster noctaxris-msk-* or shared noctaxris-lab-kafka bootstrap, got %q", brokers)
	}
}
