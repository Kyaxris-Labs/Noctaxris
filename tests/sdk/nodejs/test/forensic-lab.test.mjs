import { test } from "node:test";
import assert from "node:assert/strict";
import { requireReady, signedJsonTarget } from "../lib/helpers.mjs";

test("Lab forensics SetClock and BulkSeed crypto-mining", async (t) => {
  if (!(await requireReady(t))) return;
  if (process.env.NOCTAXRIS_LAB_FORENSICS !== "1") {
    t.skip(
      "set NOCTAXRIS_LAB_FORENSICS=1 on the API process for NoctaxrisLab SetClock/BulkSeed",
    );
    return;
  }

  const fixed = "2026-07-20T12:00:00Z";
  const setClock = await signedJsonTarget("noctaxris", "NoctaxrisLab.SetClock", {
    FixedTime: fixed,
  });
  assert.equal(setClock.status, 200, setClock.body);
  assert.equal(
    setClock.json?.ClockTime,
    fixed,
    `SetClock ClockTime=${setClock.json?.ClockTime} body=${setClock.body}`,
  );

  const seed = await signedJsonTarget("noctaxris", "NoctaxrisLab.BulkSeed", {
    ScenarioId: "crypto-mining",
    IncludeGuardDuty: true,
  });
  assert.equal(seed.status, 200, seed.body);
  assert.equal(seed.json?.ScenarioId, "crypto-mining", seed.body);
  assert.ok(
    (seed.json?.CloudTrailEventCount || 0) >= 1,
    `BulkSeed CloudTrailEventCount=${seed.json?.CloudTrailEventCount} body=${seed.body}`,
  );
  assert.ok(
    (seed.json?.GuardDutyFindingCount || 0) >= 1,
    `BulkSeed GuardDutyFindingCount=${seed.json?.GuardDutyFindingCount} body=${seed.body}`,
  );
});
