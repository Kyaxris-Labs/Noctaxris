"""Minimal Cognito USER_SRP_AUTH client (amazon-cognito-identity-js / warrant shape)."""

from __future__ import annotations

import base64
import hashlib
import hmac
import os
from datetime import datetime, timezone

_DAYS = ("Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun")
_MONTHS = (
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
)

SRP_N_HEX = (
    "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1"
    "29024E088A67CC74020BBEA63B139B22514A08798E3404DD"
    "EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245"
    "E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED"
    "EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D"
    "C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F"
    "83655D23DCA3AD961C62F356208552BB9ED529077096966D"
    "670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B"
    "E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9"
    "DE2BCBF6955817183995497CEA956AE515D2261898FA0510"
    "15728E5A8AAAC42DAD33170D04507A33A85521ABDF1CBA64"
    "ECFB850458DBEF0A8AEA71575D060C7DB3970F85A6E1E4C7"
    "ABF5AE8CDB0933D71E8C94E04A25619DCEE3D2261AD2EE6B"
    "F12FFA06D98A0864D87602733EC86A64521F2B18177B200C"
    "BBE117577A615D6C770988C0BAD946E208E24FA074E5AB31"
    "43DB5BFCE0FD108E4B82D120A93AD2CAFFFFFFFFFFFFFFFF"
)

SRP_N = int(SRP_N_HEX, 16)
SRP_G = 2


def _hash_sha256(buf: bytes) -> str:
    h = hashlib.sha256(buf).hexdigest()
    return h if len(h) >= 64 else ("0" * (64 - len(h))) + h


def _hex_hash(hex_string: str) -> str:
    try:
        return _hash_sha256(bytes.fromhex(hex_string))
    except ValueError:
        return _hash_sha256(b"")


SRP_K = int(_hex_hash("00" + SRP_N_HEX + "0" + "2"), 16)


def _pad_hex(v) -> str:
    if isinstance(v, int):
        h = format(v, "x").lower()
    else:
        h = str(v).strip().lower()
    if len(h) % 2 == 1:
        h = "0" + h
    elif h and h[0] in "89abcdef":
        h = "00" + h
    return h


def _pool_name(pool_id: str) -> str:
    i = pool_id.find("_")
    return pool_id[i + 1 :] if i >= 0 and i + 1 < len(pool_id) else pool_id


def _compute_hkdf(ikm: bytes, salt: bytes) -> bytes:
    prk = hmac.new(salt, ikm, hashlib.sha256).digest()
    info = b"Caldera Derived Key" + bytes([1])
    return hmac.new(prk, info, hashlib.sha256).digest()[:16]


def _private_x(pool_id: str, username: str, password: str, salt_hex: str) -> int:
    inner = _hash_sha256((_pool_name(pool_id) + username + ":" + password).encode())
    return int(_hex_hash(_pad_hex(salt_hex) + inner), 16)


def _calculate_u(a: int, b: int) -> int:
    return int(_hex_hash(_pad_hex(a) + _pad_hex(b)), 16)


def _srp_timestamp(at: datetime | None = None) -> str:
    t = (at or datetime.now(timezone.utc)).astimezone(timezone.utc)
    # Go: Mon Jan 2 15:04:05 UTC 2006 (no zero-pad on day); Mon=0
    day = _DAYS[t.weekday()]
    month = _MONTHS[t.month - 1]
    return (
        f"{day} {month} {t.day} "
        f"{t.hour:02d}:{t.minute:02d}:{t.second:02d} UTC {t.year}"
    )


class SDKSRPClient:
    def __init__(self, pool_id: str, username: str, password: str) -> None:
        self.pool_id = pool_id
        self.username = username
        self.password = password
        while True:
            a = int.from_bytes(os.urandom(128), "big") % SRP_N
            if a != 0:
                self.small_a = a
                break
        self.large_a = pow(SRP_G, self.small_a, SRP_N)

    def srp_a_hex(self) -> str:
        return format(self.large_a, "x").lower()

    def password_verifier_responses(
        self, params: dict[str, str], at: datetime | None = None
    ) -> dict[str, str]:
        user_id = (params.get("USER_ID_FOR_SRP") or self.username).strip()
        salt_hex = (params.get("SALT") or "").strip()
        srp_b_hex = (params.get("SRP_B") or "").strip()
        secret_block = (params.get("SECRET_BLOCK") or "").strip()
        b = int(srp_b_hex, 16)
        if b == 0:
            raise ValueError("invalid SRP_B")
        u = _calculate_u(self.large_a, b)
        if u == 0:
            raise ValueError("U cannot be zero")
        x = _private_x(self.pool_id, user_id, self.password, salt_hex)
        gx = pow(SRP_G, x, SRP_N)
        kv = (SRP_K * gx) % SRP_N
        base = (b - kv) % SRP_N
        exp = self.small_a + u * x
        s = pow(base, exp, SRP_N)
        ikm = bytes.fromhex(_pad_hex(s))
        salt = bytes.fromhex(_pad_hex(u))
        hkdf = _compute_hkdf(ikm, salt)
        secret_bytes = base64.b64decode(secret_block)
        ts = _srp_timestamp(at)
        msg = _pool_name(self.pool_id).encode() + user_id.encode() + secret_bytes + ts.encode()
        signature = base64.b64encode(hmac.new(hkdf, msg, hashlib.sha256).digest()).decode()
        return {
            "USERNAME": user_id,
            "PASSWORD_CLAIM_SECRET_BLOCK": secret_block,
            "PASSWORD_CLAIM_SIGNATURE": signature,
            "TIMESTAMP": ts,
        }
