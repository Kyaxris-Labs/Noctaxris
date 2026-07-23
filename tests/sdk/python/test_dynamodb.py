def test_dynamodb_table_item_round_trip(dynamodb_client, unique_prefix):
    table = f"{unique_prefix}-ddb"
    dynamodb_client.create_table(
        TableName=table,
        AttributeDefinitions=[{"AttributeName": "pk", "AttributeType": "S"}],
        KeySchema=[{"AttributeName": "pk", "KeyType": "HASH"}],
        BillingMode="PAY_PER_REQUEST",
    )
    try:
        desc = dynamodb_client.describe_table(TableName=table)
        assert desc["Table"]["TableName"] == table

        dynamodb_client.put_item(
            TableName=table,
            Item={"pk": {"S": "1"}, "data": {"S": "sdk-ddb"}},
        )
        got = dynamodb_client.get_item(TableName=table, Key={"pk": {"S": "1"}})
        assert got["Item"]["data"]["S"] == "sdk-ddb"

        dynamodb_client.delete_item(TableName=table, Key={"pk": {"S": "1"}})
        dynamodb_client.delete_table(TableName=table)
    finally:
        try:
            dynamodb_client.delete_table(TableName=table)
        except Exception:
            pass
