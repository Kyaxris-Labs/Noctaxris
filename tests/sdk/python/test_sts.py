def test_sts_get_caller_identity(sts_client):
    out = sts_client.get_caller_identity()
    assert out.get("Account"), "expected Account"
    assert out.get("Arn"), "expected Arn"
    assert out.get("UserId"), "expected UserId"
