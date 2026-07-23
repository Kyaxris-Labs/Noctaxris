def test_ssm_string_parameter_round_trip(ssm_client, unique_prefix):
    name = f"/lab/{unique_prefix}/config"
    value = f"plain-{unique_prefix}"
    ssm_client.put_parameter(Name=name, Type="String", Value=value)
    try:
        got = ssm_client.get_parameter(Name=name)
        assert got["Parameter"]["Value"] == value
        ssm_client.delete_parameter(Name=name)
    finally:
        try:
            ssm_client.delete_parameter(Name=name)
        except Exception:
            pass
