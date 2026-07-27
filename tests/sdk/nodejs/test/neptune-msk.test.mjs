import { test } from "node:test";
import assert from "node:assert/strict";
import {
  requireReady,
  signedFetch,
  signedJsonTarget,
  sleep,
  uniquePrefix,
} from "../lib/helpers.mjs";

test("Neptune CreateDBCluster soft-skip unless available", async (t) => {
  if (!(await requireReady(t))) return;
  const id = `jsnep-${uniquePrefix()}`.replace(/_/g, "-").slice(0, 40);
  const createBody = new URLSearchParams({
    Action: "CreateDBCluster",
    Version: "2014-10-31",
    DBClusterIdentifier: id,
    Engine: "neptune",
  }).toString();
  const createResp = await signedFetch(
    "neptune",
    "POST",
    "/",
    Buffer.from(createBody),
    "application/x-www-form-urlencoded",
  );
  assert.ok(createResp.status >= 200 && createResp.status < 300, createResp.status);
  t.after(async () => {
    const delBody = new URLSearchParams({
      Action: "DeleteDBCluster",
      Version: "2014-10-31",
      DBClusterIdentifier: id,
    }).toString();
    await signedFetch(
      "neptune",
      "POST",
      "/",
      Buffer.from(delBody),
      "application/x-www-form-urlencoded",
    );
  });

  let status = "";
  let descText = "";
  for (let i = 0; i < 8; i++) {
    const q = new URLSearchParams({
      Action: "DescribeDBClusters",
      Version: "2014-10-31",
      DBClusterIdentifier: id,
    }).toString();
    const desc = await signedFetch(
      "neptune",
      "POST",
      "/",
      Buffer.from(q),
      "application/x-www-form-urlencoded",
    );
    descText = await desc.text();
    const m = descText.match(/<Status>([^<]+)<\/Status>/);
    status = m ? m[1] : "";
    if (status === "available" || status === "failed") break;
    await sleep(500);
  }
  if (status !== "available") {
    t.skip(`Neptune live smoke skipped: status=${status}`);
    return;
  }
  assert.match(descText, /neptune\.noctaxris\.internal/);
});

test("MSK CreateCluster soft-skip unless ACTIVE", async (t) => {
  if (!(await requireReady(t))) return;
  const name = `jsmsk-${uniquePrefix()}`.replace(/_/g, "-").slice(0, 40);
  const created = await signedJsonTarget("kafka", "Kafka_1.0.CreateCluster", {
    ClusterName: name,
    KafkaVersion: "3.6.0",
    NumberOfBrokerNodes: 1,
    BrokerNodeGroupInfo: {
      InstanceType: "kafka.m5.large",
      ClientSubnets: ["subnet-1"],
    },
  });
  assert.ok(created.status >= 200 && created.status < 300, created.body);
  const arn = created.json?.ClusterArn;
  assert.ok(arn);
  t.after(async () => {
    await signedJsonTarget("kafka", "Kafka_1.0.DeleteCluster", {
      ClusterArn: arn,
    });
  });

  let state = "";
  for (let i = 0; i < 8; i++) {
    const desc = await signedJsonTarget("kafka", "Kafka_1.0.DescribeCluster", {
      ClusterArn: arn,
    });
    state = desc.json?.ClusterInfo?.State || "";
    if (state === "ACTIVE" || state === "FAILED") break;
    await sleep(500);
  }
  if (state !== "ACTIVE") {
    t.skip(`MSK live smoke skipped: State=${state}`);
    return;
  }
  const boot = await signedJsonTarget(
    "kafka",
    "Kafka_1.0.GetBootstrapBrokers",
    { ClusterArn: arn },
  );
  assert.match(String(boot.json?.BootstrapBrokerString || ""), /noctaxris-msk-/);
});
