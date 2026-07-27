"""Athena CloudTrail-shaped JSON: Records unwrap, LIKE, json_extract."""

from __future__ import annotations

import json

from conftest import json_target


def _athena_rows(query: str, database: str) -> list[list[str]]:
    started = json_target(
        "AmazonAthena.StartQueryExecution",
        "athena",
        {
            "QueryString": query,
            "QueryExecutionContext": {"Database": database},
        },
    )
    qid = started["QueryExecutionId"]
    exec_out = json_target(
        "AmazonAthena.GetQueryExecution",
        "athena",
        {"QueryExecutionId": qid},
    )
    state = exec_out["QueryExecution"]["Status"]["State"]
    assert state == "SUCCEEDED", exec_out
    results = json_target(
        "AmazonAthena.GetQueryResults",
        "athena",
        {"QueryExecutionId": qid},
    )
    rows = results.get("ResultSet", {}).get("Rows") or []
    out: list[list[str]] = []
    for row in rows:
        out.append(
            [cell.get("VarCharValue") or "" for cell in (row.get("Data") or [])]
        )
    return out


def test_athena_cloudtrail_records_like_json_extract(
    s3_client, glue_client, unique_prefix
):
    bucket = f"{unique_prefix}-athct".lower()
    db = f"{unique_prefix.replace('-', '_')}_ctdb"[:48]
    table = "events"
    key = "trail/delivery.json"
    payload = {
        "Records": [
            {
                "eventName": "AssumeRole",
                "eventID": "e1",
                "userIdentity": {"type": "IAMUser", "userName": "alice"},
            },
            {
                "eventName": "PutObject",
                "eventID": "e2",
                "userIdentity": {"type": "AWSService", "userName": "s3"},
            },
            {
                "eventName": "AssumeRoleWithSAML",
                "eventID": "e3",
                "userIdentity": {"type": "IAMUser", "userName": "bob"},
            },
        ]
    }

    s3_client.create_bucket(Bucket=bucket)
    try:
        s3_client.put_object(
            Bucket=bucket, Key=key, Body=json.dumps(payload).encode("utf-8")
        )
        glue_client.create_database(DatabaseInput={"Name": db})
        glue_client.create_table(
            DatabaseName=db,
            TableInput={
                "Name": table,
                "StorageDescriptor": {
                    "Location": f"s3://{bucket}/trail/",
                    "Columns": [
                        {"Name": "eventName", "Type": "string"},
                        {"Name": "eventID", "Type": "string"},
                        {"Name": "userIdentity", "Type": "string"},
                    ],
                    "InputFormat": "org.apache.hive.hcatalog.data.JsonSerDe",
                    "SerdeInfo": {
                        "SerializationLibrary": "org.openx.data.jsonserde.JsonSerDe"
                    },
                },
            },
        )

        unwrap = _athena_rows(
            f"SELECT eventName, eventID FROM {db}.events ORDER BY eventID", db
        )
        assert len(unwrap) == 4, unwrap
        assert unwrap[1][0] == "AssumeRole"
        assert unwrap[3][0] == "AssumeRoleWithSAML"

        like_rows = _athena_rows(
            f"SELECT eventName FROM {db}.events WHERE eventName LIKE 'Assume%'",
            db,
        )
        assert len(like_rows) == 3, like_rows
        joined = like_rows[1][0] + like_rows[2][0]
        assert "AssumeRole" in joined
        assert "AssumeRoleWithSAML" in joined

        jx_rows = _athena_rows(
            f"SELECT eventName FROM {db}.events WHERE json_extract(userIdentity, '$.type') = 'IAMUser'",
            db,
        )
        assert len(jx_rows) == 3, jx_rows
        jx_joined = jx_rows[1][0] + jx_rows[2][0]
        assert "AssumeRole" in jx_joined
        assert "AssumeRoleWithSAML" in jx_joined
    finally:
        try:
            glue_client.delete_table(DatabaseName=db, Name=table)
        except Exception:
            pass
        try:
            glue_client.delete_database(Name=db)
        except Exception:
            pass
        try:
            s3_client.delete_object(Bucket=bucket, Key=key)
        except Exception:
            pass
        try:
            s3_client.delete_bucket(Bucket=bucket)
        except Exception:
            pass
