"""MemoryDB Create/Describe/Delete; soft-skip when nested engine not available."""

from __future__ import annotations

import time

import pytest
from conftest import json_target, unique_prefix


def test_memorydb_create_describe_delete_soft_skip_nested(unique_prefix):
    name = f"sdk-mdb-{unique_prefix}"[:40]
    try:
        created = json_target(
            "AmazonMemoryDB.CreateCluster",
            "memorydb",
            {
                "ClusterName": name,
                "NodeType": "db.t4g.small",
                "ACLName": "open-access",
                "Engine": "redis",
            },
        )
        cluster = created.get("Cluster") or {}
        assert cluster.get("Name") == name
        ep = cluster.get("ClusterEndpoint") or {}
        assert "memorydb.noctaxris.internal" in (ep.get("Address") or "")

        status = ""
        for _ in range(16):
            desc = json_target(
                "AmazonMemoryDB.DescribeClusters",
                "memorydb",
                {"ClusterName": name},
            )
            clusters = desc.get("Clusters") or []
            assert clusters
            status = clusters[0].get("Status") or ""
            if status in ("available", "failed"):
                break
            time.sleep(0.5)

        if status != "available":
            pytest.skip(
                f"MemoryDB nested soft-skip: Status={status!r} "
                "(Compose noctaxris-engine for available)"
            )
    finally:
        try:
            json_target(
                "AmazonMemoryDB.DeleteCluster",
                "memorydb",
                {"ClusterName": name},
            )
        except Exception:
            pass
