package sdk_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestDynamoDBTableItemRoundTrip(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newDDB(t, cfg)
	ctx := context.Background()

	table := uniquePrefix(t) + "-ddb"
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(table),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(table)})
	})

	desc, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(table)})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}
	if desc.Table == nil || desc.Table.TableName == nil || *desc.Table.TableName != table {
		t.Fatalf("DescribeTable unexpected: %+v", desc.Table)
	}

	_, err = client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(table),
		Item: map[string]types.AttributeValue{
			"pk":   &types.AttributeValueMemberS{Value: "1"},
			"data": &types.AttributeValueMemberS{Value: "sdk-ddb"},
		},
	})
	if err != nil {
		t.Fatalf("PutItem: %v", err)
	}

	got, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(table),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "1"},
		},
	})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	data, ok := got.Item["data"].(*types.AttributeValueMemberS)
	if !ok || data.Value != "sdk-ddb" {
		t.Fatalf("GetItem data=%v", got.Item["data"])
	}

	_, err = client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(table),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "1"},
		},
	})
	if err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}

	_, err = client.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(table)})
	if err != nil {
		t.Fatalf("DeleteTable: %v", err)
	}
}
