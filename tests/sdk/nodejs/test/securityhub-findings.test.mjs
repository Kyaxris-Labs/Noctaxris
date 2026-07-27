import { test } from "node:test";
import assert from "node:assert/strict";
import { GetCallerIdentityCommand } from "@aws-sdk/client-sts";
import {
  newSTS,
  requireReady,
  signedJsonTarget,
  uniquePrefix,
} from "../lib/helpers.mjs";

test("Security Hub BatchImportFindings then GetFindings", async (t) => {
  if (!(await requireReady(t))) return;

  const prefix = uniquePrefix();
  const region = process.env.AWS_DEFAULT_REGION || "us-east-1";
  const findingId = `sdk-sh-${prefix}`;
  const generatorId = "noctaxris-sdk-lab";
  const now = new Date().toISOString();

  const sts = newSTS();
  const caller = await sts.send(new GetCallerIdentityCommand({}));
  const accountId = caller.Account;
  assert.ok(accountId, "GetCallerIdentity missing Account");
  const productArn = `arn:aws:securityhub:${region}:${accountId}:product/${accountId}/default`;

  const imported = await signedJsonTarget(
    "securityhub",
    "SecurityHub.BatchImportFindings",
    {
      Findings: [
        {
          SchemaVersion: "2018-10-08",
          Id: findingId,
          ProductArn: productArn,
          GeneratorId: generatorId,
          AwsAccountId: accountId,
          Types: [
            "Software and Configuration Checks/Vulnerabilities/CVE",
          ],
          CreatedAt: now,
          UpdatedAt: now,
          Severity: { Label: "HIGH" },
          Title: "sdk lab finding",
          Description: "security hub sdk import",
          Resources: [
            {
              Type: "AwsS3Bucket",
              Id: "arn:aws:s3:::sdk-sh-lab",
            },
          ],
        },
      ],
    },
  );
  assert.equal(
    imported.status,
    200,
    `BatchImportFindings status=${imported.status} body=${imported.body}`,
  );
  assert.equal(imported.json?.SuccessCount, 1);
  assert.equal(imported.json?.FailedCount, 0);

  const got = await signedJsonTarget("securityhub", "SecurityHub.GetFindings", {
    GeneratorId: generatorId,
    SeverityLabel: "HIGH",
    ResourceType: "AwsS3Bucket",
  });
  assert.equal(
    got.status,
    200,
    `GetFindings status=${got.status} body=${got.body}`,
  );
  const findings = got.json?.Findings || [];
  const found = findings.some(
    (f) => f?.Id === findingId || f?.Title === "sdk lab finding",
  );
  assert.ok(found, `GetFindings missing imported finding: ${got.body}`);
});
