package sdk_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestDynamoDBLSIQuery(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newDDB(t, cfg)
	ctx := context.Background()

	table := uniquePrefix(t) + "-lsi"
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(table),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("status"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
		},
		LocalSecondaryIndexes: []types.LocalSecondaryIndex{
			{
				IndexName: aws.String("ByStatus"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("status"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
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
	if desc.Table == nil || len(desc.Table.LocalSecondaryIndexes) != 1 {
		t.Fatalf("LSI missing: %+v", desc.Table)
	}

	_, err = client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(table),
		Item: map[string]types.AttributeValue{
			"pk":     &types.AttributeValueMemberS{Value: "u1"},
			"sk":     &types.AttributeValueMemberS{Value: "o1"},
			"status": &types.AttributeValueMemberS{Value: "OPEN"},
		},
	})
	if err != nil {
		t.Fatalf("PutItem: %v", err)
	}

	out, err := client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(table),
		IndexName:              aws.String("ByStatus"),
		KeyConditionExpression: aws.String("pk = :pk AND #s = :st"),
		ExpressionAttributeNames: map[string]string{
			"#s": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "u1"},
			":st": &types.AttributeValueMemberS{Value: "OPEN"},
		},
	})
	if err != nil {
		t.Fatalf("Query LSI: %v", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("items=%d", len(out.Items))
	}
	pk, ok := out.Items[0]["pk"].(*types.AttributeValueMemberS)
	if !ok || pk.Value != "u1" {
		t.Fatalf("pk=%v", out.Items[0]["pk"])
	}
	st, ok := out.Items[0]["status"].(*types.AttributeValueMemberS)
	if !ok || st.Value != "OPEN" {
		t.Fatalf("status=%v", out.Items[0]["status"])
	}
}

func TestDynamoDBPartiQLStatement(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newDDB(t, cfg)
	ctx := context.Background()

	table := uniquePrefix(t) + "-partiql"
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

	_, err = client.ExecuteStatement(ctx, &dynamodb.ExecuteStatementInput{
		Statement: aws.String("INSERT INTO \"" + table + "\" VALUE {'pk':?,'data':?}"),
		Parameters: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: "1"},
			&types.AttributeValueMemberS{Value: "partiql"},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteStatement INSERT: %v", err)
	}

	sel, err := client.ExecuteStatement(ctx, &dynamodb.ExecuteStatementInput{
		Statement: aws.String("SELECT * FROM \"" + table + "\" WHERE pk=?"),
		Parameters: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: "1"},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteStatement SELECT: %v", err)
	}
	if len(sel.Items) != 1 {
		t.Fatalf("select items=%d", len(sel.Items))
	}
	data, ok := sel.Items[0]["data"].(*types.AttributeValueMemberS)
	if !ok || data.Value != "partiql" {
		t.Fatalf("data=%v", sel.Items[0]["data"])
	}
}
