"""Neptune CreateDBCluster; soft-skip when not available."""

from __future__ import annotations

import time
import xml.etree.ElementTree as ET

import pytest
from conftest import form_action, json_target


def _neptune_form(params: dict[str, str]) -> bytes:
    return form_action("neptune", params)


def _status_from_xml(raw: bytes) -> str:
    root = ET.fromstring(raw)
    for el in root.iter():
        if el.tag.endswith("Status") and el.text:
            return el.text.strip()
    return ""


def test_neptune_create_skip_without_engine(unique_prefix):
    cluster_id = f"{unique_prefix}-nep".replace("_", "-")[:40]
    try:
        _neptune_form(
            {
                "Action": "CreateDBCluster",
                "Version": "2014-10-31",
                "DBClusterIdentifier": cluster_id,
                "Engine": "neptune",
            }
        )
        status = ""
        for _ in range(8):
            raw = _neptune_form(
                {
                    "Action": "DescribeDBClusters",
                    "Version": "2014-10-31",
                    "DBClusterIdentifier": cluster_id,
                }
            )
            status = _status_from_xml(raw)
            if status in ("available", "failed"):
                break
            time.sleep(0.5)
        if status != "available":
            pytest.skip(f"Neptune engine not available (status={status!r})")
    finally:
        try:
            _neptune_form(
                {
                    "Action": "DeleteDBCluster",
                    "Version": "2014-10-31",
                    "DBClusterIdentifier": cluster_id,
                }
            )
        except Exception:
            pass


def test_msk_create_skip_without_engine(unique_prefix):
    name = f"{unique_prefix}-msk".replace("_", "-")[:40]
    arn = None
    try:
        created = json_target(
            "Kafka_1.0.CreateCluster",
            "kafka",
            {
                "ClusterName": name,
                "KafkaVersion": "3.6.0",
                "NumberOfBrokerNodes": 1,
                "BrokerNodeGroupInfo": {
                    "InstanceType": "kafka.m5.large",
                    "ClientSubnets": ["subnet-1"],
                },
            },
        )
        arn = created.get("ClusterArn")
        assert arn
        state = ""
        for _ in range(8):
            desc = json_target(
                "Kafka_1.0.DescribeCluster",
                "kafka",
                {"ClusterArn": arn},
            )
            info = desc.get("ClusterInfo") or {}
            state = info.get("State") or ""
            if state in ("ACTIVE", "FAILED"):
                break
            time.sleep(0.5)
        if state != "ACTIVE":
            pytest.skip(f"MSK engine not ACTIVE (state={state!r})")
        boot = json_target(
            "Kafka_1.0.GetBootstrapBrokers",
            "kafka",
            {"ClusterArn": arn},
        )
        brokers = boot.get("BootstrapBrokerString") or ""
        assert "noctaxris-msk-" in brokers
    finally:
        if arn:
            try:
                json_target(
                    "Kafka_1.0.DeleteCluster",
                    "kafka",
                    {"ClusterArn": arn},
                )
            except Exception:
                pass
