import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateInstancesCommand,
  DeleteInstanceCommand,
  GetInstanceCommand,
  LightsailClient,
} from "@aws-sdk/client-lightsail";
import {
  AutoScalingClient,
  CreateAutoScalingGroupCommand,
  CreateLaunchConfigurationCommand,
  DeleteAutoScalingGroupCommand,
  DeleteLaunchConfigurationCommand,
  DescribeAutoScalingGroupsCommand,
  SetDesiredCapacityCommand,
} from "@aws-sdk/client-auto-scaling";
import {
  CreateApplicationCommand,
  CreateEnvironmentCommand,
  DeleteApplicationCommand,
  ElasticBeanstalkClient,
  TerminateEnvironmentCommand,
} from "@aws-sdk/client-elastic-beanstalk";
import {
  endpoint,
  region,
  requireReady,
  signedFetch,
  uniquePrefix,
} from "../lib/helpers.mjs";

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

test("Lightsail create get delete", async (t) => {
  if (!(await requireReady(t))) return;
  const c = new LightsailClient(baseClientConfig());
  const name = `${uniquePrefix()}-ls`;
  await c.send(
    new CreateInstancesCommand({
      instanceNames: [name],
      availabilityZone: "us-east-1a",
      blueprintId: "ubuntu_22_04",
      bundleId: "nano_3_0",
    }),
  );
  t.after(async () => {
    try {
      await c.send(new DeleteInstanceCommand({ instanceName: name }));
    } catch {
      /* ignore */
    }
  });
  const got = await c.send(new GetInstanceCommand({ instanceName: name }));
  assert.equal(got.instance?.name, name);
});

test("Auto Scaling SetDesiredCapacity yields InstanceIds", async (t) => {
  if (!(await requireReady(t))) return;
  const c = new AutoScalingClient(baseClientConfig());
  const prefix = uniquePrefix();
  const lc = `${prefix}-lc`;
  const asg = `${prefix}-asg`;
  await c.send(
    new CreateLaunchConfigurationCommand({
      LaunchConfigurationName: lc,
      ImageId: "ami-12345678",
      InstanceType: "t3.micro",
    }),
  );
  t.after(async () => {
    try {
      await c.send(
        new DeleteAutoScalingGroupCommand({
          AutoScalingGroupName: asg,
          ForceDelete: true,
        }),
      );
    } catch {
      /* ignore */
    }
    try {
      await c.send(
        new DeleteLaunchConfigurationCommand({ LaunchConfigurationName: lc }),
      );
    } catch {
      /* ignore */
    }
  });
  await c.send(
    new CreateAutoScalingGroupCommand({
      AutoScalingGroupName: asg,
      LaunchConfigurationName: lc,
      MinSize: 1,
      MaxSize: 3,
      DesiredCapacity: 1,
      AvailabilityZones: ["us-east-1a"],
    }),
  );
  await c.send(
    new SetDesiredCapacityCommand({
      AutoScalingGroupName: asg,
      DesiredCapacity: 2,
    }),
  );
  const desc = await c.send(
    new DescribeAutoScalingGroupsCommand({ AutoScalingGroupNames: [asg] }),
  );
  assert.equal(desc.AutoScalingGroups?.[0]?.DesiredCapacity, 2);
  const members = desc.AutoScalingGroups?.[0]?.Instances || [];
  assert.equal(members.length, 2);
  for (const m of members) {
    assert.match(String(m.InstanceId || ""), /^i-/);
    const ls = String(m.LifecycleState || "");
    assert.ok(
      ls === "Pending" || ls === "InService",
      `LifecycleState=${ls} (Pending OK without engine)`,
    );
  }
});

test("Elastic Beanstalk Ready environment", async (t) => {
  if (!(await requireReady(t))) return;
  const c = new ElasticBeanstalkClient(baseClientConfig());
  const prefix = uniquePrefix();
  const app = `${prefix}-app`;
  const env = `${prefix}-env`;
  await c.send(new CreateApplicationCommand({ ApplicationName: app }));
  t.after(async () => {
    try {
      await c.send(new TerminateEnvironmentCommand({ EnvironmentName: env }));
    } catch {
      /* ignore */
    }
    try {
      await c.send(
        new DeleteApplicationCommand({
          ApplicationName: app,
          TerminateEnvByForce: true,
        }),
      );
    } catch {
      /* ignore */
    }
  });
  const created = await c.send(
    new CreateEnvironmentCommand({
      ApplicationName: app,
      EnvironmentName: env,
    }),
  );
  assert.equal(created.Status, "Ready");
});

test("Backup vault plan job recovery point", async (t) => {
  if (!(await requireReady(t))) return;
  const prefix = uniquePrefix();
  const vault = `${prefix}-vault`.toLowerCase();

  const createVault = await signedFetch(
    "backup",
    "PUT",
    `/backup-vaults/${encodeURIComponent(vault)}`,
    JSON.stringify({}),
    "application/json",
  );
  assert.equal(createVault.status, 200, await createVault.text());
  t.after(async () => {
    await signedFetch(
      "backup",
      "DELETE",
      `/backup-vaults/${encodeURIComponent(vault)}`,
      null,
      "",
    );
  });

  const createPlan = await signedFetch(
    "backup",
    "PUT",
    "/backup/plans/",
    JSON.stringify({
      BackupPlan: {
        BackupPlanName: `${prefix}-plan`,
        Rules: [
          {
            RuleName: "daily",
            TargetBackupVaultName: vault,
            ScheduleExpression: "cron(0 5 ? * * *)",
          },
        ],
      },
    }),
    "application/json",
  );
  const planBody = await createPlan.text();
  assert.equal(createPlan.status, 200, planBody);
  assert.match(planBody, /BackupPlanId/);

  const start = await signedFetch(
    "backup",
    "PUT",
    "/backup-jobs",
    JSON.stringify({
      BackupVaultName: vault,
      ResourceArn: "arn:aws:s3:::lab-bucket",
      IamRoleArn: "arn:aws:iam::000000000001:role/Backup",
    }),
    "application/json",
  );
  const startBody = await start.text();
  assert.equal(start.status, 200, startBody);
  assert.match(startBody, /RecoveryPointArn/);

  const list = await signedFetch(
    "backup",
    "GET",
    `/backup-vaults/${encodeURIComponent(vault)}/recovery-points/`,
    null,
    "",
  );
  const listBody = await list.text();
  assert.equal(list.status, 200, listBody);
  assert.match(listBody, /S3/);
});
