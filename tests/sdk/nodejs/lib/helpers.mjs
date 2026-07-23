import { DynamoDBClient } from "@aws-sdk/client-dynamodb";
import { EventBridgeClient } from "@aws-sdk/client-eventbridge";
import { IAMClient } from "@aws-sdk/client-iam";
import { KMSClient } from "@aws-sdk/client-kms";
import { LambdaClient } from "@aws-sdk/client-lambda";
import { S3Client } from "@aws-sdk/client-s3";
import { SecretsManagerClient } from "@aws-sdk/client-secrets-manager";
import { SNSClient } from "@aws-sdk/client-sns";
import { SQSClient } from "@aws-sdk/client-sqs";
import { SSMClient } from "@aws-sdk/client-ssm";
import { STSClient } from "@aws-sdk/client-sts";

export function endpoint() {
  const v = (process.env.NOCTAXRIS_ENDPOINT || "http://127.0.0.1:4566").trim();
  return v.replace(/\/+$/, "");
}

function skipIfDownEnabled() {
  const v = (process.env.NOCTAXRIS_SKIP_IF_DOWN || "").trim().toLowerCase();
  return v === "1" || v === "true" || v === "yes";
}

/**
 * Fail with a clear message when the API is down, unless NOCTAXRIS_SKIP_IF_DOWN is set.
 * Returns false when the test was skipped so callers can return early.
 * @param {import("node:test").TestContext} t
 * @returns {Promise<boolean>}
 */
export async function requireReady(t) {
  const ep = endpoint();
  const url = `${ep}/_noctaxris/ready`;
  let errMsg;
  try {
    const resp = await fetch(url, { signal: AbortSignal.timeout(2000) });
    if (!resp.ok) {
      errMsg = `Noctaxris not ready at ${ep}: status ${resp.status}`;
    }
  } catch (err) {
    errMsg = `Noctaxris not reachable at ${ep}: ${err}`;
  }
  if (!errMsg) {
    return true;
  }
  if (skipIfDownEnabled()) {
    t.skip(errMsg);
    return false;
  }
  throw new Error(
    `${errMsg} (set NOCTAXRIS_SKIP_IF_DOWN=1 to skip when the API is down)`,
  );
}

export function uniquePrefix() {
  return `njsit-${process.hrtime.bigint()}`;
}

function baseConfig() {
  return {
    region: process.env.AWS_DEFAULT_REGION || "us-east-1",
    endpoint: endpoint(),
    credentials: {
      accessKeyId: process.env.AWS_ACCESS_KEY_ID || "AKIAROOTEXAMPLE01",
      secretAccessKey:
        process.env.AWS_SECRET_ACCESS_KEY ||
        "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
    },
  };
}

export function newSTS() {
  return new STSClient(baseConfig());
}

export function newS3() {
  return new S3Client({ ...baseConfig(), forcePathStyle: true });
}

export function newDDB() {
  return new DynamoDBClient(baseConfig());
}

export function newIAM() {
  return new IAMClient(baseConfig());
}

export function newKMS() {
  return new KMSClient(baseConfig());
}

export function newSQS() {
  return new SQSClient(baseConfig());
}

export function newSNS() {
  return new SNSClient(baseConfig());
}

export function newLambda() {
  return new LambdaClient(baseConfig());
}

export function newEvents() {
  return new EventBridgeClient(baseConfig());
}

export function newSSM() {
  return new SSMClient(baseConfig());
}

export function newSecrets() {
  return new SecretsManagerClient(baseConfig());
}
