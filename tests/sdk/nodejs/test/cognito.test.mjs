import { test } from "node:test";
import assert from "node:assert/strict";
import {
  AdminCreateUserCommand,
  AdminInitiateAuthCommand,
  CreateUserPoolClientCommand,
  CreateUserPoolCommand,
  DeleteUserPoolCommand,
  DescribeUserPoolCommand,
  InitiateAuthCommand,
  RespondToAuthChallengeCommand,
  UpdateUserPoolCommand,
} from "@aws-sdk/client-cognito-identity-provider";
import { newSDKSRPClient } from "../lib/cognito-srp.mjs";
import { newCognito, requireReady, uniquePrefix } from "../lib/helpers.mjs";

test("Cognito USER_SRP_AUTH round-trip", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newCognito();
  const prefix = uniquePrefix();

  const poolOut = await client.send(
    new CreateUserPoolCommand({ PoolName: `sdk-srp-${prefix}` }),
  );
  const poolID = poolOut.UserPool?.Id;
  assert.ok(poolID, "CreateUserPool missing Id");
  t.after(async () => {
    try {
      await client.send(new DeleteUserPoolCommand({ UserPoolId: poolID }));
    } catch {
      /* ignore */
    }
  });

  const clientOut = await client.send(
    new CreateUserPoolClientCommand({
      UserPoolId: poolID,
      ClientName: "sdk-srp-app",
    }),
  );
  const clientID = clientOut.UserPoolClient?.ClientId;
  assert.ok(clientID, "CreateUserPoolClient missing ClientId");

  const password = "Secret1!";
  const username = `srp-${prefix}`;
  await client.send(
    new AdminCreateUserCommand({
      UserPoolId: poolID,
      Username: username,
      TemporaryPassword: password,
    }),
  );

  const srp = newSDKSRPClient(poolID, username, password);
  const initOut = await client.send(
    new InitiateAuthCommand({
      ClientId: clientID,
      AuthFlow: "USER_SRP_AUTH",
      AuthParameters: {
        USERNAME: username,
        SRP_A: srp.srpAHex(),
      },
    }),
  );
  assert.equal(initOut.ChallengeName, "PASSWORD_VERIFIER");
  const responses = srp.passwordVerifierResponses(initOut.ChallengeParameters || {});
  const respondOut = await client.send(
    new RespondToAuthChallengeCommand({
      ClientId: clientID,
      ChallengeName: "PASSWORD_VERIFIER",
      Session: initOut.Session,
      ChallengeResponses: responses,
    }),
  );
  assert.ok(respondOut.AuthenticationResult?.AccessToken, "missing AccessToken");
  assert.ok(respondOut.AuthenticationResult?.IdToken, "missing IdToken");
});

test("Cognito UpdateUserPool LambdaConfig", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newCognito();
  const prefix = uniquePrefix();

  const poolOut = await client.send(
    new CreateUserPoolCommand({ PoolName: `sdk-trig-${prefix}` }),
  );
  const poolID = poolOut.UserPool?.Id;
  assert.ok(poolID, "CreateUserPool missing Id");
  t.after(async () => {
    try {
      await client.send(new DeleteUserPoolCommand({ UserPoolId: poolID }));
    } catch {
      /* ignore */
    }
  });

  const lambdaARN = `arn:aws:lambda:us-east-1:000000000001:function:pre-signup-${prefix}`;
  await client.send(
    new UpdateUserPoolCommand({
      UserPoolId: poolID,
      LambdaConfig: { PreSignUp: lambdaARN },
    }),
  );
  const desc = await client.send(
    new DescribeUserPoolCommand({ UserPoolId: poolID }),
  );
  assert.equal(desc.UserPool?.LambdaConfig?.PreSignUp, lambdaARN);
});

test("Cognito LambdaConfig trigger fail-closed", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newCognito();
  const prefix = uniquePrefix();

  const poolOut = await client.send(
    new CreateUserPoolCommand({ PoolName: `sdk-failtrig-${prefix}` }),
  );
  const poolID = poolOut.UserPool?.Id;
  assert.ok(poolID, "CreateUserPool missing Id");
  t.after(async () => {
    try {
      await client.send(new DeleteUserPoolCommand({ UserPoolId: poolID }));
    } catch {
      /* ignore */
    }
  });

  const missingARN = `arn:aws:lambda:us-east-1:000000000001:function:missing-${prefix}`;
  await client.send(
    new UpdateUserPoolCommand({
      UserPoolId: poolID,
      LambdaConfig: { PreAuthentication: missingARN },
    }),
  );

  const app = await client.send(
    new CreateUserPoolClientCommand({
      UserPoolId: poolID,
      ClientName: "app",
    }),
  );
  const clientID = app.UserPoolClient?.ClientId;
  assert.ok(clientID, "CreateUserPoolClient missing ClientId");

  const password = "Secret1!";
  const username = `u-${prefix}`;
  await client.send(
    new AdminCreateUserCommand({
      UserPoolId: poolID,
      Username: username,
      TemporaryPassword: password,
    }),
  );

  await assert.rejects(
    () =>
      client.send(
        new AdminInitiateAuthCommand({
          UserPoolId: poolID,
          ClientId: clientID,
          AuthFlow: "ADMIN_USER_PASSWORD_AUTH",
          AuthParameters: {
            USERNAME: username,
            PASSWORD: password,
          },
        }),
      ),
    (err) => {
      assert.match(String(err), /UnexpectedLambdaException/);
      return true;
    },
  );
});
