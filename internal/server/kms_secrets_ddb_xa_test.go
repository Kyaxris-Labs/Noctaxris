package server_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustKMSJSONWithCreds(
	t *testing.T,
	handler http.Handler,
	target string,
	payload map[string]any,
	akid, secret string,
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "TrentService."+target)
	signHeader(t, req, raw, akid, secret, testRegion, "kms", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func mustDynamoJSONWithCreds(
	t *testing.T,
	handler http.Handler,
	target string,
	payload map[string]any,
	akid, secret string,
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810."+target)
	signHeader(t, req, raw, akid, secret, testRegion, "dynamodb", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestKMSCrossAccountIdentityAndKeyPolicyAllow(t *testing.T) {
	handler, st, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustKMSJSONWithCreds(t, handler, "CreateKey", map[string]any{}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := createOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)
	keyARN, _ := meta["Arn"].(string)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:Encrypt","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "kmsenc", identityAllow); err != nil {
		t.Fatal(err)
	}
	keyPolicy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"kms:Encrypt","Resource":"*"}]}`,
		callerUserARN,
	)
	putPol := mustKMSJSONWithCreds(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     keyPolicy,
	}, ownerAKID, ownerSecret, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}

	encRec := mustKMSJSONWithCreds(t, handler, "Encrypt", map[string]any{
		"KeyId":     keyARN,
		"Plaintext": base64.StdEncoding.EncodeToString([]byte("xa-kms")),
	}, callerAKID, callerSecret, now)
	if encRec.Code != http.StatusOK {
		t.Fatalf("Encrypt status=%d want 200 body=%q", encRec.Code, encRec.Body.String())
	}
}

func TestKMSCrossAccountIdentityOnlyDeny(t *testing.T) {
	handler, st, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustKMSJSONWithCreds(t, handler, "CreateKey", map[string]any{}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	_ = json.Unmarshal(createRec.Body.Bytes(), &createOut)
	meta, _ := createOut["KeyMetadata"].(map[string]any)
	keyARN, _ := meta["Arn"].(string)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:Encrypt","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "kmsenc", identityAllow); err != nil {
		t.Fatal(err)
	}

	encRec := mustKMSJSONWithCreds(t, handler, "Encrypt", map[string]any{
		"KeyId":     keyARN,
		"Plaintext": base64.StdEncoding.EncodeToString([]byte("nope")),
	}, callerAKID, callerSecret, now)
	if encRec.Code != http.StatusForbidden {
		t.Fatalf("Encrypt status=%d want 403 body=%q", encRec.Code, encRec.Body.String())
	}
}

func TestKMSCrossAccountPolicyOnlyDeny(t *testing.T) {
	handler, _, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustKMSJSONWithCreds(t, handler, "CreateKey", map[string]any{}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	_ = json.Unmarshal(createRec.Body.Bytes(), &createOut)
	meta, _ := createOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)
	keyARN, _ := meta["Arn"].(string)

	keyPolicy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"kms:Encrypt","Resource":"*"}]}`,
		callerUserARN,
	)
	putPol := mustKMSJSONWithCreds(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     keyPolicy,
	}, ownerAKID, ownerSecret, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}

	encRec := mustKMSJSONWithCreds(t, handler, "Encrypt", map[string]any{
		"KeyId":     keyARN,
		"Plaintext": base64.StdEncoding.EncodeToString([]byte("nope")),
	}, callerAKID, callerSecret, now)
	if encRec.Code != http.StatusForbidden {
		t.Fatalf("Encrypt status=%d want 403 body=%q", encRec.Code, encRec.Body.String())
	}
}

func TestSecretsCrossAccountIdentityAndPolicyAllow(t *testing.T) {
	handler, st, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSecretsJSONWithCreds(t, handler, "CreateSecret", map[string]any{
		"Name":         "xa-secret",
		"SecretString": "secret-xa",
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	_ = json.Unmarshal(createRec.Body.Bytes(), &createOut)
	secretARN, _ := createOut["ARN"].(string)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "smget", identityAllow); err != nil {
		t.Fatal(err)
	}
	policy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`,
		callerUserARN,
	)
	putPol := mustSecretsJSONWithCreds(t, handler, "PutResourcePolicy", map[string]any{
		"SecretId":       "xa-secret",
		"ResourcePolicy": policy,
	}, ownerAKID, ownerSecret, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}

	getRec := mustSecretsJSONWithCreds(t, handler, "GetSecretValue", map[string]any{
		"SecretId": secretARN,
	}, callerAKID, callerSecret, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetSecretValue status=%d want 200 body=%q", getRec.Code, getRec.Body.String())
	}
}

func TestSecretsCrossAccountIdentityOnlyDeny(t *testing.T) {
	handler, st, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSecretsJSONWithCreds(t, handler, "CreateSecret", map[string]any{
		"Name":         "xa-secret-id",
		"SecretString": "nope",
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	_ = json.Unmarshal(createRec.Body.Bytes(), &createOut)
	secretARN, _ := createOut["ARN"].(string)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "smget", identityAllow); err != nil {
		t.Fatal(err)
	}

	getRec := mustSecretsJSONWithCreds(t, handler, "GetSecretValue", map[string]any{
		"SecretId": secretARN,
	}, callerAKID, callerSecret, now)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("GetSecretValue status=%d want 403 body=%q", getRec.Code, getRec.Body.String())
	}
}

func TestSecretsCrossAccountPolicyOnlyDeny(t *testing.T) {
	handler, _, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSecretsJSONWithCreds(t, handler, "CreateSecret", map[string]any{
		"Name":         "xa-secret-pol",
		"SecretString": "nope",
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	_ = json.Unmarshal(createRec.Body.Bytes(), &createOut)
	secretARN, _ := createOut["ARN"].(string)

	policy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`,
		callerUserARN,
	)
	putPol := mustSecretsJSONWithCreds(t, handler, "PutResourcePolicy", map[string]any{
		"SecretId":       "xa-secret-pol",
		"ResourcePolicy": policy,
	}, ownerAKID, ownerSecret, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}

	getRec := mustSecretsJSONWithCreds(t, handler, "GetSecretValue", map[string]any{
		"SecretId": secretARN,
	}, callerAKID, callerSecret, now)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("GetSecretValue status=%d want 403 body=%q", getRec.Code, getRec.Body.String())
	}
}

