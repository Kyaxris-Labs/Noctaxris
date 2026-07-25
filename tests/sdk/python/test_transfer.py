"""Transfer Family CreateServer/User + lab PutFile/GetFile."""

from __future__ import annotations

import base64

from conftest import json_target


def test_transfer_server_user_lab_files(transfer_client, unique_prefix):
    user = "alice"
    path = "docs/readme.txt"
    payload = "hello-transfer-py"
    server_id = None
    try:
        created = transfer_client.create_server(Protocols=["SFTP"])
        server_id = created["ServerId"]
        assert server_id

        desc = transfer_client.describe_server(ServerId=server_id)
        assert desc["Server"]["State"] == "ONLINE"

        transfer_client.create_user(ServerId=server_id, UserName=user)

        put = json_target(
            "TransferService.PutFile",
            "transfer",
            {
                "ServerId": server_id,
                "UserName": user,
                "Path": path,
                "Body": payload,
            },
        )
        assert put is not None

        got = json_target(
            "TransferService.GetFile",
            "transfer",
            {"ServerId": server_id, "UserName": user, "Path": path},
        )
        raw = base64.b64decode(got["BodyBase64"])
        assert raw.decode("utf-8") == payload
    finally:
        if server_id:
            try:
                transfer_client.delete_user(ServerId=server_id, UserName=user)
            except Exception:
                pass
            try:
                transfer_client.delete_server(ServerId=server_id)
            except Exception:
                pass
