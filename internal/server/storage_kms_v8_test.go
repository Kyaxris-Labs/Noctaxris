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

func TestSecretsEncryptionContextCondition(t *testing.T) {
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
	keyARN, _ := meta["Arn"].(string)

	createRec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "ctx-secret",
		"SecretString": "classified",
		"KmsKeyId":     keyID,
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var secOut map[string]any
	_ = json.Unmarshal(createRec.Body.Bytes(), &secOut)
	secretARN, _ := secOut["ARN"].(string)

	allowMatch := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":"kms:*","Resource":"*"},
			{"Effect":"Deny","Principal":{"AWS":"*"},"Action":"kms:Decrypt","Resource":"*",
			 "Condition":{"StringNotEquals":{"kms:EncryptionContext:SecretARN":"%s"}}}
		]
	}`, testAccountID, secretARN)
	putPol := mustKMSJSON(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     allowMatch,
	}, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy match status=%d body=%q", putPol.Code, putPol.Body.String())
	}
	okGet := mustSecretsJSON(t, handler, "GetSecretValue", map[string]any{
		"SecretId": "ctx-secret",
	}, now)
	if okGet.Code != http.StatusOK {
		t.Fatalf("GetSecretValue matching context status=%d body=%q", okGet.Code, okGet.Body.String())
	}

	denyMismatch := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":["kms:Encrypt","kms:GenerateDataKey","kms:DescribeKey","kms:CreateGrant"],"Resource":"*"},
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":"kms:Decrypt","Resource":"*",
			 "Condition":{"StringEquals":{"kms:EncryptionContext:SecretARN":"%s-other"}}}
		]
	}`, testAccountID, testAccountID, secretARN)
	putDeny := mustKMSJSON(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     denyMismatch,
	}, now)
	if putDeny.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy mismatch status=%d body=%q", putDeny.Code, putDeny.Body.String())
	}
	badGet := mustSecretsJSON(t, handler, "GetSecretValue", map[string]any{
		"SecretId": "ctx-secret",
	}, now)
	if badGet.Code != http.StatusForbidden {
		t.Fatalf("GetSecretValue mismatched SecretARN condition status=%d want 403 body=%q keyARN=%s",
			badGet.Code, badGet.Body.String(), keyARN)
	}
}

func TestSSMEncryptionContextCondition(t *testing.T) {
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

	putRec := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name":  "/ctx/token",
		"Value": "s3cr3t",
		"Type":  "SecureString",
		"KeyId": keyID,
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutParameter status=%d body=%q", putRec.Code, putRec.Body.String())
	}
	paramARN := fmt.Sprintf("arn:aws:ssm:us-east-1:%s:parameter/ctx/token", testAccountID)

	allowMatch := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":"kms:*","Resource":"*"},
			{"Effect":"Deny","Principal":{"AWS":"*"},"Action":"kms:Decrypt","Resource":"*",
			 "Condition":{"StringNotEquals":{"kms:EncryptionContext:PARAMETER_ARN":"%s"}}}
		]
	}`, testAccountID, paramARN)
	putPol := mustKMSJSON(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     allowMatch,
	}, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy match status=%d body=%q", putPol.Code, putPol.Body.String())
	}
	okGet := mustSSMJSON(t, handler, "GetParameter", map[string]any{
		"Name":           "/ctx/token",
		"WithDecryption": true,
	}, now)
	if okGet.Code != http.StatusOK {
		t.Fatalf("GetParameter matching context status=%d body=%q", okGet.Code, okGet.Body.String())
	}

	denyMismatch := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":["kms:Encrypt","kms:DescribeKey","kms:CreateGrant"],"Resource":"*"},
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":"kms:Decrypt","Resource":"*",
			 "Condition":{"StringEquals":{"kms:EncryptionContext:PARAMETER_ARN":"%s-other"}}}
		]
	}`, testAccountID, testAccountID, paramARN)
	putDeny := mustKMSJSON(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     denyMismatch,
	}, now)
	if putDeny.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy mismatch status=%d body=%q", putDeny.Code, putDeny.Body.String())
	}
	badGet := mustSSMJSON(t, handler, "GetParameter", map[string]any{
		"Name":           "/ctx/token",
		"WithDecryption": true,
	}, now)
	if badGet.Code != http.StatusForbidden {
		t.Fatalf("GetParameter mismatched PARAMETER_ARN condition status=%d want 403 body=%q",
			badGet.Code, badGet.Body.String())
	}
}

