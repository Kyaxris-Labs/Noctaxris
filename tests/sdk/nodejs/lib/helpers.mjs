import { AppConfigClient } from "@aws-sdk/client-appconfig";
import { AppConfigDataClient } from "@aws-sdk/client-appconfigdata";
import { AppSyncClient } from "@aws-sdk/client-appsync";
import { ApiGatewayV2Client } from "@aws-sdk/client-apigatewayv2";
import { APIGatewayClient } from "@aws-sdk/client-api-gateway";
import { CloudTrailClient } from "@aws-sdk/client-cloudtrail";
import { CognitoIdentityProviderClient } from "@aws-sdk/client-cognito-identity-provider";
import { ConfigServiceClient } from "@aws-sdk/client-config-service";
import { DynamoDBClient } from "@aws-sdk/client-dynamodb";
import { EventBridgeClient } from "@aws-sdk/client-eventbridge";
import { FirehoseClient } from "@aws-sdk/client-firehose";
import { GlueClient } from "@aws-sdk/client-glue";
import { IAMClient } from "@aws-sdk/client-iam";
import { KinesisClient } from "@aws-sdk/client-kinesis";
import { KMSClient } from "@aws-sdk/client-kms";
import { LambdaClient } from "@aws-sdk/client-lambda";
import { MqClient } from "@aws-sdk/client-mq";
import { OpenSearchClient } from "@aws-sdk/client-opensearch";
import { S3Client } from "@aws-sdk/client-s3";
import { SecretsManagerClient } from "@aws-sdk/client-secrets-manager";
import { SFNClient } from "@aws-sdk/client-sfn";
import { SNSClient } from "@aws-sdk/client-sns";
import { SQSClient } from "@aws-sdk/client-sqs";
import { SSMClient } from "@aws-sdk/client-ssm";
import { STSClient } from "@aws-sdk/client-sts";
import { Sha256 } from "@aws-crypto/sha256-js";
import { HttpRequest } from "@smithy/protocol-http";
import { SignatureV4 } from "@smithy/signature-v4";

export function endpoint() {
  const v = (process.env.NOCTAXRIS_ENDPOINT || "http://127.0.0.1:4566").trim();
  return v.replace(/\/+$/, "");
}

export function region() {
  return process.env.AWS_DEFAULT_REGION || "us-east-1";
}

export function credentials() {
  return {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID || "AKIAROOTEXAMPLE01",
    secretAccessKey:
      process.env.AWS_SECRET_ACCESS_KEY ||
      "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
  };
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

export function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function baseConfig() {
  return {
    region: region(),
    endpoint: endpoint(),
    credentials: credentials(),
  };
}

/**
 * POST JSON with X-Amz-Target for lab JSON services that are not REST/XML
 * (CloudFront, Route53, ELBv2, Transfer PutFile, etc.).
 * @param {string} service SigV4 service name
 * @param {string} target X-Amz-Target value
 * @param {Record<string, unknown>} payload
 * @returns {Promise<{ status: number, body: string, json: any }>}
 */
export async function signedJsonTarget(service, target, payload = {}) {
  const body = JSON.stringify(payload ?? {});
  const ep = new URL(endpoint());
  const headers = {
    host: ep.host,
    "content-type": "application/x-amz-json-1.1",
    "x-amz-target": target,
  };
  const req = new HttpRequest({
    method: "POST",
    protocol: ep.protocol,
    hostname: ep.hostname,
    port: ep.port ? Number(ep.port) : undefined,
    path: "/",
    headers,
    body,
  });
  const signer = new SignatureV4({
    service,
    region: region(),
    credentials: credentials(),
    sha256: Sha256,
  });
  const signed = await signer.sign(req);
  const resp = await fetch(endpoint() + "/", {
    method: "POST",
    headers: signed.headers,
    body,
  });
  const text = await resp.text();
  let json = null;
  if (text) {
    try {
      json = JSON.parse(text);
    } catch {
      json = null;
    }
  }
  return { status: resp.status, body: text, json };
}

/**
 * SigV4-signed fetch for lab path APIs (CloudFront edge, Transfer home, OpenSearch query).
 * @param {string} service
 * @param {string} method
 * @param {string} path absolute path starting with /
 * @param {string|Uint8Array|undefined} body
 * @param {string|undefined} contentType
 */
export async function signedFetch(service, method, path, body, contentType) {
  const ep = new URL(endpoint());
  const headers = { host: ep.host };
  if (contentType) {
    headers["content-type"] = contentType;
  }
  const req = new HttpRequest({
    method,
    protocol: ep.protocol,
    hostname: ep.hostname,
    port: ep.port ? Number(ep.port) : undefined,
    path,
    headers,
    body: body ?? undefined,
  });
  const signer = new SignatureV4({
    service,
    region: region(),
    credentials: credentials(),
    sha256: Sha256,
  });
  const signed = await signer.sign(req);
  return fetch(`${ep.origin}${path}`, {
    method,
    headers: signed.headers,
    body: body ?? undefined,
  });
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

export function newKinesis() {
  return new KinesisClient(baseConfig());
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

export function newAPIGWv2() {
  return new ApiGatewayV2Client(baseConfig());
}

export function newAPIGW() {
  return new APIGatewayClient(baseConfig());
}

export function newCognito() {
  return new CognitoIdentityProviderClient(baseConfig());
}

export function newGlue() {
  return new GlueClient(baseConfig());
}

export function newAppConfig() {
  return new AppConfigClient(baseConfig());
}

export function newAppConfigData() {
  return new AppConfigDataClient(baseConfig());
}

export function newConfig() {
  return new ConfigServiceClient(baseConfig());
}

export function newSFN() {
  return new SFNClient(baseConfig());
}

export function newCloudTrail() {
  return new CloudTrailClient(baseConfig());
}

export function newFirehose() {
  return new FirehoseClient(baseConfig());
}

export function newOpenSearch() {
  return new OpenSearchClient(baseConfig());
}

export function newMQ() {
  return new MqClient(baseConfig());
}

export function newAppSync() {
  return new AppSyncClient(baseConfig());
}
