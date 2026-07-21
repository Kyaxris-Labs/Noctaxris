package server_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestKMSEncryptionContextBoundAsAAD(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := createOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	plain := base64.StdEncoding.EncodeToString([]byte("ctx-secret"))
	encRec := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":     keyID,
		"Plaintext": plain,
		"EncryptionContext": map[string]any{
			"purpose": "lab",
			"stage":   "test",
		},
	}, now)
	if encRec.Code != http.StatusOK {
		t.Fatalf("Encrypt status=%d body=%q", encRec.Code, encRec.Body.String())
	}
	var encOut map[string]any
	if err := json.Unmarshal(encRec.Body.Bytes(), &encOut); err != nil {
		t.Fatal(err)
	}
	blob, _ := encOut["CiphertextBlob"].(string)

	okDec := mustKMSJSON(t, handler, "Decrypt", map[string]any{
		"KeyId":          keyID,
		"CiphertextBlob": blob,
		"EncryptionContext": map[string]any{
			"stage":   "test",
			"purpose": "lab",
		},
	}, now)
	if okDec.Code != http.StatusOK {
		t.Fatalf("Decrypt matching context status=%d body=%q", okDec.Code, okDec.Body.String())
	}

	badDec := mustKMSJSON(t, handler, "Decrypt", map[string]any{
		"KeyId":          keyID,
		"CiphertextBlob": blob,
		"EncryptionContext": map[string]any{
			"purpose": "other",
		},
	}, now)
	if badDec.Code != http.StatusBadRequest || !strings.Contains(badDec.Body.String(), "InvalidCiphertextException") {
		t.Fatalf("Decrypt mismatched context want InvalidCiphertextException, status=%d body=%q",
			badDec.Code, badDec.Body.String())
	}

	emptyDec := mustKMSJSON(t, handler, "Decrypt", map[string]any{
		"KeyId":          keyID,
		"CiphertextBlob": blob,
	}, now)
	if emptyDec.Code != http.StatusBadRequest || !strings.Contains(emptyDec.Body.String(), "InvalidCiphertextException") {
		t.Fatalf("Decrypt empty context want InvalidCiphertextException, status=%d body=%q",
			emptyDec.Code, emptyDec.Body.String())
	}
}

