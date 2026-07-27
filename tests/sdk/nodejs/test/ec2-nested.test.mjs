import { test } from "node:test";
import assert from "node:assert/strict";
import {
  DescribeInstancesCommand,
  EC2Client,
  RunInstancesCommand,
  StartInstancesCommand,
  StopInstancesCommand,
  TerminateInstancesCommand,
} from "@aws-sdk/client-ec2";
import { endpoint, region, requireReady } from "../lib/helpers.mjs";

function baseClientConfig() {
  return {
    region: region(),
    endpoint: endpoint(),
    credentials: {
      accessKeyId: process.env.AWS_ACCESS_KEY_ID || "AKIAROOTEXAMPLE01",
      secretAccessKey:
        process.env.AWS_SECRET_ACCESS_KEY ||
        "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
    },
  };
}

test("EC2 Run Stop Start Terminate pending OK", async (t) => {
  if (!(await requireReady(t))) return;
  const c = new EC2Client(baseClientConfig());

  const run = await c.send(
    new RunInstancesCommand({
      ImageId: "ami-alpine",
      InstanceType: "t3.micro",
      MinCount: 1,
      MaxCount: 1,
    }),
  );
  const id = run.Instances?.[0]?.InstanceId;
  assert.ok(id, `missing InstanceId: ${JSON.stringify(run)}`);
  assert.match(id, /^i-/);

  try {
    const desc = await c.send(
      new DescribeInstancesCommand({ InstanceIds: [id] }),
    );
    const state = desc.Reservations?.[0]?.Instances?.[0]?.State?.Name || "";
    assert.ok(
      state === "pending" || state === "running",
      `after Run want pending|running got ${state}`,
    );

    const stop = await c.send(new StopInstancesCommand({ InstanceIds: [id] }));
    assert.ok((stop.StoppingInstances || []).length >= 1);

    const start = await c.send(new StartInstancesCommand({ InstanceIds: [id] }));
    assert.ok((start.StartingInstances || []).length >= 1);
    const started = start.StartingInstances?.[0]?.CurrentState?.Name || "";
    assert.ok(
      started === "pending" || started === "running",
      `after Start want pending|running got ${started}`,
    );

    const term = await c.send(
      new TerminateInstancesCommand({ InstanceIds: [id] }),
    );
    assert.ok((term.TerminatingInstances || []).length >= 1);
  } finally {
    try {
      await c.send(new TerminateInstancesCommand({ InstanceIds: [id] }));
    } catch {
      /* already terminated */
    }
  }
});
