import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateDeliveryStreamCommand,
  DeleteDeliveryStreamCommand,
  DescribeDeliveryStreamCommand,
} from "@aws-sdk/client-firehose";
import {
  CreateRoleCommand,
  DeleteRoleCommand,
  PutRolePolicyCommand,
} from "@aws-sdk/client-iam";
import {
  CreateEventSourceMappingCommand,
  CreateFunctionCommand,
  DeleteEventSourceMappingCommand,
  DeleteFunctionCommand,
} from "@aws-sdk/client-lambda";
import {
  CreateBrokerCommand,
  DeleteBrokerCommand,
  DescribeBrokerCommand,
} from "@aws-sdk/client-mq";
import {
  CreateDomainCommand,
  DeleteDomainCommand,
  DescribeDomainCommand,
} from "@aws-sdk/client-opensearch";
import {
  newFirehose,
  newIAM,
  newLambda,
  newMQ,
  newOpenSearch,
  requireReady,
  signedFetch,
  sleep,
  uniquePrefix,
} from "../lib/helpers.mjs";

const LAMBDA_TRUST =
  '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}';
const FIREHOSE_TRUST =
  '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"sts:AssumeRole"}]}';

function crc32(buf) {
  let c = 0xffffffff;
  for (let i = 0; i < buf.length; i++) {
    c ^= buf[i];
    for (let k = 0; k < 8; k++) {
      c = c & 1 ? (c >>> 1) ^ 0xedb88320 : c >>> 1;
    }
  }
  return (c ^ 0xffffffff) >>> 0;
}

function minimalPythonZip() {
  const name = Buffer.from("index.py");
  const data = Buffer.from(
    "def handler(event, context):\n    return {'ok': True}\n",
  );
  const crc = crc32(data);
  const local = Buffer.alloc(30 + name.length);
  local.writeUInt32LE(0x04034b50, 0);
  local.writeUInt16LE(20, 4);
  local.writeUInt16LE(0, 6);
  local.writeUInt16LE(0, 8);
  local.writeUInt16LE(0, 10);
  local.writeUInt16LE(0, 12);
  local.writeUInt32LE(crc >>> 0, 14);
  local.writeUInt32LE(data.length, 18);
  local.writeUInt32LE(data.length, 22);
  local.writeUInt16LE(name.length, 26);
  local.writeUInt16LE(0, 28);
  name.copy(local, 30);

  const central = Buffer.alloc(46 + name.length);
  central.writeUInt32LE(0x02014b50, 0);
  central.writeUInt16LE(20, 4);
  central.writeUInt16LE(20, 6);
  central.writeUInt16LE(0, 8);
  central.writeUInt16LE(0, 10);
  central.writeUInt16LE(0, 12);
  central.writeUInt16LE(0, 14);
  central.writeUInt32LE(crc >>> 0, 16);
  central.writeUInt32LE(data.length, 20);
  central.writeUInt32LE(data.length, 24);
  central.writeUInt16LE(name.length, 28);
  central.writeUInt16LE(0, 30);
  central.writeUInt16LE(0, 32);
  central.writeUInt16LE(0, 34);
  central.writeUInt16LE(0, 36);
  central.writeUInt32LE(0, 38);
  central.writeUInt32LE(0, 42);
  name.copy(central, 46);

  const eocd = Buffer.alloc(22);
  eocd.writeUInt32LE(0x06054b50, 0);
  eocd.writeUInt16LE(0, 4);
  eocd.writeUInt16LE(0, 6);
  eocd.writeUInt16LE(1, 8);
  eocd.writeUInt16LE(1, 10);
  eocd.writeUInt32LE(central.length, 12);
  eocd.writeUInt32LE(local.length + data.length, 16);
  eocd.writeUInt16LE(0, 20);

  return Buffer.concat([local, data, central, eocd]);
}

/** Lab Active = Created + non-stub Endpoint (SDK may omit lab DomainStatus string). */
function isOpenSearchActive(domainStatus) {
  if (!domainStatus) return false;
  const ep = String(domainStatus.Endpoint || "");
  return domainStatus.Created === true && ep !== "" && !ep.startsWith("stub://");
}

function isOpenSearchTerminal(domainStatus) {
  if (!domainStatus) return false;
  if (isOpenSearchActive(domainStatus)) return true;
  const lab = domainStatus.DomainStatus;
  if (lab === "CreateFailed" || lab === "Active") return true;
  if (domainStatus.Created === false && !domainStatus.Processing) return true;
  const ep = String(domainStatus.Endpoint || "");
  return ep.startsWith("stub://");
}

