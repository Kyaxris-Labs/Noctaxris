import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateRoleCommand,
  DeleteRoleCommand,
} from "@aws-sdk/client-iam";
import {
  newIAM,
  requireReady,
  signedJsonTarget,
  uniquePrefix,
} from "../lib/helpers.mjs";

const CODEBUILD_TRUST =
  '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"codebuild.amazonaws.com"},"Action":"sts:AssumeRole"}]}';

test("CodeBuild CreateProject List BatchGet + CodeCommit PutFile; StartBuild nested-gated", async (t) => {
  if (!(await requireReady(t))) return;
  const iam = newIAM();
  const prefix = uniquePrefix();

  let roleName = `${prefix}-cb-role`;
  if (roleName.length > 64) roleName = roleName.slice(0, 64);
  const roleOut = await iam.send(
    new CreateRoleCommand({
      RoleName: roleName,
      AssumeRolePolicyDocument: CODEBUILD_TRUST,
    }),
  );
  const roleArn = roleOut.Role?.Arn;
  assert.ok(roleArn, "missing role Arn");
  t.after(async () => {
    try {
      await iam.send(new DeleteRoleCommand({ RoleName: roleName }));
    } catch {
      /* ignore */
    }
  });

  let repoName = `cb-src-${prefix}`.replaceAll("_", "-");
  if (repoName.length > 100) repoName = repoName.slice(0, 100);
  const repo = await signedJsonTarget(
    "codecommit",
    "CodeCommit_20150413.CreateRepository",
    { repositoryName: repoName },
  );
  assert.equal(repo.status, 200, repo.body);
  t.after(async () => {
    try {
      await signedJsonTarget("codecommit", "CodeCommit_20150413.DeleteRepository", {
        repositoryName: repoName,
      });
    } catch {
      /* ignore */
    }
  });

  const put = await signedJsonTarget("codecommit", "CodeCommit_20150413.PutFile", {
    repositoryName: repoName,
    branchName: "main",
    filePath: "README.md",
    fileContent: "hello-codecommit",
  });
  assert.equal(put.status, 200, put.body);
  assert.ok(put.json?.commitId, "PutFile missing commitId");

  let projectName = `cb-proj-${prefix}`.replaceAll("_", "-");
  if (projectName.length > 100) projectName = projectName.slice(0, 100);
  const create = await signedJsonTarget(
    "codebuild",
    "CodeBuild_20161006.CreateProject",
    {
      name: projectName,
      serviceRole: roleArn,
      source: {
        type: "CODECOMMIT",
        location: repoName,
        buildspec:
          '{"version":"0.2","phases":{"build":{"commands":["cat README.md"]}}}',
      },
      environment: {
        type: "LINUX_CONTAINER",
        image: "alpine:3.20",
      },
      artifacts: { type: "NO_ARTIFACTS" },
    },
  );
  assert.equal(create.status, 200, create.body);
  assert.equal(create.json?.project?.name, projectName);
  t.after(async () => {
    try {
      await signedJsonTarget("codebuild", "CodeBuild_20161006.DeleteProject", {
        name: projectName,
      });
    } catch {
      /* ignore */
    }
  });

  const list = await signedJsonTarget(
    "codebuild",
    "CodeBuild_20161006.ListProjects",
    {},
  );
  assert.equal(list.status, 200, list.body);
  assert.ok(
    Array.isArray(list.json?.projects) &&
      list.json.projects.includes(projectName),
    `ListProjects missing ${projectName}: ${list.body}`,
  );

  const batch = await signedJsonTarget(
    "codebuild",
    "CodeBuild_20161006.BatchGetProjects",
    { names: [projectName] },
  );
  assert.equal(batch.status, 200, batch.body);
  const projects = batch.json?.projects;
  assert.equal(projects?.length, 1, batch.body);
  assert.equal(projects[0]?.source?.type, "CODECOMMIT");
  assert.equal(projects[0]?.source?.location, repoName);

  const wh = await signedJsonTarget(
    "codebuild",
    "CodeBuild_20161006.CreateWebhook",
    {
      projectName,
      filterGroups: [[{ type: "EVENT", pattern: "PUSH" }]],
    },
  );
  assert.equal(wh.status, 200, wh.body);
  assert.ok(wh.json?.webhook?.payloadUrl, wh.body);
  assert.ok(wh.json?.webhook?.secret, wh.body);
  const listWh = await signedJsonTarget(
    "codebuild",
    "CodeBuild_20161006.ListWebhooks",
    { projectName },
  );
  assert.equal(listWh.status, 200, listWh.body);
  assert.equal(listWh.json?.webhooks?.length, 1, listWh.body);
  const delWh = await signedJsonTarget(
    "codebuild",
    "CodeBuild_20161006.DeleteWebhook",
    { projectName },
  );
  assert.equal(delWh.status, 200, delWh.body);

  if (process.env.NOCTAXRIS_NESTED !== "1") {
    return;
  }

  const start = await signedJsonTarget(
    "codebuild",
    "CodeBuild_20161006.StartBuild",
    { projectName },
  );
  if (
    start.status === 503 &&
    String(start.body).includes("compute unavailable")
  ) {
    t.skip(
      "CodeBuild StartBuild skipped: compute unavailable (nested engine not healthy)",
    );
    return;
  }
  assert.equal(start.status, 200, start.body);
  assert.ok(start.json?.build?.id, `StartBuild missing build.id: ${start.body}`);
});
