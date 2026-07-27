import { test } from "node:test";
import assert from "node:assert/strict";
import {
  requireReady,
  signedFetch,
  sleep,
  uniquePrefix,
} from "../lib/helpers.mjs";

async function rdsForm(params) {
  const body = new URLSearchParams(params).toString();
  const resp = await signedFetch(
    "rds",
    "POST",
    "/",
    body,
    "application/x-www-form-urlencoded",
  );
  const text = await resp.text();
  return { status: resp.status, body: text };
}

test("RDS MySQL and MariaDB Create Describe Delete soft-skip nested", async (t) => {
  if (!(await requireReady(t))) {
    return;
  }
  const prefix = uniquePrefix();
  for (const engine of ["mysql", "mariadb"]) {
    await t.test(engine, async (t) => {
      let id = `${prefix}-${engine}`.toLowerCase().replace(/_/g, "-");
      if (id.length > 60) {
        id = id.slice(0, 60);
      }
      const create = await rdsForm({
        Action: "CreateDBInstance",
        Version: "2014-10-31",
        DBInstanceIdentifier: id,
        Engine: engine,
        DBInstanceClass: "db.t3.micro",
        MasterUsername: "root",
        MasterUserPassword: "lab-password-1",
        AllocatedStorage: "20",
        DBName: "appdb",
      });
      assert.equal(create.status, 200, create.body);
      assert.match(create.body, new RegExp(id));
      assert.match(create.body, new RegExp(engine));
      t.after(async () => {
        await rdsForm({
          Action: "DeleteDBInstance",
          Version: "2014-10-31",
          DBInstanceIdentifier: id,
        });
      });

      const desc = await rdsForm({
        Action: "DescribeDBInstances",
        Version: "2014-10-31",
        DBInstanceIdentifier: id,
      });
      assert.equal(desc.status, 200, desc.body);
      assert.match(desc.body, new RegExp(id));

      if (process.env.NOCTAXRIS_NESTED === "1") {
        let last = desc.body;
        for (let i = 0; i < 45; i++) {
          const poll = await rdsForm({
            Action: "DescribeDBInstances",
            Version: "2014-10-31",
            DBInstanceIdentifier: id,
          });
          last = poll.body;
          if (last.includes("available")) {
            break;
          }
          if (last.includes("failed")) {
            t.skip(
              `RDS ${engine} nested soft-skip: status failed (nested engine not healthy)`,
            );
            return;
          }
          await sleep(2000);
        }
        if (!last.includes("available")) {
          t.skip(
            `RDS ${engine} nested soft-skip: status not available (healthy DinD required)`,
          );
          return;
        }
      }

      const del = await rdsForm({
        Action: "DeleteDBInstance",
        Version: "2014-10-31",
        DBInstanceIdentifier: id,
      });
      assert.equal(del.status, 200, del.body);
    });
  }
});
