import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateKeyCommand,
  DecryptCommand,
  EncryptCommand,
  ScheduleKeyDeletionCommand,
} from "@aws-sdk/client-kms";
import { newKMS, requireReady, uniquePrefix } from "../lib/helpers.mjs";

test("KMS encrypt decrypt round-trip", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newKMS();
  const created = await client.send(
    new CreateKeyCommand({ Description: `${uniquePrefix()}-key` }),
  );
  const keyId = created.KeyMetadata?.KeyId;
  assert.ok(keyId, "CreateKey missing KeyId");

  t.after(async () => {
    try {
      await client.send(
        new ScheduleKeyDeletionCommand({
          KeyId: keyId,
          PendingWindowInDays: 7,
        }),
      );
    } catch {
      /* ignore */
    }
  });

  const plain = Buffer.from("noctaxris-kms");
  const enc = await client.send(
    new EncryptCommand({ KeyId: keyId, Plaintext: plain }),
  );
  assert.ok(enc.CiphertextBlob?.length, "Encrypt returned empty ciphertext");

  const dec = await client.send(
    new DecryptCommand({ CiphertextBlob: enc.CiphertextBlob }),
  );
  assert.deepEqual(Buffer.from(dec.Plaintext), plain);
});
