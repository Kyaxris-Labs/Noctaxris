def test_secrets_manager_round_trip(secretsmanager_client, unique_prefix):
    name = f"lab/{unique_prefix}/api-key"
    secret = f'{{"apiKey":"key-{unique_prefix}"}}'
    secretsmanager_client.create_secret(Name=name, SecretString=secret)
    try:
        got = secretsmanager_client.get_secret_value(SecretId=name)
        assert got["SecretString"] == secret
        secretsmanager_client.delete_secret(
            SecretId=name, ForceDeleteWithoutRecovery=True
        )
    finally:
        try:
            secretsmanager_client.delete_secret(
                SecretId=name, ForceDeleteWithoutRecovery=True
            )
        except Exception:
            pass
