def test_dynamodb_lsi_query(dynamodb_client, unique_prefix):
    table = f"{unique_prefix}-lsi"
    dynamodb_client.create_table(
        TableName=table,
        AttributeDefinitions=[
            {"AttributeName": "pk", "AttributeType": "S"},
            {"AttributeName": "sk", "AttributeType": "S"},
            {"AttributeName": "status", "AttributeType": "S"},
        ],
        KeySchema=[
            {"AttributeName": "pk", "KeyType": "HASH"},
            {"AttributeName": "sk", "KeyType": "RANGE"},
        ],
        LocalSecondaryIndexes=[
            {
                "IndexName": "ByStatus",
                "KeySchema": [
                    {"AttributeName": "pk", "KeyType": "HASH"},
                    {"AttributeName": "status", "KeyType": "RANGE"},
                ],
                "Projection": {"ProjectionType": "ALL"},
            }
        ],
        BillingMode="PAY_PER_REQUEST",
    )
    try:
        desc = dynamodb_client.describe_table(TableName=table)
        assert len(desc["Table"]["LocalSecondaryIndexes"]) == 1

        dynamodb_client.put_item(
            TableName=table,
            Item={
                "pk": {"S": "u1"},
                "sk": {"S": "o1"},
                "status": {"S": "OPEN"},
            },
        )
        out = dynamodb_client.query(
            TableName=table,
            IndexName="ByStatus",
            KeyConditionExpression="pk = :pk AND #s = :st",
            ExpressionAttributeNames={"#s": "status"},
            ExpressionAttributeValues={
                ":pk": {"S": "u1"},
                ":st": {"S": "OPEN"},
            },
        )
        assert len(out["Items"]) == 1
        assert out["Items"][0]["pk"]["S"] == "u1"
        assert out["Items"][0]["status"]["S"] == "OPEN"
    finally:
        try:
            dynamodb_client.delete_table(TableName=table)
        except Exception:
            pass


def test_dynamodb_partiql_statement(dynamodb_client, unique_prefix):
    table = f"{unique_prefix}-partiql"
    dynamodb_client.create_table(
        TableName=table,
        AttributeDefinitions=[{"AttributeName": "pk", "AttributeType": "S"}],
        KeySchema=[{"AttributeName": "pk", "KeyType": "HASH"}],
        BillingMode="PAY_PER_REQUEST",
    )
    try:
        dynamodb_client.execute_statement(
            Statement=f'INSERT INTO "{table}" VALUE {{\'pk\':?,\'data\':?}}',
            Parameters=[{"S": "1"}, {"S": "partiql"}],
        )
        sel = dynamodb_client.execute_statement(
            Statement=f'SELECT * FROM "{table}" WHERE pk=?',
            Parameters=[{"S": "1"}],
        )
        assert len(sel["Items"]) == 1
        assert sel["Items"][0]["data"]["S"] == "partiql"
    finally:
        try:
            dynamodb_client.delete_table(TableName=table)
        except Exception:
            pass
