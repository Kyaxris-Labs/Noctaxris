import { test } from "node:test";
import assert from "node:assert/strict";
import { requireReady, signedJsonTarget } from "../lib/helpers.mjs";

test("GuardDuty CreateDetector InjectFindings List GetFindings", async (t) => {
  if (!(await requireReady(t))) return;
  if (process.env.NOCTAXRIS_GUARDDUTY_INJECT !== "1") {
    t.skip(
      "set NOCTAXRIS_GUARDDUTY_INJECT=1 on the API process for lab InjectFindings",
    );
    return;
  }

  const created = await signedJsonTarget(
    "guardduty",
    "GuardDuty.CreateDetector",
    {},
  );
  assert.equal(
    created.status,
    200,
    `CreateDetector status=${created.status} body=${created.body}`,
  );
  const detectorId = created.json?.DetectorId;
  assert.ok(detectorId, `CreateDetector missing DetectorId: ${created.body}`);

  const injected = await signedJsonTarget(
    "guardduty",
    "NoctaxrisGuardDuty.InjectFindings",
    {
      DetectorId: detectorId,
      Findings: [
        {
          type: "UnauthorizedAccess:IAMUser/InstanceCredentialExfiltration.InsideAWS",
          severity: 8,
          title: "sdk cred exfil",
        },
      ],
    },
  );
  assert.equal(
    injected.status,
    200,
    `InjectFindings status=${injected.status} body=${injected.body}`,
  );
  const findingIds = injected.json?.FindingIds || [];
  assert.equal(findingIds.length, 1, `FindingIds=${JSON.stringify(findingIds)}`);
  const findingId = findingIds[0];
  assert.ok(findingId);

  const listed = await signedJsonTarget(
    "guardduty",
    "GuardDuty.ListFindings",
    { DetectorId: detectorId },
  );
  assert.equal(
    listed.status,
    200,
    `ListFindings status=${listed.status} body=${listed.body}`,
  );
  assert.ok(
    (listed.json?.FindingIds || []).includes(findingId),
    `ListFindings missing ${findingId}: ${listed.body}`,
  );

  const got = await signedJsonTarget("guardduty", "GuardDuty.GetFindings", {
    DetectorId: detectorId,
    FindingIds: [findingId],
  });
  assert.equal(
    got.status,
    200,
    `GetFindings status=${got.status} body=${got.body}`,
  );
  const findings = got.json?.Findings || [];
  assert.equal(findings.length, 1);
  assert.ok(findings[0]?.type, `GetFindings missing type: ${got.body}`);
});
