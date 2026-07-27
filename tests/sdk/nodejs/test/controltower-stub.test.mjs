import { test } from "node:test";
import assert from "node:assert/strict";
import { requireReady, signedJsonTarget } from "../lib/helpers.mjs";

test("Control Tower ListLandingZones empty and GetLandingZone not found", async (t) => {
  if (!(await requireReady(t))) return;

  const listed = await signedJsonTarget(
    "controltower",
    "ControlTower.ListLandingZones",
    {},
  );
  assert.equal(
    listed.status,
    200,
    `ListLandingZones status=${listed.status} body=${listed.body}`,
  );
  const zones = listed.json?.landingZones || [];
  assert.equal(zones.length, 0, `landingZones=${JSON.stringify(zones)}`);

  const got = await signedJsonTarget(
    "controltower",
    "ControlTower.GetLandingZone",
    {
      landingZoneIdentifier:
        "arn:aws:controltower:us-east-1:000000000001:landingzone/lz-deadbeef",
    },
  );
  assert.equal(
    got.status,
    400,
    `GetLandingZone status=${got.status} body=${got.body} want 400`,
  );
  assert.match(
    got.body,
    /ResourceNotFoundException/,
    `GetLandingZone body=${got.body}`,
  );
});
