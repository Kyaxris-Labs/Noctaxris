package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestKMSValidationAndGenerateDataKeyEdges(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	for _, action := range []string{
		"Encrypt", "DescribeKey", "EnableKey", "DisableKey",
		"GetKeyPolicy", "GenerateDataKey", "GenerateDataKeyWithoutPlaintext",
		"CreateGrant", "ListGrants", "EnableKeyRotation", "GetKeyRotationStatus",
		"Sign", "Verify", "GetPublicKey", "ListResourceTags",
	} {
		rec := mustKMSJSON(t, handler, action, map[string]any{}, now)
		if rec.Code == http.StatusOK {
			t.Fatalf("%s without KeyId should fail: %s", action, rec.Body.String())
		}
	}

	create := mustKMSJSON(t, handler, "CreateKey", map[string]any{
		"Description": "edges",
		"Tags":        []map[string]any{{"TagKey": "env", "TagValue": "lab"}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateKey %d %s", create.Code, create.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	meta, _ := created["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	gdk := mustKMSJSON(t, handler, "GenerateDataKeyWithoutPlaintext", map[string]any{
		"KeyId": keyID, "KeySpec": "AES_256",
	}, now)
	if gdk.Code != http.StatusOK {
		t.Fatalf("GenerateDataKeyWithoutPlaintext %d %s", gdk.Code, gdk.Body.String())
	}

	badAliasTarget := mustKMSJSON(t, handler, "CreateAlias", map[string]any{
		"AliasName": "alias/no-key", "TargetKeyId": "missing-key-id",
	}, now)
	if badAliasTarget.Code == http.StatusOK {
		t.Fatalf("CreateAlias bad target should fail")
	}

	enc := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId": keyID, "Plaintext": base64.StdEncoding.EncodeToString([]byte("edge")),
		"EncryptionContext": map[string]string{"a": "1"},
	}, now)
	if enc.Code != http.StatusOK {
		t.Fatalf("Encrypt %d %s", enc.Code, enc.Body.String())
	}
	var encOut map[string]any
	_ = json.Unmarshal(enc.Body.Bytes(), &encOut)
	blob, _ := encOut["CiphertextBlob"].(string)

	badDecCtx := mustKMSJSON(t, handler, "Decrypt", map[string]any{
		"CiphertextBlob":    blob,
		"EncryptionContext": map[string]string{"a": "wrong"},
	}, now)
	if badDecCtx.Code == http.StatusOK {
		t.Fatalf("Decrypt bad context should fail")
	}

	notFound := mustKMSJSON(t, handler, "DescribeKey", map[string]any{"KeyId": "no-such"}, now)
	if notFound.Code == http.StatusOK {
		t.Fatalf("DescribeKey missing should fail")
	}

	disable := mustKMSJSON(t, handler, "DisableKey", map[string]any{"KeyId": keyID}, now)
	if disable.Code != http.StatusOK {
		t.Fatalf("DisableKey %d %s", disable.Code, disable.Body.String())
	}
	encDisabled := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId": keyID, "Plaintext": base64.StdEncoding.EncodeToString([]byte("x")),
	}, now)
	if encDisabled.Code == http.StatusOK {
		t.Fatalf("Encrypt disabled key should fail")
	}
	enable := mustKMSJSON(t, handler, "EnableKey", map[string]any{"KeyId": keyID}, now)
	if enable.Code != http.StatusOK {
		t.Fatalf("EnableKey %d %s", enable.Code, enable.Body.String())
	}
}