func TestSecretsGetRequiresKMSDecrypt(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createKey := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if createKey.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createKey.Code, createKey.Body.String())
	}
	var keyOut map[string]any
	_ = json.Unmarshal(createKey.Body.Bytes(), &keyOut)
	meta, _ := keyOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	createRec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "kms-gated",
		"SecretString": "classified",
		"KmsKeyId":     keyID,
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	_, guestARN, err := st.CreateUser(testAccountID, "sm-guest")
	if err != nil {
		t.Fatal(err)
	}
	guestAKID, guestSecret, err := st.CreateUserAccessKey(testAccountID, "sm-guest")
	if err != nil {
		t.Fatal(err)
	}
	allowSecret := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`,
		guestARN,
	)
	putPol := mustSecretsJSON(t, handler, "PutResourcePolicy", map[string]any{
		"SecretId":       "kms-gated",
		"ResourcePolicy": allowSecret,
	}, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}

	// Key policy allows only account root (default). Guest has secretsmanager Allow but
	// no identity kms:Decrypt → EvaluateKMS denies.
	denyRec := mustSecretsJSONWithCreds(t, handler, "GetSecretValue", map[string]any{
		"SecretId": "kms-gated",
	}, guestAKID, guestSecret, now)
	if denyRec.Code != http.StatusForbidden {
		t.Fatalf("GetSecretValue without kms:Decrypt status=%d want 403 body=%q", denyRec.Code, denyRec.Body.String())
	}

	if err := st.PutInlinePolicy(guestARN, "kms-decrypt",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:Decrypt","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	okRec := mustSecretsJSONWithCreds(t, handler, "GetSecretValue", map[string]any{
		"SecretId": "kms-gated",
	}, guestAKID, guestSecret, now)
	if okRec.Code != http.StatusOK {
		t.Fatalf("GetSecretValue with kms:Decrypt status=%d want 200 body=%q", okRec.Code, okRec.Body.String())
	}
}

func TestSSMSecureStringGetRequiresKMSDecrypt(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createKey := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if createKey.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createKey.Code, createKey.Body.String())
	}
	var keyOut map[string]any
	_ = json.Unmarshal(createKey.Body.Bytes(), &keyOut)
	meta, _ := keyOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	putRec := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name":  "/secure/token",
		"Value": "s3cr3t",
		"Type":  "SecureString",
		"KeyId": keyID,
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutParameter status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	_, guestARN, err := st.CreateUser(testAccountID, "ssm-guest")
	if err != nil {
		t.Fatal(err)
	}
	guestAKID, guestSecret, err := st.CreateUserAccessKey(testAccountID, "ssm-guest")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(guestARN, "ssm-get",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ssm:GetParameter","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}

	denyRec := mustSSMJSONWithCreds(t, handler, "GetParameter", map[string]any{
		"Name":           "/secure/token",
		"WithDecryption": true,
	}, guestAKID, guestSecret, now)
	if denyRec.Code != http.StatusForbidden {
		t.Fatalf("GetParameter without kms:Decrypt status=%d want 403 body=%q", denyRec.Code, denyRec.Body.String())
	}

	if err := st.PutInlinePolicy(guestARN, "kms-decrypt",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:Decrypt","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	okRec := mustSSMJSONWithCreds(t, handler, "GetParameter", map[string]any{
		"Name":           "/secure/token",
		"WithDecryption": true,
	}, guestAKID, guestSecret, now)
	if okRec.Code != http.StatusOK {
		t.Fatalf("GetParameter with kms:Decrypt status=%d want 200 body=%q", okRec.Code, okRec.Body.String())
	}
}

func TestDynamoDBFilterExpressionScanQuery(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "filter-items",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	for _, item := range []map[string]any{
		{"pk": map[string]any{"S": "1"}, "status": map[string]any{"S": "active"}},
		{"pk": map[string]any{"S": "2"}, "status": map[string]any{"S": "idle"}},
	} {
		put := mustDynamoJSON(t, handler, "PutItem", map[string]any{
			"TableName": "filter-items",
			"Item":      item,
		}, now)
		if put.Code != http.StatusOK {
			t.Fatalf("PutItem status=%d body=%q", put.Code, put.Body.String())
		}
	}

	scanRec := mustDynamoJSON(t, handler, "Scan", map[string]any{
		"TableName":        "filter-items",
		"FilterExpression": "#s = :v",
		"ExpressionAttributeNames": map[string]any{
			"#s": "status",
		},
		"ExpressionAttributeValues": map[string]any{
			":v": map[string]any{"S": "active"},
		},
	}, now)
	if scanRec.Code != http.StatusOK {
		t.Fatalf("Scan status=%d body=%q", scanRec.Code, scanRec.Body.String())
	}
	var scanOut map[string]any
	if err := json.Unmarshal(scanRec.Body.Bytes(), &scanOut); err != nil {
		t.Fatal(err)
	}
	items, _ := scanOut["Items"].([]any)
	if len(items) != 1 {
		t.Fatalf("Scan filtered want 1 item, got %d body=%q", len(items), scanRec.Body.String())
	}

	unsupported := mustDynamoJSON(t, handler, "Scan", map[string]any{
		"TableName":        "filter-items",
		"FilterExpression": "size(tags) > :n",
		"ExpressionAttributeValues": map[string]any{
			":n": map[string]any{"N": "1"},
		},
	}, now)
	if unsupported.Code != http.StatusBadRequest || !strings.Contains(unsupported.Body.String(), "ValidationException") {
		t.Fatalf("unsupported FilterExpression want ValidationException, status=%d body=%q",
			unsupported.Code, unsupported.Body.String())
	}

	queryRec := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "filter-items",
		"KeyConditionExpression": "pk = :pk",
		"FilterExpression":       "status = :st",
		"ExpressionAttributeValues": map[string]any{
			":pk": map[string]any{"S": "2"},
			":st": map[string]any{"S": "active"},
		},
	}, now)
	if queryRec.Code != http.StatusOK {
		t.Fatalf("Query status=%d body=%q", queryRec.Code, queryRec.Body.String())
	}
	var queryOut map[string]any
	_ = json.Unmarshal(queryRec.Body.Bytes(), &queryOut)
	qItems, _ := queryOut["Items"].([]any)
	if len(qItems) != 0 {
		t.Fatalf("Query filtered want 0 items, got %d body=%q", len(qItems), queryRec.Body.String())
	}
}

func TestS3SSEKMSEncryptionContextObjectARN(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createKey := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if createKey.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createKey.Code, createKey.Body.String())
	}
	var keyOut map[string]any
	_ = json.Unmarshal(createKey.Body.Bytes(), &keyOut)
	meta, _ := keyOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/kms-ctx-bucket", nil, "s3", now, nil)
	payload := []byte("hello-sse-ctx")
	putRec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/kms-ctx-bucket/obj.txt", payload, "s3", now, map[string]string{
		"x-amz-server-side-encryption":                "aws:kms",
		"x-amz-server-side-encryption-aws-kms-key-id": keyID,
	})
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutObject SSE-KMS status=%d body=%q", putRec.Code, putRec.Body.String())
	}
	getRec := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/kms-ctx-bucket/obj.txt", nil, "s3", now, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetObject SSE-KMS status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	if getRec.Body.String() != string(payload) {
		t.Fatalf("GetObject body=%q want %q", getRec.Body.String(), payload)
	}
}
