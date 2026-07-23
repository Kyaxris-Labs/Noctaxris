TRUST = (
    '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
    '"Principal":{"Service":"lambda.amazonaws.com"},'
    '"Action":"sts:AssumeRole"}]}'
)


def test_iam_user_and_role_round_trip(iam_client, unique_prefix):
    user_name = f"{unique_prefix}-user"
    role_name = f"{unique_prefix}-role"

    iam_client.create_user(UserName=user_name)
    try:
        user = iam_client.get_user(UserName=user_name)
        assert user["User"]["UserName"] == user_name

        iam_client.create_role(
            RoleName=role_name,
            AssumeRolePolicyDocument=TRUST,
        )
        try:
            role = iam_client.get_role(RoleName=role_name)
            assert role["Role"].get("Arn"), "GetRole missing ARN"
            iam_client.delete_role(RoleName=role_name)
        finally:
            try:
                iam_client.delete_role(RoleName=role_name)
            except Exception:
                pass

        iam_client.delete_user(UserName=user_name)
    finally:
        try:
            iam_client.delete_user(UserName=user_name)
        except Exception:
            pass
