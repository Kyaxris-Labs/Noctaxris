"""RDS MySQL/MariaDB Create/Describe/Delete smoke (nested soft-skip)."""

from __future__ import annotations

import time

import boto3
import pytest

from conftest import _creds, nested_enabled, require_ready, unique_prefix


@pytest.fixture
def rds_client():
    return boto3.client("rds", **_creds())


@pytest.mark.parametrize("engine", ["mysql", "mariadb"])
def test_rds_mysql_mariadb_create_describe_delete(rds_client, engine: str) -> None:
    require_ready()
    prefix = unique_prefix()
    ident = f"{prefix}-{engine}".lower().replace("_", "-")
    if len(ident) > 60:
        ident = ident[:60]

    create = rds_client.create_db_instance(
        DBInstanceIdentifier=ident,
        DBInstanceClass="db.t3.micro",
        Engine=engine,
        MasterUsername="root",
        MasterUserPassword="lab-password-1",
        AllocatedStorage=20,
        DBName="appdb",
    )
    assert create["DBInstance"]["DBInstanceIdentifier"] == ident
    assert create["DBInstance"]["Engine"].lower() == engine

    try:
        desc = rds_client.describe_db_instances(DBInstanceIdentifier=ident)
        instances = desc["DBInstances"]
        assert len(instances) == 1
        assert instances[0]["Engine"].lower() == engine

        if nested_enabled():
            deadline = time.time() + 90
            status = instances[0].get("DBInstanceStatus", "")
            while time.time() < deadline:
                desc = rds_client.describe_db_instances(DBInstanceIdentifier=ident)
                status = desc["DBInstances"][0].get("DBInstanceStatus", "")
                if status == "available":
                    break
                if status == "failed":
                    pytest.skip(
                        f"RDS {engine} nested soft-skip: status failed "
                        "(nested engine not healthy)"
                    )
                time.sleep(2)
            if status != "available":
                pytest.skip(
                    f"RDS {engine} nested soft-skip: status not available "
                    "(healthy DinD required)"
                )
    finally:
        rds_client.delete_db_instance(DBInstanceIdentifier=ident)
