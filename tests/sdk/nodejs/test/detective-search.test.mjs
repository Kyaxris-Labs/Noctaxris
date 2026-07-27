import { test } from "node:test";
import assert from "node:assert/strict";
import { GetCallerIdentityCommand } from "@aws-sdk/client-sts";
import {
  newSTS,
  requireReady,
  signedJsonTarget,
  uniquePrefix,
} from "../lib/helpers.mjs";

test("Detective CreateGraph SearchGraph with CT+GD inject seed", async (t) => {
  if (!(await requireReady(t))) return;
  if (
    process.env.NOCTAXRIS_CLOUDTRAIL_INJECT !== "1" ||
    process.env.NOCTAXRIS_GUARDDUTY_INJECT !== "1"
  ) {
    t.skip(
      "requires NOCTAXRIS_CLOUDTRAIL_INJECT=1 and NOCTAXRIS_GUARDDUTY_INJECT=1 on the API process for SearchGraph seed",
    );
    return;
  }

  const sts = newSTS();
  const caller = await sts.send(new GetCallerIdentityCommand({}));
  assert.ok(caller.Account, "GetCallerIdentity missing Account");
  const accountID = caller.Account;
  const prefix = uniquePrefix();
  const sharedIP = "203.0.113.50";
  const roleArn = `arn:aws:iam::${accountID}:role/SdkDetective-${prefix}`;
  const eventID = `sdk-det-${prefix}`;

  const created = await signedJsonTarget(
    "detective",
    "AmazonDetective.CreateGraph",
    {},
  );
  assert.equal(created.status, 200, created.body);
  const graphArn = created.json?.GraphArn;
  assert.ok(graphArn, `CreateGraph missing GraphArn: ${created.body}`);

  const injCT = await signedJsonTarget(
    "cloudtrail",
    "NoctaxrisCloudTrail.InjectEvents",
    {
      Events: [
        {
          eventTime: "2026-07-20T12:00:00Z",
          sourceIPAddress: sharedIP,
          userIdentity: { type: "IAMUser", arn: roleArn },
          eventSource: "sts.amazonaws.com",
          eventName: "AssumeRole",
          eventID,
          readOnly: true,
        },
      ],
    },
  );
  assert.equal(injCT.status, 200, injCT.body);

  const detector = await signedJsonTarget(
    "guardduty",
    "GuardDuty.CreateDetector",
    {},
  );
  assert.equal(detector.status, 200, detector.body);
  const detectorID = detector.json?.DetectorId;
  assert.ok(detectorID, `CreateDetector missing DetectorId: ${detector.body}`);

  const injGD = await signedJsonTarget(
    "guardduty",
    "NoctaxrisGuardDuty.InjectFindings",
    {
      DetectorId: detectorID,
      Findings: [
        {
          type: "UnauthorizedAccess:IAMUser/InstanceCredentialExfiltration.InsideAWS",
          severity: 8,
          resource: {
            resourceType: "AccessKey",
            accessKeyDetails: { userName: "sdk-detective" },
          },
          service: {
            action: {
              awsApiCallAction: {
                remoteIpDetails: { ipAddressV4: sharedIP },
              },
            },
          },
        },
      ],
    },
  );
  assert.equal(injGD.status, 200, injGD.body);

  const search = await signedJsonTarget(
    "detective",
    "AmazonDetective.SearchGraph",
    {
      GraphArn: graphArn,
      ResourceArn: roleArn,
      IpAddress: sharedIP,
      MaxResults: 10,
    },
  );
  assert.equal(search.status, 200, search.body);
  assert.ok(
    (search.json?.CloudTrailEvents || []).length >= 1,
    `SearchGraph CloudTrailEvents empty: ${search.body}`,
  );
  assert.ok(
    (search.json?.GuardDutyFindings || []).length >= 1,
    `SearchGraph GuardDutyFindings empty: ${search.body}`,
  );
});
