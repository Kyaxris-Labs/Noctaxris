import assert from "node:assert/strict";
import test from "node:test";
import {
  CreateDeploymentCommand,
  CreateResourceCommand,
  CreateRestApiCommand,
  DeleteRestApiCommand,
  GetResourcesCommand,
  PutIntegrationCommand,
  PutMethodCommand,
} from "@aws-sdk/client-api-gateway";
import { endpoint, newAPIGW, requireReady, uniquePrefix } from "../lib/helpers.mjs";

test("REST API MOCK execute path", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newAPIGW();
  const prefix = uniquePrefix();

  const api = await client.send(
    new CreateRestApiCommand({ name: `${prefix}-rest` }),
  );
  assert.ok(api.id);
  const apiId = api.id;
  t.after(async () => {
    try {
      await client.send(new DeleteRestApiCommand({ restApiId: apiId }));
    } catch {
      /* ignore */
    }
  });

  const resources = await client.send(
    new GetResourcesCommand({ restApiId: apiId }),
  );
  const root = (resources.items || []).find((r) => r.path === "/");
  assert.ok(root?.id);

  const child = await client.send(
    new CreateResourceCommand({
      restApiId: apiId,
      parentId: root.id,
      pathPart: "hello",
    }),
  );
  assert.ok(child.id);

  try {
    await client.send(
      new PutMethodCommand({
        restApiId: apiId,
        resourceId: child.id,
        httpMethod: "GET",
        authorizationType: "NONE",
      }),
    );
  } catch (err) {
    const msg = String(err?.message || err);
    if (msg.includes("NOCTAXRIS_ALLOW_OPEN_DATA_PLANE")) {
      t.skip(
        "REST MOCK soft-skip: set NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1 (compose.lab-open.yaml) for AuthType NONE on non-loopback listen",
      );
      return;
    }
    throw err;
  }

  await client.send(
    new PutIntegrationCommand({
      restApiId: apiId,
      resourceId: child.id,
      httpMethod: "GET",
      type: "MOCK",
      requestTemplates: {
        "application/json": '{"message":"sdk-mock-ok"}',
      },
    }),
  );

  await client.send(
    new CreateDeploymentCommand({
      restApiId: apiId,
      stageName: "dev",
    }),
  );

  const url = `${endpoint()}/restapis/${apiId}/dev/_user_request_/hello`;
  const resp = await fetch(url);
  const body = await resp.text();
  assert.equal(resp.status, 200, body);
  assert.match(body, /sdk-mock-ok/);
});
