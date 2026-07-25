"""Glue crawler StartCrawler + GetTable from CSV."""

from __future__ import annotations


def test_glue_crawler_start_creates_table(s3_client, glue_client, unique_prefix):
    db = f"{unique_prefix}_db".replace("-", "_")
    crawler = f"{unique_prefix}-crawler"
    bucket = f"{unique_prefix}-glue".lower()
    key = "data/people.csv"
    csv = b"id,name\n1,alice\n"

    s3_client.create_bucket(Bucket=bucket)
    s3_client.put_object(Bucket=bucket, Key=key, Body=csv)
    try:
        glue_client.create_database(DatabaseInput={"Name": db})
        glue_client.create_crawler(
            Name=crawler,
            Role="",
            DatabaseName=db,
            Targets={"S3Targets": [{"Path": f"s3://{bucket}/data/"}]},
        )
        glue_client.start_crawler(Name=crawler)
        cr = glue_client.get_crawler(Name=crawler)["Crawler"]
        assert cr["State"] == "READY"

        table = glue_client.get_table(DatabaseName=db, Name="people")["Table"]
        cols = [c["Name"] for c in table["StorageDescriptor"]["Columns"]]
        assert cols == ["id", "name"] or "id" in cols
    finally:
        try:
            glue_client.delete_crawler(Name=crawler)
        except Exception:
            pass
        try:
            glue_client.delete_table(DatabaseName=db, Name="people")
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
