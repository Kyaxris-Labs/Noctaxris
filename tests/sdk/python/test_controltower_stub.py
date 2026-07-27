"""Control Tower honest stub: empty ListLandingZones; GetLandingZone not found."""

from __future__ import annotations

import json

from conftest import endpoint, signed_request


def _json_target_status(target: str, service: str, payload: dict | None = None):
    body = json.dumps(payload or {}).encode("utf-8")
    status, raw, _ = signed_request(
        "POST",
        endpoint() + "/",
        service=service,
        body=body,
        headers={"X-Amz-Target": target},
        content_type="application/x-amz-json-1.1",
    )
    parsed = json.loads(raw.decode("utf-8")) if raw else {}
    return status, raw, parsed


def test_controltower_list_empty_get_not_found():
    status, raw, listed = _json_target_status(
        "ControlTower.ListLandingZones",
        "controltower",
        {},
    )
    assert status == 200, f"ListLandingZones status={status} body={raw!r}"
    zones = listed.get("landingZones") or []
    assert zones == [], f"landingZones={zones!r} want empty"

    status, raw, _ = _json_target_status(
        "ControlTower.GetLandingZone",
        "controltower",
        {
            "landingZoneIdentifier": (
                "arn:aws:controltower:us-east-1:000000000001:landingzone/lz-deadbeef"
            )
        },
    )
    assert status == 400, f"GetLandingZone status={status} body={raw!r} want 400"
    assert b"ResourceNotFoundException" in raw, f"body={raw!r}"
