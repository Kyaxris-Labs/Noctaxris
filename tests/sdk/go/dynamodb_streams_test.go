package sdk_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodbstreams"
	streamtypes "github.com/aws/aws-sdk-go-v2/service/dynamodbstreams/types"
)

func newDDBStreams(t *testing.T, cfg aws.Config) *dynamodbstreams.Client {
	t.Helper()
	return dynamodbstreams.NewFromConfig(cfg, func(o *dynamodbstreams.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func TestDynamoDBTransactWriteOldImageStream(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	ddb := newDDB(t, cfg)
	streams := newDDBStreams(t, cfg)
	ctx := context.Background()
	table := uniquePrefix(t) + "-oldimg"

	createOut, err := ddb.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(table),
		AttributeDefinitions: []ddbtypes.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: ddbtypes.ScalarAttributeTypeS},
		},
		KeySchema: []ddbtypes.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: ddbtypes.KeyTypeHash},
		},
		BillingMode: ddbtypes.BillingModePayPerRequest,
		StreamSpecification: &ddbtypes.StreamSpecification{
			StreamEnabled:  aws.Bool(true),
			StreamViewType: ddbtypes.StreamViewTypeOldImage,
		},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ddb.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(table)})
	})
	streamARN := aws.ToString(createOut.TableDescription.LatestStreamArn)
	if streamARN == "" {
		t.Fatal("CreateTable missing LatestStreamArn")
	}

	_, err = ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(table),
		Item: map[string]ddbtypes.AttributeValue{
			"pk":   &ddbtypes.AttributeValueMemberS{Value: "1"},
			"attr": &ddbtypes.AttributeValueMemberS{Value: "v1"},
		},
	})
	if err != nil {
		t.Fatalf("PutItem: %v", err)
	}

	_, err = ddb.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []ddbtypes.TransactWriteItem{
			{
				Update: &ddbtypes.Update{
					TableName: aws.String(table),
					Key: map[string]ddbtypes.AttributeValue{
						"pk": &ddbtypes.AttributeValueMemberS{Value: "1"},
					},
					UpdateExpression: aws.String("SET #a = :v"),
					ExpressionAttributeNames: map[string]string{
						"#a": "attr",
					},
					ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
						":v": &ddbtypes.AttributeValueMemberS{Value: "v2"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("TransactWriteItems Update: %v", err)
	}

	desc, err := streams.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
		StreamArn: aws.String(streamARN),
	})
	if err != nil {
		t.Fatalf("DescribeStream: %v", err)
	}
	if desc.StreamDescription == nil || len(desc.StreamDescription.Shards) == 0 {
		t.Fatalf("DescribeStream missing shards: %+v", desc.StreamDescription)
	}
	shardID := aws.ToString(desc.StreamDescription.Shards[0].ShardId)
	if shardID == "" {
		t.Fatal("empty ShardId")
	}

	itOut, err := streams.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
		StreamArn:          aws.String(streamARN),
		ShardId:            aws.String(shardID),
		ShardIteratorType:  streamtypes.ShardIteratorTypeTrimHorizon,
	})
	if err != nil {
		t.Fatalf("GetShardIterator: %v", err)
	}
	iterator := aws.ToString(itOut.ShardIterator)
	if iterator == "" {
		t.Fatal("empty ShardIterator")
	}

	recsOut, err := streams.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{
		ShardIterator: aws.String(iterator),
		Limit:         aws.Int32(10),
	})
	if err != nil {
		t.Fatalf("GetRecords: %v", err)
	}

	var modify *streamtypes.Record
	for i := range recsOut.Records {
		r := &recsOut.Records[i]
		if r.EventName == streamtypes.OperationTypeModify {
			modify = r
			break
		}
	}
	if modify == nil || modify.Dynamodb == nil {
		t.Fatalf("want MODIFY stream record, got %+v", recsOut.Records)
	}
	if len(modify.Dynamodb.NewImage) != 0 {
		t.Fatalf("OLD_IMAGE must omit NewImage, got %+v", modify.Dynamodb.NewImage)
	}
	oldAttr, ok := modify.Dynamodb.OldImage["attr"].(*streamtypes.AttributeValueMemberS)
	if !ok || oldAttr.Value != "v1" {
		t.Fatalf("OldImage attr=%v want v1", modify.Dynamodb.OldImage["attr"])
	}
}