func TestIAMEmptyParamNegatives(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	cases := []struct {
		body string
	}{
		{"Action=GetUser&Version=2010-05-08&UserName="},
		{"Action=DeleteUser&Version=2010-05-08&UserName="},
		{"Action=GetRole&Version=2010-05-08&RoleName="},
		{"Action=DeleteRole&Version=2010-05-08&RoleName="},
		{"Action=CreatePolicy&Version=2010-05-08&PolicyName=&PolicyDocument=%7B%7D"},
		{"Action=GetPolicy&Version=2010-05-08&PolicyArn="},
		{"Action=CreateGroup&Version=2010-05-08&GroupName="},
		{"Action=GetGroup&Version=2010-05-08&GroupName="},
		{"Action=CreateInstanceProfile&Version=2010-05-08&InstanceProfileName="},
		{"Action=GetInstanceProfile&Version=2010-05-08&InstanceProfileName="},
		{"Action=CreateOpenIDConnectProvider&Version=2010-05-08&Url="},
		{"Action=CreateSAMLProvider&Version=2010-05-08&Name=&SAMLMetadataDocument=e30="},
	}
	for _, tc := range cases {
		rec := iamForm(t, handler, tc.body, now)
		if rec.Code == http.StatusOK {
			t.Fatalf("body=%q got OK want non-OK body=%q", tc.body, rec.Body.String())
		}
	}

	// Happy path list ops still succeed (covers list branches)
	for _, body := range []string{
		"Action=ListUsers&Version=2010-05-08",
		"Action=ListRoles&Version=2010-05-08",
		"Action=ListGroups&Version=2010-05-08",
		"Action=ListPolicies&Version=2010-05-08",
		"Action=ListInstanceProfiles&Version=2010-05-08",
		"Action=ListOpenIDConnectProviders&Version=2010-05-08",
		"Action=ListSAMLProviders&Version=2010-05-08",
	} {
		rec := iamForm(t, handler, body, now)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%q", body, rec.Code, rec.Body.String())
		}
	}
}

func TestSecretsManagerPutListTagNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	empty := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name": "", "SecretString": "x",
	}, now)
	if empty.Code == http.StatusOK {
		t.Fatalf("CreateSecret empty name should fail")
	}

	create := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name": "ops-secret", "SecretString": "v1",
		"Tags": []map[string]any{{"Key": "env", "Value": "lab"}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateSecret %d %s", create.Code, create.Body.String())
	}
	dup := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name": "ops-secret", "SecretString": "v2",
	}, now)
	if dup.Code == http.StatusOK {
		t.Fatalf("CreateSecret dup should fail")
	}

	put := mustSecretsJSON(t, handler, "PutSecretValue", map[string]any{
		"SecretId": "ops-secret", "SecretString": "v2",
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutSecretValue %d %s", put.Code, put.Body.String())
	}
	putMiss := mustSecretsJSON(t, handler, "PutSecretValue", map[string]any{
		"SecretId": "no-secret", "SecretString": "x",
	}, now)
	if putMiss.Code == http.StatusOK {
		t.Fatalf("PutSecretValue missing should fail")
	}

	get := mustSecretsJSON(t, handler, "GetSecretValue", map[string]any{"SecretId": "ops-secret"}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "v2") {
		t.Fatalf("GetSecretValue %d %s", get.Code, get.Body.String())
	}
	desc := mustSecretsJSON(t, handler, "DescribeSecret", map[string]any{"SecretId": "ops-secret"}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeSecret %d %s", desc.Code, desc.Body.String())
	}
	list := mustSecretsJSON(t, handler, "ListSecrets", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "ops-secret") {
		t.Fatalf("ListSecrets %d %s", list.Code, list.Body.String())
	}

	tag := mustSecretsJSON(t, handler, "TagResource", map[string]any{
		"SecretId": "ops-secret",
		"Tags":     []map[string]any{{"Key": "team", "Value": "sec"}},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagResource %d %s", tag.Code, tag.Body.String())
	}
	untag := mustSecretsJSON(t, handler, "UntagResource", map[string]any{
		"SecretId": "ops-secret", "TagKeys": []string{"team"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource %d %s", untag.Code, untag.Body.String())
	}

	pol := mustSecretsJSON(t, handler, "PutResourcePolicy", map[string]any{
		"SecretId": "ops-secret",
		"ResourcePolicy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`,
	}, now)
	if pol.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy %d %s", pol.Code, pol.Body.String())
	}
	getPol := mustSecretsJSON(t, handler, "GetResourcePolicy", map[string]any{"SecretId": "ops-secret"}, now)
	if getPol.Code != http.StatusOK {
		t.Fatalf("GetResourcePolicy %d %s", getPol.Code, getPol.Body.String())
	}
	delPol := mustSecretsJSON(t, handler, "DeleteResourcePolicy", map[string]any{"SecretId": "ops-secret"}, now)
	if delPol.Code != http.StatusOK {
		t.Fatalf("DeleteResourcePolicy %d %s", delPol.Code, delPol.Body.String())
	}

	del := mustSecretsJSON(t, handler, "DeleteSecret", map[string]any{
		"SecretId": "ops-secret", "ForceDeleteWithoutRecovery": true,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteSecret %d %s", del.Code, del.Body.String())
	}
	getGone := mustSecretsJSON(t, handler, "GetSecretValue", map[string]any{"SecretId": "ops-secret"}, now)
	if getGone.Code == http.StatusOK {
		t.Fatalf("GetSecretValue after delete should fail")
	}
}

func TestLambdaUpdateCodeAndAliasNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-code-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-code-role"

	create := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "code-fn",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateFunction %d %s", create.Code, create.Body.String())
	}

	updCode := mustLambdaJSON(t, handler, "UpdateFunctionCode", map[string]any{
		"FunctionName": "code-fn",
		"ZipFile":      testLambdaZipB64(t),
	}, now)
	if updCode.Code != http.StatusOK {
		t.Fatalf("UpdateFunctionCode %d %s", updCode.Code, updCode.Body.String())
	}
	updMiss := mustLambdaJSON(t, handler, "UpdateFunctionCode", map[string]any{
		"FunctionName": "no-fn",
		"ZipFile":      testLambdaZipB64(t),
	}, now)
	if updMiss.Code == http.StatusOK {
		t.Fatalf("UpdateFunctionCode missing should fail")
	}
	badZip := mustLambdaJSON(t, handler, "UpdateFunctionCode", map[string]any{
		"FunctionName": "code-fn",
		"ZipFile":      "!!!not-b64!!!",
	}, now)
	if badZip.Code == http.StatusOK {
		t.Fatalf("UpdateFunctionCode bad zip should fail")
	}

	pub := mustLambdaJSON(t, handler, "PublishVersion", map[string]any{
		"FunctionName": "code-fn", "Description": "v1",
	}, now)
	if pub.Code != http.StatusOK {
		t.Fatalf("PublishVersion %d %s", pub.Code, pub.Body.String())
	}
	aliasBad := mustLambdaJSON(t, handler, "CreateAlias", map[string]any{
		"FunctionName": "code-fn", "Name": "live", "FunctionVersion": "99",
	}, now)
	if aliasBad.Code == http.StatusOK {
		t.Fatalf("CreateAlias bad version should fail")
	}
	alias := mustLambdaJSON(t, handler, "CreateAlias", map[string]any{
		"FunctionName": "code-fn", "Name": "live", "FunctionVersion": "1",
	}, now)
	if alias.Code != http.StatusOK {
		t.Fatalf("CreateAlias %d %s", alias.Code, alias.Body.String())
	}
	aliasDup := mustLambdaJSON(t, handler, "CreateAlias", map[string]any{
		"FunctionName": "code-fn", "Name": "live", "FunctionVersion": "1",
	}, now)
	if aliasDup.Code == http.StatusOK {
		t.Fatalf("CreateAlias dup should fail")
	}
	getAliasMiss := mustLambdaJSON(t, handler, "GetAlias", map[string]any{
		"FunctionName": "code-fn", "Name": "nope",
	}, now)
	if getAliasMiss.Code == http.StatusOK {
		t.Fatalf("GetAlias missing should fail")
	}
	updAliasMiss := mustLambdaJSON(t, handler, "UpdateAlias", map[string]any{
		"FunctionName": "code-fn", "Name": "nope", "FunctionVersion": "1",
	}, now)
	if updAliasMiss.Code == http.StatusOK {
		t.Fatalf("UpdateAlias missing should fail")
	}
	delAliasMiss := mustLambdaJSON(t, handler, "DeleteAlias", map[string]any{
		"FunctionName": "code-fn", "Name": "nope",
	}, now)
	if delAliasMiss.Code == http.StatusOK {
		t.Fatalf("DeleteAlias missing should fail")
	}

	_ = mustLambdaJSON(t, handler, "DeleteAlias", map[string]any{
		"FunctionName": "code-fn", "Name": "live",
	}, now)
	_ = mustLambdaJSON(t, handler, "DeleteFunction", map[string]any{"FunctionName": "code-fn"}, now)
}

func TestDynamoDBQueryFilterAndConditionNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "query-neg",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "sk", "AttributeType": "S"},
			{"AttributeName": "gsi1", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
			{"AttributeName": "sk", "KeyType": "RANGE"},
		},
		"GlobalSecondaryIndexes": []map[string]any{{
			"IndexName": "GSI1",
			"KeySchema": []map[string]any{
				{"AttributeName": "gsi1", "KeyType": "HASH"},
			},
			"Projection": map[string]any{"ProjectionType": "ALL"},
		}},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable %d %s", create.Code, create.Body.String())
	}

	for _, sk := range []string{"a", "b", "c"} {
		put := mustDynamoJSON(t, handler, "PutItem", map[string]any{
			"TableName": "query-neg",
			"Item": map[string]any{
				"pk":   map[string]any{"S": "u1"},
				"sk":   map[string]any{"S": sk},
				"gsi1": map[string]any{"S": "g"},
				"n":    map[string]any{"N": "1"},
			},
		}, now)
		if put.Code != http.StatusOK {
			t.Fatalf("PutItem %d %s", put.Code, put.Body.String())
		}
	}

	q := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "query-neg",
		"KeyConditionExpression": "pk = :pk",
		"FilterExpression":       "attribute_exists(n)",
		"ExpressionAttributeValues": map[string]any{
			":pk": map[string]any{"S": "u1"},
		},
		"Limit": 2,
	}, now)
	if q.Code != http.StatusOK {
		t.Fatalf("Query %d %s", q.Code, q.Body.String())
	}

	qGSI := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "query-neg",
		"IndexName":              "GSI1",
		"KeyConditionExpression": "gsi1 = :g",
		"ExpressionAttributeValues": map[string]any{
			":g": map[string]any{"S": "g"},
		},
	}, now)
	if qGSI.Code != http.StatusOK {
		t.Fatalf("Query GSI %d %s", qGSI.Code, qGSI.Body.String())
	}

	badQ := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName": "query-neg",
	}, now)
	if badQ.Code != http.StatusBadRequest {
		t.Fatalf("Query missing key condition want 400 got %d %s", badQ.Code, badQ.Body.String())
	}

	condPut := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "query-neg",
		"Item": map[string]any{
			"pk": map[string]any{"S": "u1"},
			"sk": map[string]any{"S": "a"},
		},
		"ConditionExpression": "attribute_not_exists(pk)",
	}, now)
	if condPut.Code == http.StatusOK {
		t.Fatalf("conditional PutItem should fail")
	}

	txGetBadKey := mustDynamoJSON(t, handler, "TransactGetItems", map[string]any{
		"TransactItems": []map[string]any{{
			"Get": map[string]any{
				"TableName": "query-neg",
				"Key":       map[string]any{"pk": map[string]any{"S": "u1"}},
			},
		}},
	}, now)
	if txGetBadKey.Code != http.StatusBadRequest {
		t.Fatalf("TransactGet missing range key want 400 got %d %s", txGetBadKey.Code, txGetBadKey.Body.String())
	}

	_ = mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "query-neg"}, now)
}

func TestSNSPublishSubscribeNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	topicARN := snsCreateTopic(t, handler, "ops-sns-topic", now)

	pubMissing := mustSNSQuery(t, handler,
		"Action=Publish&Version=2010-03-31&TopicArn="+url.QueryEscape("arn:aws:sns:"+testRegion+":"+testAccountID+":nope")+
			"&Message=hi",
		testAccessKey, testSecret, now)
	if pubMissing.Code == http.StatusOK {
		t.Fatalf("Publish missing topic should fail")
	}
	pub := mustSNSQuery(t, handler,
		"Action=Publish&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+"&Message=hello",
		testAccessKey, testSecret, now)
	if pub.Code != http.StatusOK {
		t.Fatalf("Publish %d %s", pub.Code, pub.Body.String())
	}

	subBad := mustSNSQuery(t, handler,
		"Action=Subscribe&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Protocol=email&Endpoint=",
		testAccessKey, testSecret, now)
	if subBad.Code == http.StatusOK {
		t.Fatalf("Subscribe empty endpoint should fail")
	}

	qARN := "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":sns-ops-q"
	_ = mustSQSJSON(t, handler, "CreateQueue", map[string]any{"QueueName": "sns-ops-q"}, now)
	sub := mustSNSQuery(t, handler,
		"Action=Subscribe&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Protocol=sqs&Endpoint="+url.QueryEscape(qARN),
		testAccessKey, testSecret, now)
	if sub.Code != http.StatusOK {
		t.Fatalf("Subscribe %d %s", sub.Code, sub.Body.String())
	}

	listBy := mustSNSQuery(t, handler,
		"Action=ListSubscriptionsByTopic&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN),
		testAccessKey, testSecret, now)
	if listBy.Code != http.StatusOK {
		t.Fatalf("ListSubscriptionsByTopic %d %s", listBy.Code, listBy.Body.String())
	}

	del := mustSNSQuery(t, handler,
		"Action=DeleteTopic&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN),
		testAccessKey, testSecret, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteTopic %d %s", del.Code, del.Body.String())
	}
}
