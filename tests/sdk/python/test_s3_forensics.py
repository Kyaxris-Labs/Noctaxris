"""S3 Object Lock lite, bucket logging, and versioning delete markers."""

from __future__ import annotations

from datetime import datetime, timedelta, timezone

import botocore.exceptions
import pytest


def _empty_and_delete_bucket(s3_client, bucket: str) -> None:
    try:
        vers = s3_client.list_object_versions(Bucket=bucket)
        for v in vers.get("Versions") or []:
            try:
                s3_client.delete_object(
                    Bucket=bucket,
                    Key=v["Key"],
                    VersionId=v.get("VersionId"),
                    BypassGovernanceRetention=True,
                )
            except Exception:
                pass
        for m in vers.get("DeleteMarkers") or []:
            try:
                s3_client.delete_object(
                    Bucket=bucket,
                    Key=m["Key"],
                    VersionId=m.get("VersionId"),
                )
            except Exception:
                pass
    except Exception:
        pass
    try:
        listed = s3_client.list_objects_v2(Bucket=bucket)
        for obj in listed.get("Contents") or []:
            try:
                s3_client.delete_object(
                    Bucket=bucket,
                    Key=obj["Key"],
                    BypassGovernanceRetention=True,
                )
            except Exception:
                pass
    except Exception:
        pass
    try:
        s3_client.delete_bucket(Bucket=bucket)
    except Exception:
        pass


def test_s3_forensics_object_lock_lite(s3_client, unique_prefix):
    bucket = f"{unique_prefix}-lock".lower()
    key = "locked.txt"
    s3_client.create_bucket(Bucket=bucket, ObjectLockEnabledForBucket=True)
    try:
        retain = datetime.now(timezone.utc) + timedelta(hours=24)
        s3_client.put_object(
            Bucket=bucket,
            Key=key,
            Body=b"secret",
            ObjectLockMode="GOVERNANCE",
            ObjectLockRetainUntilDate=retain,
        )
        with pytest.raises(botocore.exceptions.ClientError) as exc:
            s3_client.delete_object(Bucket=bucket, Key=key)
        assert "AccessDenied" in str(exc.value)

        s3_client.delete_object(
            Bucket=bucket,
            Key=key,
            BypassGovernanceRetention=True,
        )
    finally:
        _empty_and_delete_bucket(s3_client, bucket)


def test_s3_forensics_bucket_logging(s3_client, unique_prefix):
    src = f"{unique_prefix}-src".lower()
    logs = f"{unique_prefix}-logs".lower()
    key = "a.txt"
    s3_client.create_bucket(Bucket=src)
    s3_client.create_bucket(Bucket=logs)
    try:
        s3_client.put_bucket_logging(
            Bucket=src,
            BucketLoggingStatus={
                "LoggingEnabled": {
                    "TargetBucket": logs,
                    "TargetPrefix": "s3/",
                }
            },
        )
        got = s3_client.get_bucket_logging(Bucket=src)
        assert got["LoggingEnabled"]["TargetBucket"] == logs

        s3_client.put_object(Bucket=src, Key=key, Body=b"abc")
        listed = s3_client.list_objects_v2(Bucket=logs, Prefix="s3/")
        contents = listed.get("Contents") or []
        assert contents, f"expected access log under s3/: {contents}"
        body = s3_client.get_object(Bucket=logs, Key=contents[0]["Key"])[
            "Body"
        ].read()
        line = body.decode("utf-8")
        assert "REST.PUT.OBJECT" in line
        assert key in line
    finally:
        _empty_and_delete_bucket(s3_client, src)
        _empty_and_delete_bucket(s3_client, logs)


def test_s3_forensics_versioning_delete_markers(s3_client, unique_prefix):
    bucket = f"{unique_prefix}-ver".lower()
    key = "obj.txt"
    s3_client.create_bucket(Bucket=bucket)
    try:
        s3_client.put_bucket_versioning(
            Bucket=bucket,
            VersioningConfiguration={"Status": "Enabled"},
        )
        put = s3_client.put_object(Bucket=bucket, Key=key, Body=b"payload")
        assert put.get("VersionId"), "PutObject missing VersionId"

        deleted = s3_client.delete_object(Bucket=bucket, Key=key)
        assert deleted.get("DeleteMarker") is True

        listed = s3_client.list_object_versions(Bucket=bucket, Prefix=key)
        assert listed.get("Versions"), "missing object version"
        assert listed.get("DeleteMarkers"), "missing delete marker"
    finally:
        _empty_and_delete_bucket(s3_client, bucket)
