package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFirehoseDescribeListDeleteAndPutRecordBatch(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := st.CreateBucket(testAccountID, "cov-fh-bucket"); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"s3:PutObject","Resource":"*"}]}`
	if err := st.PutBucketPolicy(testAccountID, "cov-fh-bucket", policy); err != nil {
		t.Fatal(err)
	}

	create := mustJSONTarget(t, handler, "Firehose_20150804.CreateDeliveryStream", "firehose", map[string]any{
		"DeliveryStreamName": "cov-fh",
		"S3DestinationConfiguration": map[string]any{
			"BucketARN": "arn:aws:s3:::cov-fh-bucket",
			"Prefix":    "data/",
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDeliveryStream status=%d body=%q", create.Code, create.Body.String())
	}

	desc := mustJSONTarget(t, handler, "Firehose_20150804.DescribeDeliveryStream", "firehose", map[string]any{
		"DeliveryStreamName": "cov-fh",
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "cov-fh") {
		t.Fatalf("DescribeDeliveryStream status=%d body=%q", desc.Code, desc.Body.String())
	}
	missing := mustJSONTarget(t, handler, "Firehose_20150804.DescribeDeliveryStream", "firehose", map[string]any{
		"DeliveryStreamName": "nope",
	}, now)
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("Describe missing want ResourceNotFound status=%d body=%q", missing.Code, missing.Body.String())
	}

	list := mustJSONTarget(t, handler, "Firehose_20150804.ListDeliveryStreams", "firehose", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "cov-fh") {
		t.Fatalf("ListDeliveryStreams status=%d body=%q", list.Code, list.Body.String())
	}

	batch := mustJSONTarget(t, handler, "Firehose_20150804.PutRecordBatch", "firehose", map[string]any{
		"DeliveryStreamName": "cov-fh",
		"Records": []map[string]any{
			{"Data": base64.StdEncoding.EncodeToString([]byte("a"))},
			{"Data": base64.StdEncoding.EncodeToString([]byte("b"))},
		},
	}, now)
	if batch.Code != http.StatusOK {
		t.Fatalf("PutRecordBatch status=%d body=%q", batch.Code, batch.Body.String())
	}
	batchMissing := mustJSONTarget(t, handler, "Firehose_20150804.PutRecordBatch", "firehose", map[string]any{
		"DeliveryStreamName": "nope",
		"Records":            []map[string]any{{"Data": base64.StdEncoding.EncodeToString([]byte("x"))}},
	}, now)
	if batchMissing.Code != http.StatusBadRequest {
		t.Fatalf("PutRecordBatch missing want 400 status=%d body=%q", batchMissing.Code, batchMissing.Body.String())
	}

	del := mustJSONTarget(t, handler, "Firehose_20150804.DeleteDeliveryStream", "firehose", map[string]any{
		"DeliveryStreamName": "cov-fh",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteDeliveryStream status=%d body=%q", del.Code, del.Body.String())
	}
	delGone := mustJSONTarget(t, handler, "Firehose_20150804.DeleteDeliveryStream", "firehose", map[string]any{
		"DeliveryStreamName": "cov-fh",
	}, now)
	if delGone.Code != http.StatusBadRequest || !strings.Contains(delGone.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("Delete missing want ResourceNotFound status=%d body=%q", delGone.Code, delGone.Body.String())
	}

	unknown := mustJSONTarget(t, handler, "Firehose_20150804.UpdateDestination", "firehose", map[string]any{}, now)
	if unknown.Code != http.StatusNotImplemented {
		t.Fatalf("unknown Firehose action want 501 status=%d body=%q", unknown.Code, unknown.Body.String())
	}
}

func TestELBv2DescribeDeleteListenersAndAttributes(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	lb := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateLoadBalancer", "elasticloadbalancing", map[string]any{
		"Name": "cov-alb", "Type": "application", "Scheme": "internet-facing",
		"Subnets": []string{"subnet-a", "subnet-b"},
	}, now)
	if lb.Code != http.StatusOK {
		t.Fatalf("CreateLoadBalancer status=%d body=%q", lb.Code, lb.Body.String())
	}
	var lbOut map[string]any
	_ = json.Unmarshal(lb.Body.Bytes(), &lbOut)
	lbs, _ := lbOut["LoadBalancers"].([]any)
	lbARN := ""
	if len(lbs) > 0 {
		if m, ok := lbs[0].(map[string]any); ok {
			lbARN, _ = m["LoadBalancerArn"].(string)
		}
	}
	if lbARN == "" {
		t.Fatalf("missing LB ARN: %s", lb.Body.String())
	}

	tg := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "cov-tg", "Protocol": "HTTP", "Port": 80, "VpcId": "vpc-1", "TargetType": "ip",
	}, now)
	if tg.Code != http.StatusOK {
		t.Fatalf("CreateTargetGroup status=%d body=%q", tg.Code, tg.Body.String())
	}
	var tgOut map[string]any
	_ = json.Unmarshal(tg.Body.Bytes(), &tgOut)
	tgs, _ := tgOut["TargetGroups"].([]any)
	tgARN := ""
	if len(tgs) > 0 {
		if m, ok := tgs[0].(map[string]any); ok {
			tgARN, _ = m["TargetGroupArn"].(string)
		}
	}

	listener := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateListener", "elasticloadbalancing", map[string]any{
		"LoadBalancerArn": lbARN, "Protocol": "HTTP", "Port": 80,
		"DefaultActions": []map[string]any{{
			"Type": "forward", "TargetGroupArn": tgARN,
		}},
	}, now)
	if listener.Code != http.StatusOK {
		t.Fatalf("CreateListener status=%d body=%q", listener.Code, listener.Body.String())
	}
	var lisOut map[string]any
	_ = json.Unmarshal(listener.Body.Bytes(), &lisOut)
	listeners, _ := lisOut["Listeners"].([]any)
	lisARN := ""
	if len(listeners) > 0 {
		if m, ok := listeners[0].(map[string]any); ok {
			lisARN, _ = m["ListenerArn"].(string)
		}
	}

	descLB := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DescribeLoadBalancers", "elasticloadbalancing", map[string]any{
		"LoadBalancerArns": []string{lbARN},
	}, now)
	if descLB.Code != http.StatusOK || !strings.Contains(descLB.Body.String(), "cov-alb") {
		t.Fatalf("DescribeLoadBalancers status=%d body=%q", descLB.Code, descLB.Body.String())
	}
	descAll := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DescribeLoadBalancers", "elasticloadbalancing", map[string]any{}, now)
	if descAll.Code != http.StatusOK {
		t.Fatalf("DescribeLoadBalancers all status=%d body=%q", descAll.Code, descAll.Body.String())
	}
	attrs := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DescribeLoadBalancerAttributes", "elasticloadbalancing", map[string]any{
		"LoadBalancerArn": lbARN,
	}, now)
	if attrs.Code != http.StatusOK {
		t.Fatalf("DescribeLoadBalancerAttributes status=%d body=%q", attrs.Code, attrs.Body.String())
	}

	descTG := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DescribeTargetGroups", "elasticloadbalancing", map[string]any{
		"TargetGroupArns": []string{tgARN},
	}, now)
	if descTG.Code != http.StatusOK || !strings.Contains(descTG.Body.String(), "cov-tg") {
		t.Fatalf("DescribeTargetGroups status=%d body=%q", descTG.Code, descTG.Body.String())
	}
	descLis := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DescribeListeners", "elasticloadbalancing", map[string]any{
		"LoadBalancerArn": lbARN,
	}, now)
	if descLis.Code != http.StatusOK || !strings.Contains(descLis.Body.String(), lisARN) {
		t.Fatalf("DescribeListeners status=%d body=%q", descLis.Code, descLis.Body.String())
	}

	delLis := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DeleteListener", "elasticloadbalancing", map[string]any{
		"ListenerArn": lisARN,
	}, now)
	if delLis.Code != http.StatusOK {
		t.Fatalf("DeleteListener status=%d body=%q", delLis.Code, delLis.Body.String())
	}
	delLisGone := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DeleteListener", "elasticloadbalancing", map[string]any{
		"ListenerArn": lisARN,
	}, now)
	if delLisGone.Code == http.StatusOK {
		t.Fatalf("DeleteListener missing should fail: %q", delLisGone.Body.String())
	}
	delTG := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DeleteTargetGroup", "elasticloadbalancing", map[string]any{
		"TargetGroupArn": tgARN,
	}, now)
	if delTG.Code != http.StatusOK {
		t.Fatalf("DeleteTargetGroup status=%d body=%q", delTG.Code, delTG.Body.String())
	}
	delTGGone := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DeleteTargetGroup", "elasticloadbalancing", map[string]any{
		"TargetGroupArn": tgARN,
	}, now)
	if delTGGone.Code == http.StatusOK {
		t.Fatalf("DeleteTargetGroup missing should fail: %q", delTGGone.Body.String())
	}
}

func TestLightsailGetListsCoverage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	bundles := mustJSONTarget(t, handler, "Lightsail_20161128.GetBundles", "lightsail", map[string]any{}, now)
	if bundles.Code != http.StatusOK {
		t.Fatalf("GetBundles status=%d body=%q", bundles.Code, bundles.Body.String())
	}

	create := mustJSONTarget(t, handler, "Lightsail_20161128.CreateInstances", "lightsail", map[string]any{
		"instanceNames": []string{"cov-ls"}, "availabilityZone": "us-east-1a",
		"blueprintId": "ubuntu_22_04", "bundleId": "nano_3_0",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateInstances status=%d body=%q", create.Code, create.Body.String())
	}
	listInst := mustJSONTarget(t, handler, "Lightsail_20161128.GetInstances", "lightsail", map[string]any{}, now)
	if listInst.Code != http.StatusOK || !strings.Contains(listInst.Body.String(), "cov-ls") {
		t.Fatalf("GetInstances status=%d body=%q", listInst.Code, listInst.Body.String())
	}

	mustJSONTarget(t, handler, "Lightsail_20161128.CreateDisk", "lightsail", map[string]any{
		"diskName": "cov-disk", "availabilityZone": "us-east-1a", "sizeInGb": 8,
	}, now)
	listDisks := mustJSONTarget(t, handler, "Lightsail_20161128.GetDisks", "lightsail", map[string]any{}, now)
	if listDisks.Code != http.StatusOK || !strings.Contains(listDisks.Body.String(), "cov-disk") {
		t.Fatalf("GetDisks status=%d body=%q", listDisks.Code, listDisks.Body.String())
	}

	mustJSONTarget(t, handler, "Lightsail_20161128.AllocateStaticIp", "lightsail", map[string]any{
		"staticIpName": "cov-ip",
	}, now)
	getIP := mustJSONTarget(t, handler, "Lightsail_20161128.GetStaticIp", "lightsail", map[string]any{
		"staticIpName": "cov-ip",
	}, now)
	if getIP.Code != http.StatusOK {
		t.Fatalf("GetStaticIp status=%d body=%q", getIP.Code, getIP.Body.String())
	}
	listIPs := mustJSONTarget(t, handler, "Lightsail_20161128.GetStaticIps", "lightsail", map[string]any{}, now)
	if listIPs.Code != http.StatusOK || !strings.Contains(listIPs.Body.String(), "cov-ip") {
		t.Fatalf("GetStaticIps status=%d body=%q", listIPs.Code, listIPs.Body.String())
	}
	missingIP := mustJSONTarget(t, handler, "Lightsail_20161128.GetStaticIp", "lightsail", map[string]any{
		"staticIpName": "missing",
	}, now)
	if missingIP.Code == http.StatusOK {
		t.Fatalf("GetStaticIp missing should fail: %q", missingIP.Body.String())
	}

	mustJSONTarget(t, handler, "Lightsail_20161128.CreateKeyPair", "lightsail", map[string]any{
		"keyPairName": "cov-key",
	}, now)
	listKP := mustJSONTarget(t, handler, "Lightsail_20161128.GetKeyPairs", "lightsail", map[string]any{}, now)
	if listKP.Code != http.StatusOK || !strings.Contains(listKP.Body.String(), "cov-key") {
		t.Fatalf("GetKeyPairs status=%d body=%q", listKP.Code, listKP.Body.String())
	}
}