func TestDynamoDBCrossAccountIdentityAndPolicyAllow(t *testing.T) {
	handler, st, ownerAccount, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSONWithCreds(t, handler, "CreateTable", map[string]any{
		"TableName":            "xa-ddb",
		"AttributeDefinitions": []map[string]string{{"AttributeName": "pk", "AttributeType": "S"}},
		"KeySchema":            []map[string]string{{"AttributeName": "pk", "KeyType": "HASH"}},
		"BillingMode":          "PAY_PER_REQUEST",
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	tableARN := store.TableARN(ownerAccount, testRegion, "xa-ddb")

	putItem := mustDynamoJSONWithCreds(t, handler, "PutItem", map[string]any{
		"TableName": "xa-ddb",
		"Item":      map[string]any{"pk": map[string]string{"S": "1"}, "v": map[string]string{"S": "xa"}},
	}, ownerAKID, ownerSecret, now)
	if putItem.Code != http.StatusOK {
		t.Fatalf("PutItem status=%d body=%q", putItem.Code, putItem.Body.String())
	}

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"dynamodb:GetItem","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "ddbget", identityAllow); err != nil {
		t.Fatal(err)
	}
	policy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"dynamodb:GetItem","Resource":"*"}]}`,
		callerUserARN,
	)
	putPol := mustDynamoJSONWithCreds(t, handler, "PutResourcePolicy", map[string]any{
		"ResourceArn": tableARN,
		"Policy":      policy,
	}, ownerAKID, ownerSecret, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}

	getRec := mustDynamoJSONWithCreds(t, handler, "GetItem", map[string]any{
		"TableName": tableARN,
		"Key":       map[string]any{"pk": map[string]string{"S": "1"}},
	}, callerAKID, callerSecret, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetItem status=%d want 200 body=%q", getRec.Code, getRec.Body.String())
	}
}

func TestDynamoDBCrossAccountIdentityOnlyDeny(t *testing.T) {
	handler, st, ownerAccount, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSONWithCreds(t, handler, "CreateTable", map[string]any{
		"TableName":            "xa-ddb-id",
		"AttributeDefinitions": []map[string]string{{"AttributeName": "pk", "AttributeType": "S"}},
		"KeySchema":            []map[string]string{{"AttributeName": "pk", "KeyType": "HASH"}},
		"BillingMode":          "PAY_PER_REQUEST",
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	tableARN := store.TableARN(ownerAccount, testRegion, "xa-ddb-id")

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"dynamodb:GetItem","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "ddbget", identityAllow); err != nil {
		t.Fatal(err)
	}

	getRec := mustDynamoJSONWithCreds(t, handler, "GetItem", map[string]any{
		"TableName": tableARN,
		"Key":       map[string]any{"pk": map[string]string{"S": "1"}},
	}, callerAKID, callerSecret, now)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("GetItem status=%d want 403 body=%q", getRec.Code, getRec.Body.String())
	}
}

func TestDynamoDBCrossAccountPolicyOnlyDeny(t *testing.T) {
	handler, _, ownerAccount, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSONWithCreds(t, handler, "CreateTable", map[string]any{
		"TableName":            "xa-ddb-pol",
		"AttributeDefinitions": []map[string]string{{"AttributeName": "pk", "AttributeType": "S"}},
		"KeySchema":            []map[string]string{{"AttributeName": "pk", "KeyType": "HASH"}},
		"BillingMode":          "PAY_PER_REQUEST",
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	tableARN := store.TableARN(ownerAccount, testRegion, "xa-ddb-pol")

	policy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"dynamodb:GetItem","Resource":"*"}]}`,
		callerUserARN,
	)
	putPol := mustDynamoJSONWithCreds(t, handler, "PutResourcePolicy", map[string]any{
		"ResourceArn": tableARN,
		"Policy":      policy,
	}, ownerAKID, ownerSecret, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}

	getRec := mustDynamoJSONWithCreds(t, handler, "GetItem", map[string]any{
		"TableName": tableARN,
		"Key":       map[string]any{"pk": map[string]string{"S": "1"}},
	}, callerAKID, callerSecret, now)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("GetItem status=%d want 403 body=%q", getRec.Code, getRec.Body.String())
	}
}
