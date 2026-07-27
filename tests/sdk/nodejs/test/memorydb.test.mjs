import { test } from "node:test";
import assert from "node:assert/strict";
import { requireReady, signedJsonTarget, uniquePrefix } from "../lib/helpers.mjs";

test("MemoryDB Create Describe Delete soft-skip nested", async (t) => {
  if (!(await requireReady(t))) return;
  let name = `sdk-mdb-${uniquePrefix()}`;
  if (name.length > 40) name = name.slice(0, 40);

  const create = await signedJsonTarget(
    "memorydb",
    "AmazonMemoryDB.CreateCluster",
    {
      ClusterName: name,
      NodeType: "db.t4g.small",
      ACLName: "open-access",
      Engine: "redis",
    },
  );
  assert.equal(create.status, 200, create.body);
  assert.ok(create.json?.Cluster, `missing Cluster: ${create.body}`);
  const addr = create.json.Cluster.ClusterEndpoint?.Address || "";
  assert.ok(
    addr.includes("memorydb.noctaxris.internal"),
    `expected nested endpoint, got ${addr}`,
  );

  try {
    let status = "";
    for (let i = 0; i < 16; i++) {
      const desc = await signedJsonTarget(
        "memorydb",
        "AmazonMemoryDB.DescribeClusters",
        { ClusterName: name },
      );
      assert.equal(desc.status, 200, desc.body);
      status = desc.json?.Clusters?.[0]?.Status || "";
      if (status === "available" || status === "failed") break;
      await new Promise((r) => setTimeout(r, 500));
    }
    if (status !== "available") {
      t.skip(
        `MemoryDB nested soft-skip: Status=${status} (Compose noctaxris-engine for available)`,
      );
    }
  } finally {
    await signedJsonTarget("memorydb", "AmazonMemoryDB.DeleteCluster", {
      ClusterName: name,
    });
  }
});
