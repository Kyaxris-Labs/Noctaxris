import { createHmac, createHash, randomBytes } from "node:crypto";

// Minimal Cognito USER_SRP_AUTH client (amazon-cognito-identity-js / warrant shape).

const SRP_N_HEX =
  "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
  "29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
  "EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
  "E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
  "EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
  "C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
  "83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
  "670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
  "E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
  "DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
  "15728E5A8AAAC42DAD33170D04507A33A85521ABDF1CBA64" +
  "ECFB850458DBEF0A8AEA71575D060C7DB3970F85A6E1E4C7" +
  "ABF5AE8CDB0933D71E8C94E04A25619DCEE3D2261AD2EE6B" +
  "F12FFA06D98A0864D87602733EC86A64521F2B18177B200C" +
  "BBE117577A615D6C770988C0BAD946E208E24FA074E5AB31" +
  "43DB5BFCE0FD108E4B82D120A93AD2CAFFFFFFFFFFFFFFFF";

const SRP_N = BigInt("0x" + SRP_N_HEX);
const SRP_G = 2n;
const SRP_K = BigInt("0x" + hexHash("00" + SRP_N_HEX + "0" + "2"));

function hashSHA256(buf) {
  const sum = createHash("sha256").update(buf).digest("hex");
  return sum.length >= 64 ? sum : "0".repeat(64 - sum.length) + sum;
}

function hexHash(hexString) {
  try {
    return hashSHA256(Buffer.from(hexString, "hex"));
  } catch {
    return hashSHA256(Buffer.alloc(0));
  }
}

function padHex(v) {
  let h =
    typeof v === "bigint"
      ? v.toString(16).toLowerCase()
      : String(v).trim().toLowerCase();
  if (h.length % 2 === 1) {
    h = "0" + h;
  } else if (h.length > 0 && "89abcdef".includes(h[0])) {
    h = "00" + h;
  }
  return h;
}

function poolName(poolID) {
  const i = poolID.indexOf("_");
  return i >= 0 && i + 1 < poolID.length ? poolID.slice(i + 1) : poolID;
}

function computeHKDF(ikm, salt) {
  const prk = createHmac("sha256", salt).update(ikm).digest();
  const info = Buffer.concat([Buffer.from("Caldera Derived Key"), Buffer.from([1])]);
  return createHmac("sha256", prk).update(info).digest().subarray(0, 16);
}

function privateX(poolID, username, password, saltHex) {
  const inner = hashSHA256(
    Buffer.from(poolName(poolID) + username + ":" + password),
  );
  return BigInt("0x" + hexHash(padHex(saltHex) + inner));
}

function calculateU(a, b) {
  return BigInt("0x" + hexHash(padHex(a) + padHex(b)));
}

function modPow(base, exp, mod) {
  let result = 1n;
  let b = ((base % mod) + mod) % mod;
  let e = exp;
  while (e > 0n) {
    if (e & 1n) result = (result * b) % mod;
    b = (b * b) % mod;
    e >>= 1n;
  }
  return result;
}

function srpTimestamp(at = new Date()) {
  const days = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  const months = [
    "Jan",
    "Feb",
    "Mar",
    "Apr",
    "May",
    "Jun",
    "Jul",
    "Aug",
    "Sep",
    "Oct",
    "Nov",
    "Dec",
  ];
  const d = new Date(at);
  const day = String(d.getUTCDate());
  const hh = String(d.getUTCHours()).padStart(2, "0");
  const mm = String(d.getUTCMinutes()).padStart(2, "0");
  const ss = String(d.getUTCSeconds()).padStart(2, "0");
  return `${days[d.getUTCDay()]} ${months[d.getUTCMonth()]} ${day} ${hh}:${mm}:${ss} UTC ${d.getUTCFullYear()}`;
}

function randomSmallA() {
  for (;;) {
    const buf = randomBytes(128);
    let a = BigInt("0x" + buf.toString("hex")) % SRP_N;
    if (a !== 0n) return a;
  }
}

export function newSDKSRPClient(poolID, username, password) {
  const smallA = randomSmallA();
  const largeA = modPow(SRP_G, smallA, SRP_N);
  return {
    srpAHex() {
      return largeA.toString(16).toLowerCase();
    },
    passwordVerifierResponses(params, at = new Date()) {
      const userID = (params.USER_ID_FOR_SRP || username).trim();
      const saltHex = (params.SALT || "").trim();
      const srpBHex = (params.SRP_B || "").trim();
      const secretBlock = (params.SECRET_BLOCK || "").trim();
      const B = BigInt("0x" + srpBHex);
      if (B === 0n) throw new Error("invalid SRP_B");
      const u = calculateU(largeA, B);
      if (u === 0n) throw new Error("U cannot be zero");
      const x = privateX(poolID, userID, password, saltHex);
      const gx = modPow(SRP_G, x, SRP_N);
      const kv = (SRP_K * gx) % SRP_N;
      let base = (B - kv) % SRP_N;
      if (base < 0n) base += SRP_N;
      const exp = smallA + u * x;
      const S = modPow(base, exp, SRP_N);
      const ikm = Buffer.from(padHex(S), "hex");
      const salt = Buffer.from(padHex(u), "hex");
      const hkdf = computeHKDF(ikm, salt);
      const secretBytes = Buffer.from(secretBlock, "base64");
      const ts = srpTimestamp(at);
      const msg = Buffer.concat([
        Buffer.from(poolName(poolID)),
        Buffer.from(userID),
        secretBytes,
        Buffer.from(ts),
      ]);
      const signature = createHmac("sha256", hkdf).update(msg).digest("base64");
      return {
        USERNAME: userID,
        PASSWORD_CLAIM_SECRET_BLOCK: secretBlock,
        PASSWORD_CLAIM_SIGNATURE: signature,
        TIMESTAMP: ts,
      };
    },
  };
}