func TestDynamoDBSSEKMSEncryptionContextAndCreateTableAuthz(t *testing.T) {
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

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "sse-ctx-table",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"SSESpecification": map[string]any{
			"Enabled":        true,
			"SSEType":        "KMS",
			"KMSMasterKeyId": keyID,
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable SSE status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	putItem := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "sse-ctx-table",
		"Item": map[string]any{
			"pk":   map[string]any{"S": "1"},
			"data": map[string]any{"S": "hello"},
		},
	}, now)
	if putItem.Code != http.StatusOK {
		t.Fatalf("PutItem status=%d body=%q", putItem.Code, putItem.Body.String())
	}

	allowMatch := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":"kms:*","Resource":"*"},
			{"Effect":"Deny","Principal":{"AWS":"*"},"Action":"kms:Decrypt","Resource":"*",
			 "Condition":{"StringNotEquals":{"kms:EncryptionContext:aws:dynamodb:tableName":"sse-ctx-table"}}}
		]
	}`, testAccountID)
	putPol := mustKMSJSON(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     allowMatch,
	}, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy match status=%d body=%q", putPol.Code, putPol.Body.String())
	}
	okGet := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "sse-ctx-table",
		"Key":       map[string]any{"pk": map[string]any{"S": "1"}},
	}, now)
	if okGet.Code != http.StatusOK {
		t.Fatalf("GetItem matching context status=%d body=%q", okGet.Code, okGet.Body.String())
	}

	denyMismatch := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":["kms:Encrypt","kms:GenerateDataKey","kms:DescribeKey","kms:CreateGrant"],"Resource":"*"},
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":"kms:Decrypt","Resource":"*",
			 "Condition":{"StringEquals":{"kms:EncryptionContext:aws:dynamodb:tableName":"other-table"}}}
		]
	}`, testAccountID, testAccountID)
	putDeny := mustKMSJSON(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     denyMismatch,
	}, now)
	if putDeny.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy mismatch status=%d body=%q", putDeny.Code, putDeny.Body.String())
	}
	badGet := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "sse-ctx-table",
		"Key":       map[string]any{"pk": map[string]any{"S": "1"}},
	}, now)
	if badGet.Code != http.StatusForbidden {
		t.Fatalf("GetItem mismatched tableName condition status=%d want 403 body=%q",
			badGet.Code, badGet.Body.String())
	}

	// CreateTable SSE requires DescribeKey + CreateGrant; deny CreateGrant blocks attach.
	createKey2 := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if createKey2.Code != http.StatusOK {
		t.Fatalf("CreateKey2 status=%d body=%q", createKey2.Code, createKey2.Body.String())
	}
	var keyOut2 map[string]any
	_ = json.Unmarshal(createKey2.Body.Bytes(), &keyOut2)
	meta2, _ := keyOut2["KeyMetadata"].(map[string]any)
	keyID2, _ := meta2["KeyId"].(string)
	denyGrant := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":["kms:DescribeKey","kms:Encrypt","kms:Decrypt","kms:GenerateDataKey"],"Resource":"*"},
			{"Effect":"Deny","Principal":{"AWS":"*"},"Action":"kms:CreateGrant","Resource":"*"}
		]
	}`, testAccountID)
	putGrantDeny := mustKMSJSON(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID2,
		"PolicyName": "default",
		"Policy":     denyGrant,
	}, now)
	if putGrantDeny.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy CreateGrant deny status=%d body=%q", putGrantDeny.Code, putGrantDeny.Body.String())
	}
	blocked := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "sse-blocked",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"SSESpecification": map[string]any{
			"Enabled":        true,
			"SSEType":        "KMS",
			"KMSMasterKeyId": keyID2,
		},
	}, now)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("CreateTable without CreateGrant status=%d want 403 body=%q", blocked.Code, blocked.Body.String())
	}
	_ = st
}

func TestS3SSEKMSCustomerContextRoundTrip(t *testing.T) {
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

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/kms-cust-ctx", nil, "s3", now, nil)
	customerCtx := base64.StdEncoding.EncodeToString([]byte(`{"Department":"Finance"}`))
	payload := []byte("hello-customer-ctx")
	putRec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/kms-cust-ctx/obj.txt", payload, "s3", now, map[string]string{
		"x-amz-server-side-encryption":                 "aws:kms",
		"x-amz-server-side-encryption-aws-kms-key-id":  keyID,
		"x-amz-server-side-encryption-context":         customerCtx,
	})
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutObject customer SSE context status=%d body=%q", putRec.Code, putRec.Body.String())
	}
	getRec := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/kms-cust-ctx/obj.txt", nil, "s3", now, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetObject customer SSE context status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	if getRec.Body.String() != string(payload) {
		t.Fatalf("GetObject body=%q want %q", getRec.Body.String(), payload)
	}

	objectARN := "arn:aws:s3:::kms-cust-ctx/obj.txt"
	denyMismatch := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":["kms:GenerateDataKey","kms:Encrypt","kms:DescribeKey","kms:CreateGrant"],"Resource":"*"},
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":"kms:Decrypt","Resource":"*",
			 "Condition":{"StringEquals":{"kms:EncryptionContext:Department":"HR","kms:EncryptionContext:aws:s3:arn":"%s"}}}
		]
	}`, testAccountID, testAccountID, objectARN)
	putDeny := mustKMSJSON(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     denyMismatch,
	}, now)
	if putDeny.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy mismatch status=%d body=%q", putDeny.Code, putDeny.Body.String())
	}
	badGet := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/kms-cust-ctx/obj.txt", nil, "s3", now, nil)
	if badGet.Code != http.StatusForbidden {
		t.Fatalf("GetObject mismatched Department condition status=%d want 403 body=%q",
			badGet.Code, badGet.Body.String())
	}
}

func TestPutKeyPolicyRejectsPrincipalLess(t *testing.T) {
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

	putPol := mustKMSJSON(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:*","Resource":"*"}]}`,
	}, now)
	if putPol.Code != http.StatusBadRequest || !strings.Contains(putPol.Body.String(), "Principal") {
		t.Fatalf("PutKeyPolicy Principal-less want 400 Principal error, status=%d body=%q",
			putPol.Code, putPol.Body.String())
	}
}
