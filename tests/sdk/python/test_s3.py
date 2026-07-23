import pytest


def test_s3_bucket_object_round_trip(s3_client, unique_prefix):
    bucket = f"{unique_prefix}-bucket".lower()
    key = "hello.txt"
    body = b"noctaxris-sdk-s3"

    s3_client.create_bucket(Bucket=bucket)
    try:
        names = [b["Name"] for b in s3_client.list_buckets().get("Buckets", [])]
        assert bucket in names, f"ListBuckets missing {bucket}"

        s3_client.put_object(Bucket=bucket, Key=key, Body=body)
        got = s3_client.get_object(Bucket=bucket, Key=key)["Body"].read()
        assert got == body

        s3_client.delete_object(Bucket=bucket, Key=key)
        with pytest.raises(Exception):
            s3_client.get_object(Bucket=bucket, Key=key)
    finally:
        try:
            s3_client.delete_object(Bucket=bucket, Key=key)
        except Exception:
            pass
        try:
            s3_client.delete_bucket(Bucket=bucket)
        except Exception:
            pass