async function waitDomainStatus(os, domainName, timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  let last;
  while (Date.now() < deadline) {
    const desc = await os.send(
      new DescribeDomainCommand({ DomainName: domainName }),
    );
    last = desc.DomainStatus;
    if (isOpenSearchTerminal(last)) {
      return last;
    }
    await sleep(1000);
  }
  return last;
}

async function waitBrokerState(mq, brokerId, timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  let last;
  while (Date.now() < deadline) {
    const desc = await mq.send(
      new DescribeBrokerCommand({ BrokerId: brokerId }),
    );
    last = desc;
    const state = desc.BrokerState;
    if (
      state === "RUNNING" ||
      state === "CREATION_FAILED" ||
      state === "CRITICAL_ACTION_REQUIRED"
    ) {
      return desc;
    }
    await sleep(1000);
  }
  return last;
}

test("OpenSearch CreateDomain; skip query facade unless Active", async (t) => {
  if (!(await requireReady(t))) return;
  const os = newOpenSearch();
  const domainName = `njsos${Date.now().toString(36)}`.slice(0, 28);

  await os.send(
    new CreateDomainCommand({
      DomainName: domainName,
      EngineVersion: "OpenSearch_2.11",
    }),
  );
  t.after(async () => {
    try {
      await os.send(new DeleteDomainCommand({ DomainName: domainName }));
    } catch {
      /* ignore */
    }
  });

  const status = await waitDomainStatus(os, domainName);
  assert.ok(status, "DescribeDomain returned empty");

  if (!isOpenSearchActive(status)) {
    t.skip(
      `OpenSearch query facade skipped: Created=${status.Created} Endpoint=${status.Endpoint} (nested engine not Active)`,
    );
    return;
  }

  const put = await signedFetch(
    "es",
    "PUT",
    `/opensearch/${domainName}/lab/events/_doc/1`,
    JSON.stringify({ msg: "hi" }),
    "application/json",
  );
  assert.ok(put.status >= 200 && put.status < 300, await put.text());

  const search = await signedFetch(
    "es",
    "POST",
    `/opensearch/${domainName}/lab/events/_search`,
    JSON.stringify({ query: { match_all: {} }, size: 5 }),
    "application/json",
  );
  assert.ok(search.status >= 200 && search.status < 300, await search.text());
});

test("ActiveMQ CreateBroker; skip live unless RUNNING", async (t) => {
  if (!(await requireReady(t))) return;
  const mq = newMQ();
  const brokerName = `njsamq${Date.now().toString(36)}`.slice(0, 50);

  const created = await mq.send(
    new CreateBrokerCommand({
      BrokerName: brokerName,
      EngineType: "ACTIVEMQ",
      EngineVersion: "5.18",
      HostInstanceType: "mq.t3.micro",
      DeploymentMode: "SINGLE_INSTANCE",
      PubliclyAccessible: false,
      Users: [{ Username: "lab", Password: "lab-password-1" }],
    }),
  );
  const brokerId = created.BrokerId;
  assert.ok(brokerId, "missing BrokerId");
  t.after(async () => {
    try {
      await mq.send(new DeleteBrokerCommand({ BrokerId: brokerId }));
    } catch {
      /* ignore */
    }
  });

  const desc = await waitBrokerState(mq, brokerId);
  assert.ok(desc, "DescribeBroker returned empty");
  assert.ok(desc.BrokerState, JSON.stringify(desc));

  if (desc.BrokerState !== "RUNNING") {
    t.skip(
      `ActiveMQ live smoke skipped: BrokerState=${desc.BrokerState} (Describe returned; nested engine not RUNNING)`,
    );
    return;
  }
  assert.ok(
    Array.isArray(desc.BrokerInstances) || desc.BrokerArn,
    "RUNNING broker missing identity fields",
  );
});

