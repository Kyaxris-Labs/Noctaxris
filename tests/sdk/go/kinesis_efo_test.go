package sdk_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"
)

func TestKinesisEFOAndUpdateShardCount(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newKinesis(t, cfg)
	ctx := context.Background()

	stream := uniquePrefix(t) + "-kinesis"
	_, err := client.CreateStream(ctx, &kinesis.CreateStreamInput{
		StreamName: aws.String(stream),
		ShardCount: aws.Int32(1),
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteStream(ctx, &kinesis.DeleteStreamInput{StreamName: aws.String(stream)})
	})

	desc, err := client.DescribeStream(ctx, &kinesis.DescribeStreamInput{StreamName: aws.String(stream)})
	if err != nil || desc.StreamDescription == nil || desc.StreamDescription.StreamARN == nil {
		t.Fatalf("DescribeStream: %v %+v", err, desc)
	}
	streamARN := *desc.StreamDescription.StreamARN

	reg, err := client.RegisterStreamConsumer(ctx, &kinesis.RegisterStreamConsumerInput{
		StreamARN:    aws.String(streamARN),
		ConsumerName: aws.String("lab-consumer"),
	})
	if err != nil {
		t.Fatalf("RegisterStreamConsumer: %v", err)
	}
	if reg.Consumer == nil || reg.Consumer.ConsumerARN == nil {
		t.Fatalf("consumer missing: %+v", reg.Consumer)
	}

	_, err = client.UpdateShardCount(ctx, &kinesis.UpdateShardCountInput{
		StreamName:       aws.String(stream),
		TargetShardCount: aws.Int32(2),
		ScalingType:      types.ScalingTypeUniformScaling,
	})
	if err != nil {
		t.Fatalf("UpdateShardCount: %v", err)
	}
	after, err := client.DescribeStream(ctx, &kinesis.DescribeStreamInput{StreamName: aws.String(stream)})
	if err != nil {
		t.Fatalf("DescribeStream after scale: %v", err)
	}
	if len(after.StreamDescription.Shards) != 2 {
		t.Fatalf("shards=%d want 2", len(after.StreamDescription.Shards))
	}

	_, err = client.DeregisterStreamConsumer(ctx, &kinesis.DeregisterStreamConsumerInput{
		ConsumerARN: reg.Consumer.ConsumerARN,
	})
	if err != nil {
		t.Fatalf("DeregisterStreamConsumer: %v", err)
	}
}
