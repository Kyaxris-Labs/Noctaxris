def test_kms_encrypt_decrypt_round_trip(kms_client, unique_prefix):
    created = kms_client.create_key(Description=f"{unique_prefix}-key")
    key_id = created["KeyMetadata"]["KeyId"]
    assert key_id

    try:
        plain = b"noctaxris-kms"
        enc = kms_client.encrypt(KeyId=key_id, Plaintext=plain)
        assert enc.get("CiphertextBlob"), "Encrypt returned empty ciphertext"

        dec = kms_client.decrypt(CiphertextBlob=enc["CiphertextBlob"])
        assert dec["Plaintext"] == plain
    finally:
        try:
            kms_client.schedule_key_deletion(KeyId=key_id, PendingWindowInDays=7)
        except Exception:
            pass