test("Firehose OpenSearch Describe shape only when Active domain", async (t) => {
  if (!(await requireReady(t))) return;
  const os = newOpenSearch();
  const fh = newFirehose();
  const iam = newIAM();
  const domainName = `njsfh${Date.now().toString(36)}`.slice(0, 28);
  const streamName = `njsfh-${Date.now().toString(36)}`.slice(0, 64);
  const roleName = `njsfh-role-${Date.now().toString(36)}`.slice(0, 64);

  await os.send(
    new CreateDomainCommand({
      DomainName: domainName,
      EngineVersion: "OpenSearch_2.11",
    }),
  );
  t.after(async () => {
    try {
      await os.send(new DeleteDomainCommand({ DomainName: domainName }));
    } catch {
      /* ignore */
    }
  });

  const status = await waitDomainStatus(os, domainName);
  if (!isOpenSearchActive(status)) {
    t.skip(
      `Firehose OpenSearch Describe skipped: Created=${status?.Created} Endpoint=${status?.Endpoint} (need Active nested domain)`,
    );
    return;
  }

  const role = await iam.send(
    new CreateRoleCommand({
      RoleName: roleName,
      AssumeRolePolicyDocument: FIREHOSE_TRUST,
    }),
  );
  const roleArn = role.Role?.Arn;
  assert.ok(roleArn);
  t.after(async () => {
    try {
      await iam.send(new DeleteRoleCommand({ RoleName: roleName }));
    } catch {
      /* ignore */
    }
  });
  await iam.send(
    new PutRolePolicyCommand({
      RoleName: roleName,
      PolicyName: "os-put",
      PolicyDocument:
        '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"es:ESHttpPut","Resource":"*"}]}',
    }),
  );

  await fh.send(
    new CreateDeliveryStreamCommand({
      DeliveryStreamName: streamName,
      AmazonopensearchserviceDestinationConfiguration: {
        DomainARN: status.ARN,
        IndexName: "events",
        RoleARN: roleArn,
      },
    }),
  );
  t.after(async () => {
    try {
      await fh.send(
        new DeleteDeliveryStreamCommand({ DeliveryStreamName: streamName }),
      );
    } catch {
      /* ignore */
    }
  });

  const desc = await fh.send(
    new DescribeDeliveryStreamCommand({ DeliveryStreamName: streamName }),
  );
  const dests =
    desc.DeliveryStreamDescription?.Destinations || [];
  const raw = JSON.stringify(desc);
  assert.ok(
    raw.includes("OpenSearchDestinationDescription") ||
      raw.includes("AmazonopensearchserviceDestinationDescription"),
    `Describe missing OpenSearch destination shape: ${raw}`,
  );
  assert.ok(dests.length >= 1, raw);
});

test("Lambda MQ ESM CreateEventSourceMapping only when broker RUNNING", async (t) => {
  if (!(await requireReady(t))) return;
  const mq = newMQ();
  const iam = newIAM();
  const lam = newLambda();
  const brokerName = `njsesm${Date.now().toString(36)}`.slice(0, 50);
  const roleName = `njsesm-role-${Date.now().toString(36)}`.slice(0, 64);
  const fnName = `njsesm-fn-${Date.now().toString(36)}`.slice(0, 64);

  const created = await mq.send(
    new CreateBrokerCommand({
      BrokerName: brokerName,
      EngineType: "ACTIVEMQ",
      EngineVersion: "5.18",
      HostInstanceType: "mq.t3.micro",
      DeploymentMode: "SINGLE_INSTANCE",
      PubliclyAccessible: false,
      Users: [{ Username: "lab", Password: "lab-password-1" }],
    }),
  );
  const brokerId = created.BrokerId;
  const brokerArn = created.BrokerArn;
  assert.ok(brokerId);
  t.after(async () => {
    try {
      await mq.send(new DeleteBrokerCommand({ BrokerId: brokerId }));
    } catch {
      /* ignore */
    }
  });

  const desc = await waitBrokerState(mq, brokerId);
  if (desc?.BrokerState !== "RUNNING") {
    t.skip(
      `Lambda MQ ESM skipped: BrokerState=${desc?.BrokerState} (need RUNNING nested broker)`,
    );
    return;
  }
  assert.ok(brokerArn || desc.BrokerArn, "missing BrokerArn");

  const role = await iam.send(
    new CreateRoleCommand({
      RoleName: roleName,
      AssumeRolePolicyDocument: LAMBDA_TRUST,
    }),
  );
  const roleArn = role.Role?.Arn;
  assert.ok(roleArn);
  t.after(async () => {
    try {
      await iam.send(new DeleteRoleCommand({ RoleName: roleName }));
    } catch {
      /* ignore */
    }
  });
  await iam.send(
    new PutRolePolicyCommand({
      RoleName: roleName,
      PolicyName: "esm-mq",
      PolicyDocument:
        '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["mq:DescribeBroker"],"Resource":"*"}]}',
    }),
  );

  await lam.send(
    new CreateFunctionCommand({
      FunctionName: fnName,
      Runtime: "python3.12",
      Role: roleArn,
      Handler: "index.handler",
      Code: { ZipFile: minimalPythonZip() },
    }),
  );
  t.after(async () => {
    try {
      await lam.send(new DeleteFunctionCommand({ FunctionName: fnName }));
    } catch {
      /* ignore */
    }
  });

  const mapping = await lam.send(
    new CreateEventSourceMappingCommand({
      FunctionName: fnName,
      EventSourceArn: brokerArn || desc.BrokerArn,
      BatchSize: 5,
    }),
  );
  assert.ok(mapping.UUID, JSON.stringify(mapping));
  t.after(async () => {
    try {
      await lam.send(
        new DeleteEventSourceMappingCommand({ UUID: mapping.UUID }),
      );
    } catch {
      /* ignore */
    }
  });
});
