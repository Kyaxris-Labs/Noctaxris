import { test } from "node:test";
import assert from "node:assert/strict";
import { requireReady, signedJsonTarget, uniquePrefix } from "../lib/helpers.mjs";

test("CUR PutReportDefinition Describe Delete", async (t) => {
  if (!(await requireReady(t))) return;
  const prefix = uniquePrefix();
  const reportName = `sdk-${prefix}`;

  const put = await signedJsonTarget(
    "cur",
    "AWSOrigamiServiceGatewayService.PutReportDefinition",
    {
      ReportDefinition: {
        ReportName: reportName,
        TimeUnit: "MONTHLY",
        Format: "textORcsv",
        Compression: "GZIP",
        S3Bucket: `cur-sdk-${prefix}`,
        S3Prefix: "reports",
        S3Region: "us-east-1",
        AdditionalSchemaElements: ["RESOURCES"],
        ReportVersioning: "OVERWRITE_REPORT",
      },
    },
  );
  assert.equal(put.status, 200, put.body);
  assert.ok(put.json?.ReportName, `missing ReportName: ${put.body}`);

  const desc = await signedJsonTarget(
    "cur",
    "AWSOrigamiServiceGatewayService.DescribeReportDefinitions",
    {},
  );
  assert.equal(desc.status, 200, desc.body);
  assert.ok(
    (desc.json?.ReportDefinitions || []).length >= 1,
    `DescribeReportDefinitions empty: ${desc.body}`,
  );

  const del = await signedJsonTarget(
    "cur",
    "AWSOrigamiServiceGatewayService.DeleteReportDefinition",
    { ReportName: reportName },
  );
  assert.equal(del.status, 200, del.body);
});

test("IoT CreateThing cert AttachThingPrincipal shadow", async (t) => {
  if (!(await requireReady(t))) return;
  const prefix = `sdk-iot-${uniquePrefix()}`;

  const created = await signedJsonTarget("iot", "AWSIotService.CreateThing", {
    thingName: prefix,
    attributePayload: { attributes: { env: "sdk" } },
  });
  assert.equal(created.status, 200, created.body);

  const cert = await signedJsonTarget(
    "iot",
    "AWSIotService.CreateKeysAndCertificate",
    { setAsActive: true },
  );
  assert.equal(cert.status, 200, cert.body);
  const certArn = cert.json?.certificateArn;
  assert.ok(certArn, `missing certificateArn: ${cert.body}`);

  const pol = await signedJsonTarget("iot", "AWSIotService.CreatePolicy", {
    policyName: `${prefix}-pol`,
    policyDocument:
      '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:*","Resource":"*"}]}',
  });
  assert.equal(pol.status, 200, pol.body);

  const attach = await signedJsonTarget(
    "iot",
    "AWSIotService.AttachThingPrincipal",
    { thingName: prefix, principal: certArn },
  );
  assert.equal(attach.status, 200, attach.body);

  const shadow = await signedJsonTarget(
    "iot-data",
    "AWSIotDataService.UpdateThingShadow",
    {
      thingName: prefix,
      state: { desired: { color: "green" } },
    },
  );
  assert.equal(shadow.status, 200, shadow.body);

  const got = await signedJsonTarget(
    "iot-data",
    "AWSIotDataService.GetThingShadow",
    { thingName: prefix },
  );
  assert.equal(got.status, 200, got.body);
  assert.ok(got.json?.state, `GetThingShadow missing state: ${got.body}`);
});
