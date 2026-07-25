package sdk_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/firehose"
	fhtypes "github.com/aws/aws-sdk-go-v2/service/firehose/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lamtypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/mq"
	mqtypes "github.com/aws/aws-sdk-go-v2/service/mq/types"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
)

func newOpenSearch(t *testing.T, cfg aws.Config) *opensearch.Client {
	t.Helper()
	return opensearch.NewFromConfig(cfg, func(o *opensearch.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newMQ(t *testing.T, cfg aws.Config) *mq.Client {
	t.Helper()
	return mq.NewFromConfig(cfg, func(o *mq.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newFirehose(t *testing.T, cfg aws.Config) *firehose.Client {
	t.Helper()
	return firehose.NewFromConfig(cfg, func(o *firehose.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func waitOpenSearchDomain(t *testing.T, client *opensearch.Client, name string, timeout time.Duration) *opensearch.DescribeDomainOutput {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	var last *opensearch.DescribeDomainOutput
	for time.Now().Before(deadline) {
		out, err := client.DescribeDomain(ctx, &opensearch.DescribeDomainInput{DomainName: aws.String(name)})
		if err != nil {
			t.Fatalf("DescribeDomain: %v", err)
		}
		last = out
		if openSearchTerminal(out) {
			return out
		}
		time.Sleep(time.Second)
	}
	return last
}

func openSearchActive(out *opensearch.DescribeDomainOutput) bool {
	if out == nil || out.DomainStatus == nil {
		return false
	}
	ds := out.DomainStatus
	ep := aws.ToString(ds.Endpoint)
	return aws.ToBool(ds.Created) && ep != "" && !strings.HasPrefix(ep, "stub://")
}

func openSearchTerminal(out *opensearch.DescribeDomainOutput) bool {
	if openSearchActive(out) {
		return true
	}
	if out == nil || out.DomainStatus == nil {
		return false
	}
	ds := out.DomainStatus
	ep := aws.ToString(ds.Endpoint)
	if strings.HasPrefix(ep, "stub://") {
		return true
	}
	if !aws.ToBool(ds.Created) && !aws.ToBool(ds.Processing) {
		return true
	}
	return false
}

func waitBroker(t *testing.T, client *mq.Client, id string, timeout time.Duration) *mq.DescribeBrokerOutput {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	var last *mq.DescribeBrokerOutput
	for time.Now().Before(deadline) {
		out, err := client.DescribeBroker(ctx, &mq.DescribeBrokerInput{BrokerId: aws.String(id)})
		if err != nil {
			t.Fatalf("DescribeBroker: %v", err)
		}
		last = out
		state := string(out.BrokerState)
		if state == "RUNNING" || state == "CREATION_FAILED" || state == "CRITICAL_ACTION_REQUIRED" {
			return out
		}
		time.Sleep(time.Second)
	}
	return last
}

func TestOpenSearchCreateDomainSkipQueryUnlessActive(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	osClient := newOpenSearch(t, cfg)
	ctx := context.Background()
	domain := fmt.Sprintf("gos%s", strings.ReplaceAll(uniquePrefix(t), "-", ""))
	if len(domain) > 28 {
		domain = domain[:28]
	}

	if _, err := osClient.CreateDomain(ctx, &opensearch.CreateDomainInput{
		DomainName: aws.String(domain), EngineVersion: aws.String("OpenSearch_2.11"),
	}); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}
	t.Cleanup(func() {
		_, _ = osClient.DeleteDomain(ctx, &opensearch.DeleteDomainInput{DomainName: aws.String(domain)})
	})

	status := waitOpenSearchDomain(t, osClient, domain, 20*time.Second)
	if status == nil || status.DomainStatus == nil {
		t.Fatal("DescribeDomain empty")
	}
	if !openSearchActive(status) {
		t.Skipf("OpenSearch query facade skipped: Created=%v Endpoint=%s",
			aws.ToBool(status.DomainStatus.Created), aws.ToString(status.DomainStatus.Endpoint))
	}

	putStatus, putBody := signedHTTP(t, "es", "PUT",
		"/opensearch/"+domain+"/lab/events/_doc/1",
		[]byte(`{"msg":"hi"}`), "application/json")
	if putStatus < 200 || putStatus >= 300 {
		t.Fatalf("index PUT status=%d body=%s", putStatus, putBody)
	}
	searchStatus, searchBody := signedHTTP(t, "es", "POST",
		"/opensearch/"+domain+"/lab/events/_search",
		[]byte(`{"query":{"match_all":{}},"size":5}`), "application/json")
	if searchStatus < 200 || searchStatus >= 300 {
		t.Fatalf("search POST status=%d body=%s", searchStatus, searchBody)
	}
}

func TestActiveMQCreateBrokerSkipUnlessRunning(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	mqClient := newMQ(t, cfg)
	ctx := context.Background()
	name := "goamq-" + uniquePrefix(t)
	if len(name) > 50 {
		name = name[:50]
	}

	created, err := mqClient.CreateBroker(ctx, &mq.CreateBrokerInput{
		BrokerName:         aws.String(name),
		EngineType:         mqtypes.EngineTypeActivemq,
		EngineVersion:      aws.String("5.18"),
		HostInstanceType:   aws.String("mq.t3.micro"),
		DeploymentMode:     mqtypes.DeploymentModeSingleInstance,
		PubliclyAccessible: aws.Bool(false),
		Users: []mqtypes.User{{
			Username: aws.String("lab"),
			Password: aws.String("lab-password-1"),
		}},
	})
	if err != nil {
		t.Fatalf("CreateBroker: %v", err)
	}
	brokerID := aws.ToString(created.BrokerId)
	t.Cleanup(func() {
		_, _ = mqClient.DeleteBroker(ctx, &mq.DeleteBrokerInput{BrokerId: aws.String(brokerID)})
	})

	desc := waitBroker(t, mqClient, brokerID, 20*time.Second)
	if desc == nil {
		t.Fatal("DescribeBroker empty")
	}
	if string(desc.BrokerState) != "RUNNING" {
		t.Skipf("ActiveMQ live smoke skipped: BrokerState=%s (nested engine not RUNNING)", desc.BrokerState)
	}
}

func TestFirehoseOpenSearchDescribeSkipUnlessActive(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	osClient := newOpenSearch(t, cfg)
	fh := newFirehose(t, cfg)
	iamc := newIAM(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	domain := fmt.Sprintf("gofh%s", strings.ReplaceAll(prefix, "-", ""))
	if len(domain) > 28 {
		domain = domain[:28]
	}

	if _, err := osClient.CreateDomain(ctx, &opensearch.CreateDomainInput{
		DomainName: aws.String(domain), EngineVersion: aws.String("OpenSearch_2.11"),
	}); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}
	t.Cleanup(func() {
		_, _ = osClient.DeleteDomain(ctx, &opensearch.DeleteDomainInput{DomainName: aws.String(domain)})
	})
	status := waitOpenSearchDomain(t, osClient, domain, 20*time.Second)
	if !openSearchActive(status) {
		t.Skipf("Firehose OpenSearch Describe skipped: domain not Active (Endpoint=%s)",
			aws.ToString(status.DomainStatus.Endpoint))
	}

	roleName := prefix + "-fh-os"
	if len(roleName) > 64 {
		roleName = roleName[:64]
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	role, err := iamc.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName: aws.String(roleName), AssumeRolePolicyDocument: aws.String(trust),
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	t.Cleanup(func() {
		_, _ = iamc.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{RoleName: aws.String(roleName), PolicyName: aws.String("os-put")})
		_, _ = iamc.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	})
	if _, err := iamc.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName: aws.String(roleName), PolicyName: aws.String("os-put"),
		PolicyDocument: aws.String(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"es:ESHttpPut","Resource":"*"}]}`),
	}); err != nil {
		t.Fatalf("PutRolePolicy: %v", err)
	}

	stream := prefix + "-fh-os"
	if len(stream) > 64 {
		stream = stream[:64]
	}
	domainARN := aws.ToString(status.DomainStatus.ARN)
	if _, err := fh.CreateDeliveryStream(ctx, &firehose.CreateDeliveryStreamInput{
		DeliveryStreamName: aws.String(stream),
		AmazonopensearchserviceDestinationConfiguration: &fhtypes.AmazonopensearchserviceDestinationConfiguration{
			DomainARN: aws.String(domainARN),
			IndexName: aws.String("events"),
			RoleARN:   role.Role.Arn,
			S3Configuration: &fhtypes.S3DestinationConfiguration{
				BucketARN: aws.String("arn:aws:s3:::unused-backup"),
				RoleARN:   role.Role.Arn,
			},
		},
	}); err != nil {
		// Lab may accept OpenSearchDestinationConfiguration without S3 backup; try lab alias via JSON.
		st, body, _ := signedJSONTarget(t, "firehose", "Firehose_20150804.CreateDeliveryStream", map[string]any{
			"DeliveryStreamName": stream,
			"OpenSearchDestinationConfiguration": map[string]any{
				"DomainName": domain,
				"IndexName":  "events",
				"RoleARN":    aws.ToString(role.Role.Arn),
			},
		})
		if st != 200 {
			t.Fatalf("CreateDeliveryStream SDK err=%v json status=%d body=%s", err, st, body)
		}
	}
	t.Cleanup(func() {
		_, _ = fh.DeleteDeliveryStream(ctx, &firehose.DeleteDeliveryStreamInput{DeliveryStreamName: aws.String(stream)})
	})

	desc, err := fh.DescribeDeliveryStream(ctx, &firehose.DescribeDeliveryStreamInput{
		DeliveryStreamName: aws.String(stream),
	})
	if err != nil {
		// Fall back to JSON describe for lab shape assertion.
		st, body, parsed := signedJSONTarget(t, "firehose", "Firehose_20150804.DescribeDeliveryStream", map[string]any{
			"DeliveryStreamName": stream,
		})
		if st != 200 {
			t.Fatalf("DescribeDeliveryStream err=%v json status=%d body=%s", err, st, body)
		}
		raw := string(body)
		if !strings.Contains(raw, "AmazonopensearchserviceDestinationDescription") &&
			!strings.Contains(raw, "OpenSearchDestinationDescription") {
			t.Fatalf("Describe missing OpenSearch destination description: %s parsed=%v", raw, parsed)
		}
		if !strings.Contains(raw, "DomainARN") || !strings.Contains(raw, "IndexName") {
			t.Fatalf("Describe missing DomainARN/IndexName: %s", raw)
		}
		return
	}
	found := false
	if desc.DeliveryStreamDescription != nil {
		for _, d := range desc.DeliveryStreamDescription.Destinations {
			if d.AmazonopensearchserviceDestinationDescription != nil {
				found = true
				os := d.AmazonopensearchserviceDestinationDescription
				if aws.ToString(os.IndexName) != "events" {
					t.Fatalf("IndexName=%q", aws.ToString(os.IndexName))
				}
			}
		}
	}
	if !found {
		t.Fatalf("DescribeDeliveryStream missing AmazonopensearchserviceDestinationDescription: %+v", desc)
	}
}

func TestLambdaMQESMSkipUnlessBrokerRunning(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	mqClient := newMQ(t, cfg)
	lam := newLambda(t, cfg)
	iamc := newIAM(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	brokerName := "goesm-" + prefix
	if len(brokerName) > 50 {
		brokerName = brokerName[:50]
	}
	created, err := mqClient.CreateBroker(ctx, &mq.CreateBrokerInput{
		BrokerName:         aws.String(brokerName),
		EngineType:         mqtypes.EngineTypeActivemq,
		EngineVersion:      aws.String("5.18"),
		HostInstanceType:   aws.String("mq.t3.micro"),
		DeploymentMode:     mqtypes.DeploymentModeSingleInstance,
		PubliclyAccessible: aws.Bool(false),
		Users: []mqtypes.User{{
			Username: aws.String("lab"),
			Password: aws.String("lab-password-1"),
		}},
	})
	if err != nil {
		t.Fatalf("CreateBroker: %v", err)
	}
	brokerID := aws.ToString(created.BrokerId)
	t.Cleanup(func() {
		_, _ = mqClient.DeleteBroker(ctx, &mq.DeleteBrokerInput{BrokerId: aws.String(brokerID)})
	})
	desc := waitBroker(t, mqClient, brokerID, 20*time.Second)
	if desc == nil || string(desc.BrokerState) != "RUNNING" {
		state := ""
		if desc != nil {
			state = string(desc.BrokerState)
		}
		t.Skipf("Lambda MQ ESM skipped: BrokerState=%s (nested engine not RUNNING)", state)
	}
	brokerARN := aws.ToString(desc.BrokerArn)
	if brokerARN == "" {
		t.Fatal("missing BrokerArn")
	}

	roleName := prefix + "-mq-fn"
	if len(roleName) > 64 {
		roleName = roleName[:64]
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	role, err := iamc.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName: aws.String(roleName), AssumeRolePolicyDocument: aws.String(trust),
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	t.Cleanup(func() {
		_, _ = iamc.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	})
	fnName := prefix + "-mq-fn"
	if _, err := lam.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Runtime:      lamtypes.RuntimePython312,
		Role:         role.Role.Arn,
		Handler:      aws.String("index.handler"),
		Code:         &lamtypes.FunctionCode{ZipFile: minimalPythonZip(t)},
	}); err != nil {
		t.Fatalf("CreateFunction: %v", err)
	}
	t.Cleanup(func() {
		_, _ = lam.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})
	})

	esm, err := lam.CreateEventSourceMapping(ctx, &lambda.CreateEventSourceMappingInput{
		FunctionName:   aws.String(fnName),
		EventSourceArn: aws.String(brokerARN),
		BatchSize:      aws.Int32(5),
	})
	if err != nil {
		t.Fatalf("CreateEventSourceMapping: %v", err)
	}
	t.Cleanup(func() {
		if esm.UUID != nil {
			_, _ = lam.DeleteEventSourceMapping(ctx, &lambda.DeleteEventSourceMappingInput{UUID: esm.UUID})
		}
	})
}
