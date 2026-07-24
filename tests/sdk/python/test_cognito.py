import pytest
from botocore.exceptions import ClientError

from cognito_srp import SDKSRPClient


def test_cognito_user_srp_auth_round_trip(cognito_idp_client, unique_prefix):
    pool_out = cognito_idp_client.create_user_pool(PoolName=f"sdk-srp-{unique_prefix}")
    pool_id = pool_out["UserPool"]["Id"]
    assert pool_id

    try:
        client_out = cognito_idp_client.create_user_pool_client(
            UserPoolId=pool_id,
            ClientName="sdk-srp-app",
        )
        client_id = client_out["UserPoolClient"]["ClientId"]
        assert client_id

        password = "Secret1!"
        username = f"srp-{unique_prefix}"
        cognito_idp_client.admin_create_user(
            UserPoolId=pool_id,
            Username=username,
            TemporaryPassword=password,
        )

        srp = SDKSRPClient(pool_id, username, password)
        init_out = cognito_idp_client.initiate_auth(
            ClientId=client_id,
            AuthFlow="USER_SRP_AUTH",
            AuthParameters={
                "USERNAME": username,
                "SRP_A": srp.srp_a_hex(),
            },
        )
        assert init_out["ChallengeName"] == "PASSWORD_VERIFIER"
        responses = srp.password_verifier_responses(init_out["ChallengeParameters"])
        respond_out = cognito_idp_client.respond_to_auth_challenge(
            ClientId=client_id,
            ChallengeName="PASSWORD_VERIFIER",
            Session=init_out.get("Session"),
            ChallengeResponses=responses,
        )
        auth = respond_out.get("AuthenticationResult") or {}
        assert auth.get("AccessToken")
        assert auth.get("IdToken")
    finally:
        try:
            cognito_idp_client.delete_user_pool(UserPoolId=pool_id)
        except Exception:
            pass


def test_cognito_update_user_pool_lambda_config(cognito_idp_client, unique_prefix):
    pool_out = cognito_idp_client.create_user_pool(
        PoolName=f"sdk-trig-{unique_prefix}"
    )
    pool_id = pool_out["UserPool"]["Id"]
    assert pool_id

    try:
        lambda_arn = (
            f"arn:aws:lambda:us-east-1:000000000001:function:pre-signup-{unique_prefix}"
        )
        cognito_idp_client.update_user_pool(
            UserPoolId=pool_id,
            LambdaConfig={"PreSignUp": lambda_arn},
        )
        desc = cognito_idp_client.describe_user_pool(UserPoolId=pool_id)
        assert desc["UserPool"]["LambdaConfig"]["PreSignUp"] == lambda_arn
    finally:
        try:
            cognito_idp_client.delete_user_pool(UserPoolId=pool_id)
        except Exception:
            pass


def test_cognito_lambda_config_trigger_fail_closed(cognito_idp_client, unique_prefix):
    pool_out = cognito_idp_client.create_user_pool(
        PoolName=f"sdk-failtrig-{unique_prefix}"
    )
    pool_id = pool_out["UserPool"]["Id"]
    assert pool_id

    try:
        missing_arn = (
            f"arn:aws:lambda:us-east-1:000000000001:function:missing-{unique_prefix}"
        )
        cognito_idp_client.update_user_pool(
            UserPoolId=pool_id,
            LambdaConfig={"PreAuthentication": missing_arn},
        )

        app = cognito_idp_client.create_user_pool_client(
            UserPoolId=pool_id,
            ClientName="app",
        )
        client_id = app["UserPoolClient"]["ClientId"]

        password = "Secret1!"
        username = f"u-{unique_prefix}"
        cognito_idp_client.admin_create_user(
            UserPoolId=pool_id,
            Username=username,
            TemporaryPassword=password,
        )

        with pytest.raises(ClientError) as exc:
            cognito_idp_client.admin_initiate_auth(
                UserPoolId=pool_id,
                ClientId=client_id,
                AuthFlow="ADMIN_USER_PASSWORD_AUTH",
                AuthParameters={
                    "USERNAME": username,
                    "PASSWORD": password,
                },
            )
        assert "UnexpectedLambdaException" in str(exc.value)
    finally:
        try:
            cognito_idp_client.delete_user_pool(UserPoolId=pool_id)
        except Exception:
            pass
