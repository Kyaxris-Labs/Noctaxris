"""AppSync serviceRoleArn + multi CreateResolver (skip GraphQL without NESTED)."""

from __future__ import annotations

from conftest import (
    APPSYNC_TRUST,
    LAMBDA_TRUST,
    json_target,
    minimal_python_zip,
)


def test_appsync_passrole_multi_resolver(
    iam_client, lambda_client, account_id, unique_prefix
):
    role_name = f"{unique_prefix}-appsync-role"
    fn_hello = f"{unique_prefix}-hello"
    fn_world = f"{unique_prefix}-world"
    api_id = None

    role = iam_client.create_role(
        RoleName=role_name,
        AssumeRolePolicyDocument=APPSYNC_TRUST,
    )
    role_arn = role["Role"]["Arn"]
    iam_client.put_role_policy(
        RoleName=role_name,
        PolicyName="invoke",
        PolicyDocument=(
            '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
            '"Action":"lambda:InvokeFunction","Resource":"*"}]}'
        ),
    )

    exec_role = f"{unique_prefix}-lam-exec"
    exec = iam_client.create_role(
        RoleName=exec_role,
        AssumeRolePolicyDocument=LAMBDA_TRUST,
    )
    exec_arn = exec["Role"]["Arn"]

    try:
        for name in (fn_hello, fn_world):
            lambda_client.create_function(
                FunctionName=name,
                Runtime="python3.12",
                Role=exec_arn,
                Handler="index.handler",
                Code={"ZipFile": minimal_python_zip()},
            )

        api = json_target(
            "AWSAppSync.CreateGraphqlApi",
            "appsync",
            {
                "name": f"{unique_prefix}-gql",
                "authenticationType": "API_KEY",
            },
        )
        api_id = api["graphqlApi"]["apiId"]

        json_target(
            "AWSAppSync.StartSchemaCreation",
            "appsync",
            {
                "apiId": api_id,
                "definition": "type Query { hello: String world: String }",
            },
        )

        hello_arn = (
            f"arn:aws:lambda:us-east-1:{account_id}:function:{fn_hello}"
        )
        world_arn = (
            f"arn:aws:lambda:us-east-1:{account_id}:function:{fn_world}"
        )

        ds = json_target(
            "AWSAppSync.CreateDataSource",
            "appsync",
            {
                "apiId": api_id,
                "name": "HelloDS",
                "type": "AWS_LAMBDA",
                "serviceRoleArn": role_arn,
                "lambdaConfig": {"lambdaFunctionArn": hello_arn},
            },
        )
        assert role_arn in str(ds)

        json_target(
            "AWSAppSync.CreateDataSource",
            "appsync",
            {
                "apiId": api_id,
                "name": "WorldDS",
                "type": "AWS_LAMBDA",
                "serviceRoleArn": role_arn,
                "lambdaConfig": {"lambdaFunctionArn": world_arn},
            },
        )

        json_target(
            "AWSAppSync.CreateResolver",
            "appsync",
            {
                "apiId": api_id,
                "typeName": "Query",
                "fieldName": "hello",
                "dataSourceName": "HelloDS",
            },
        )
        json_target(
            "AWSAppSync.CreateResolver",
            "appsync",
            {
                "apiId": api_id,
                "typeName": "Query",
                "fieldName": "world",
                "dataSourceName": "WorldDS",
            },
        )

        # GraphQL Invoke needs DinD; control-plane PassRole + resolvers are enough.
    finally:
        if api_id:
            try:
                json_target(
                    "AWSAppSync.DeleteGraphqlApi",
                    "appsync",
                    {"apiId": api_id},
                )
            except Exception:
                pass
        for name in (fn_hello, fn_world):
            try:
                lambda_client.delete_function(FunctionName=name)
            except Exception:
                pass
        try:
            iam_client.delete_role_policy(RoleName=role_name, PolicyName="invoke")
        except Exception:
            pass
        try:
            iam_client.delete_role(RoleName=role_name)
        except Exception:
            pass
        try:
            iam_client.delete_role(RoleName=exec_role)
        except Exception:
            pass
