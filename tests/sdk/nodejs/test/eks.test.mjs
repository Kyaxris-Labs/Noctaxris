import { test } from "node:test";
import assert from "node:assert/strict";
import {
  requireReady,
  signedFetch,
  uniquePrefix,
} from "../lib/helpers.mjs";

test("EKS CreateDescribeListDelete soft-skip when down", async (t) => {
  if (!(await requireReady(t))) return;
  const name = `jseks-${uniquePrefix()}`.replace(/_/g, "-").slice(0, 40);
  const createBody = Buffer.from(
    JSON.stringify({
      name,
      roleArn: "arn:aws:iam::000000000001:role/eks",
      version: "1.29",
      resourcesVpcConfig: {
        subnetIds: ["subnet-1"],
        securityGroupIds: [],
      },
    }),
  );
  const createResp = await signedFetch(
    "eks",
    "POST",
    "/clusters",
    createBody,
    "application/json",
  );
  assert.ok(createResp.status >= 200 && createResp.status < 300, createResp.status);
  t.after(async () => {
    await signedFetch("eks", "DELETE", `/clusters/${encodeURIComponent(name)}`);
  });

  const created = await createResp.json();
  const status = created?.cluster?.status || "";
  if (status !== "ACTIVE") {
    t.skip(`EKS live smoke skipped: status=${status}`);
    return;
  }
  assert.match(String(created?.cluster?.endpoint || ""), /noctaxris-eks-/);

  const desc = await signedFetch("eks", "GET", `/clusters/${encodeURIComponent(name)}`);
  assert.equal(desc.status, 200);
  const described = await desc.json();
  assert.equal(described?.cluster?.status, "ACTIVE");
  const list = await signedFetch("eks", "GET", "/clusters");
  const listText = await list.text();
  assert.equal(list.status, 200);
  assert.match(listText, new RegExp(name));

  const nodes = await signedFetch(
    "eks",
    "GET",
    `/clusters/${encodeURIComponent(name)}/node-groups`,
  );
  assert.equal(nodes.status, 200);

  const del = await signedFetch(
    "eks",
    "DELETE",
    `/clusters/${encodeURIComponent(name)}`,
  );
  assert.ok(del.status >= 200 && del.status < 300, del.status);
  const gone = await signedFetch("eks", "GET", `/clusters/${encodeURIComponent(name)}`);
  assert.notEqual(gone.status, 200);
});
