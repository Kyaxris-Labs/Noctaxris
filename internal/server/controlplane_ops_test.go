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

func TestDynamoDBCreateTableValidationAndAES256(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	emptyName := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": " ",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if emptyName.Code != http.StatusBadRequest {
		t.Fatalf("empty name status=%d body=%q", emptyName.Code, emptyName.Body.String())
	}

	noHash := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName":            "no-hash",
		"AttributeDefinitions": []map[string]any{},
		"KeySchema":            []map[string]any{},
	}, now)
	if noHash.Code != http.StatusBadRequest {
		t.Fatalf("no hash status=%d body=%q", noHash.Code, noHash.Body.String())
	}

	badSSE := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "bad-sse",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"SSESpecification": map[string]any{
			"Enabled": true,
			"SSEType": "BOGUS",
		},
	}, now)
	if badSSE.Code != http.StatusBadRequest {
		t.Fatalf("bad sse status=%d body=%q", badSSE.Code, badSSE.Body.String())
	}

	kmsMissing := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "kms-missing",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"SSESpecification": map[string]any{
			"Enabled": true,
			"SSEType": "KMS",
		},
	}, now)
	if kmsMissing.Code != http.StatusBadRequest {
		t.Fatalf("kms missing status=%d body=%q", kmsMissing.Code, kmsMissing.Body.String())
	}

	aes := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "aes-table",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"SSESpecification": map[string]any{
			"Enabled": true,
			"SSEType": "AES256",
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if aes.Code != http.StatusOK {
		t.Fatalf("AES256 CreateTable status=%d body=%q", aes.Code, aes.Body.String())
	}
	dup := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "aes-table",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if dup.Code != http.StatusBadRequest || !strings.Contains(dup.Body.String(), "ResourceInUseException") {
		t.Fatalf("dup table status=%d body=%q", dup.Code, dup.Body.String())
	}

	badStream := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "bad-stream",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"StreamSpecification": map[string]any{
			"StreamEnabled":  true,
			"StreamViewType": "NOT_A_VIEW",
		},
	}, now)
	if badStream.Code != http.StatusBadRequest {
		t.Fatalf("bad stream status=%d body=%q", badStream.Code, badStream.Body.String())
	}

	tooManyGSI := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "too-many-gsi",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "g1", "AttributeType": "S"},
			{"AttributeName": "g2", "AttributeType": "S"},
			{"AttributeName": "g3", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"GlobalSecondaryIndexes": []map[string]any{
			{"IndexName": "G1", "KeySchema": []map[string]any{{"AttributeName": "g1", "KeyType": "HASH"}}, "Projection": map[string]any{"ProjectionType": "ALL"}},
			{"IndexName": "G2", "KeySchema": []map[string]any{{"AttributeName": "g2", "KeyType": "HASH"}}, "Projection": map[string]any{"ProjectionType": "ALL"}},
			{"IndexName": "G3", "KeySchema": []map[string]any{{"AttributeName": "g3", "KeyType": "HASH"}}, "Projection": map[string]any{"ProjectionType": "ALL"}},
		},
	}, now)
	if tooManyGSI.Code != http.StatusBadRequest {
		t.Fatalf("too many gsi status=%d body=%q", tooManyGSI.Code, tooManyGSI.Body.String())
	}

	del := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "aes-table"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteTable status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestDynamoDBPartiQLUpdateAndTransactNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "partiql-upd",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", create.Code, create.Body.String())
	}

	ins := mustDynamoJSON(t, handler, "ExecuteStatement", map[string]any{
		"Statement": `INSERT INTO "partiql-upd" VALUE {'pk': 'a', 'n': 1}`,
	}, now)
	if ins.Code != http.StatusOK {
		t.Fatalf("INSERT status=%d body=%q", ins.Code, ins.Body.String())
	}
	upd := mustDynamoJSON(t, handler, "ExecuteStatement", map[string]any{
		"Statement": `UPDATE "partiql-upd" SET n=2 WHERE pk='a'`,
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UPDATE status=%d body=%q", upd.Code, upd.Body.String())
	}
	selMissing := mustDynamoJSON(t, handler, "ExecuteStatement", map[string]any{
		"Statement": `SELECT * FROM "partiql-upd" WHERE pk='missing'`,
	}, now)
	if selMissing.Code != http.StatusOK {
		t.Fatalf("SELECT missing status=%d body=%q", selMissing.Code, selMissing.Body.String())
	}

	emptyTxGet := mustDynamoJSON(t, handler, "TransactGetItems", map[string]any{}, now)
	if emptyTxGet.Code != http.StatusBadRequest {
		t.Fatalf("empty TransactGetItems status=%d body=%q", emptyTxGet.Code, emptyTxGet.Body.String())
	}
	badTxGet := mustDynamoJSON(t, handler, "TransactGetItems", map[string]any{
		"TransactItems": []map[string]any{{"Put": map[string]any{}}},
	}, now)
	if badTxGet.Code != http.StatusBadRequest {
		t.Fatalf("bad TransactGetItems status=%d body=%q", badTxGet.Code, badTxGet.Body.String())
	}
	txGetOK := mustDynamoJSON(t, handler, "TransactGetItems", map[string]any{
		"TransactItems": []map[string]any{{
			"Get": map[string]any{
				"TableName": "partiql-upd",
				"Key":       map[string]any{"pk": map[string]any{"S": "a"}},
			},
		}},
	}, now)
	if txGetOK.Code != http.StatusOK {
		t.Fatalf("TransactGetItems status=%d body=%q", txGetOK.Code, txGetOK.Body.String())
	}

	emptyTxWrite := mustDynamoJSON(t, handler, "TransactWriteItems", map[string]any{}, now)
	if emptyTxWrite.Code != http.StatusBadRequest {
		t.Fatalf("empty TransactWriteItems status=%d body=%q", emptyTxWrite.Code, emptyTxWrite.Body.String())
	}

	del := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "partiql-upd"}, now)
	if del.Code != http.StatusOK {
		// may fail if items remain
		_ = mustDynamoJSON(t, handler, "ExecuteStatement", map[string]any{
			"Statement": `DELETE FROM "partiql-upd" WHERE pk='a'`,
		}, now)
		del = mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "partiql-upd"}, now)
		if del.Code != http.StatusOK {
			t.Fatalf("DeleteTable status=%d body=%q", del.Code, del.Body.String())
		}
	}
}

func TestLambdaVpcConfigRejectedAndUpdateNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-vpc-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-vpc-role"

	vpcReject := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "vpc-fn",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
		"VpcConfig":    map[string]any{"SubnetIds": []string{"subnet-1"}},
	}, now)
	if vpcReject.Code == http.StatusOK || !strings.Contains(vpcReject.Body.String(), "VpcConfig") {
		t.Fatalf("VpcConfig reject status=%d body=%q", vpcReject.Code, vpcReject.Body.String())
	}

	create := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "cfg-fn",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
		"Timeout":      3,
		"DeadLetterConfig": map[string]any{
			"TargetArn": "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":dlq",
		},
		"Environment": map[string]any{"Variables": map[string]string{"K": "V"}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", create.Code, create.Body.String())
	}

	updVpc := mustLambdaJSON(t, handler, "UpdateFunctionConfiguration", map[string]any{
		"FunctionName": "cfg-fn",
		"VpcConfig":    map[string]any{"SubnetIds": []string{"subnet-1"}},
	}, now)
	if updVpc.Code == http.StatusOK {
		t.Fatalf("UpdateFunctionConfiguration VpcConfig should fail: %s", updVpc.Body.String())
	}

	updOK := mustLambdaJSON(t, handler, "UpdateFunctionConfiguration", map[string]any{
		"FunctionName": "cfg-fn",
		"Description":  "updated",
		"MemorySize":   256,
		"Timeout":      10,
		"Environment":  map[string]any{"Variables": map[string]string{"K": "2"}},
		"DeadLetterConfig": map[string]any{
			"TargetArn": "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":dlq2",
		},
		"DestinationConfig": map[string]any{
			"OnFailure": map[string]any{"Destination": "arn:aws:sns:" + testRegion + ":" + testAccountID + ":fail"},
			"OnSuccess": map[string]any{"Destination": "arn:aws:sns:" + testRegion + ":" + testAccountID + ":ok"},
		},
	}, now)
	if updOK.Code != http.StatusOK {
		t.Fatalf("UpdateFunctionConfiguration status=%d body=%q", updOK.Code, updOK.Body.String())
	}

	missing := mustLambdaJSON(t, handler, "UpdateFunctionConfiguration", map[string]any{
		"FunctionName": "no-such-fn",
		"Timeout":      5,
	}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("missing UpdateFunctionConfiguration should fail")
	}

	del := mustLambdaJSON(t, handler, "DeleteFunction", map[string]any{"FunctionName": "cfg-fn"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteFunction status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestSQSManagedSSESendReceive(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "sse-sqs-q",
		"Attributes": map[string]string{
			"SqsManagedSseEnabled": "true",
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := out["QueueUrl"].(string)

	send := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl":    queueURL,
		"MessageBody": "secret-payload",
	}, now)
	if send.Code != http.StatusOK {
		t.Fatalf("SendMessage status=%d body=%q", send.Code, send.Body.String())
	}
	recv := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            queueURL,
		"MaxNumberOfMessages": 1,
		"WaitTimeSeconds":     0,
	}, now)
	if recv.Code != http.StatusOK || !strings.Contains(recv.Body.String(), "secret-payload") {
		t.Fatalf("ReceiveMessage status=%d body=%q", recv.Code, recv.Body.String())
	}

	batch := mustSQSJSON(t, handler, "SendMessageBatch", map[string]any{
		"QueueUrl": queueURL,
		"Entries": []map[string]any{
			{"Id": "1", "MessageBody": "a"},
			{"Id": "2", "MessageBody": "b"},
		},
	}, now)
	if batch.Code != http.StatusOK {
		t.Fatalf("SendMessageBatch status=%d body=%q", batch.Code, batch.Body.String())
	}

	del := mustSQSJSON(t, handler, "DeleteQueue", map[string]any{"QueueUrl": queueURL}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteQueue status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestKMSNegativeValidationPaths(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	missingKey := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"Plaintext": base64.StdEncoding.EncodeToString([]byte("x")),
	}, now)
	if missingKey.Code == http.StatusOK {
		t.Fatalf("Encrypt without KeyId should fail")
	}
	badAlias := mustKMSJSON(t, handler, "CreateAlias", map[string]any{
		"AliasName": "alias/no-target",
	}, now)
	if badAlias.Code == http.StatusOK {
		t.Fatalf("CreateAlias without TargetKeyId should fail")
	}
	badDecrypt := mustKMSJSON(t, handler, "Decrypt", map[string]any{}, now)
	if badDecrypt.Code == http.StatusOK {
		t.Fatalf("Decrypt without blob should fail")
	}
	create := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", create.Code, create.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	meta, _ := created["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	badCtx := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":             keyID,
		"Plaintext":         base64.StdEncoding.EncodeToString([]byte("x")),
		"EncryptionContext": "not-a-map",
	}, now)
	if badCtx.Code == http.StatusOK {
		t.Fatalf("bad EncryptionContext should fail")
	}

	disable := mustKMSJSON(t, handler, "DisableKey", map[string]any{"KeyId": keyID}, now)
	if disable.Code != http.StatusOK {
		t.Fatalf("DisableKey status=%d body=%q", disable.Code, disable.Body.String())
	}
	enable := mustKMSJSON(t, handler, "EnableKey", map[string]any{"KeyId": keyID}, now)
	if enable.Code != http.StatusOK {
		t.Fatalf("EnableKey status=%d body=%q", enable.Code, enable.Body.String())
	}
	sched := mustKMSJSON(t, handler, "ScheduleKeyDeletion", map[string]any{
		"KeyId": keyID, "PendingWindowInDays": 7,
	}, now)
	if sched.Code != http.StatusOK {
		t.Fatalf("ScheduleKeyDeletion status=%d body=%q", sched.Code, sched.Body.String())
	}
	cancel := mustKMSJSON(t, handler, "CancelKeyDeletion", map[string]any{"KeyId": keyID}, now)
	if cancel.Code != http.StatusOK {
		t.Fatalf("CancelKeyDeletion status=%d body=%q", cancel.Code, cancel.Body.String())
	}
}

func TestCognitoPoolDescribeUpdateAndNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	pool := cognitoMustOK(t, handler, "CreateUserPool", map[string]any{"PoolName": "ops-pool"}, now)
	up, _ := pool["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)

	client := cognitoMustOK(t, handler, "CreateUserPoolClient", map[string]any{
		"UserPoolId": poolID, "ClientName": "ops-client",
	}, now)
	upc, _ := client["UserPoolClient"].(map[string]any)
	clientID, _ := upc["ClientId"].(string)

	cognitoMustOK(t, handler, "AdminCreateUser", map[string]any{
		"UserPoolId": poolID, "Username": "ops-user", "TemporaryPassword": "TempPass1!",
	}, now)
	cognitoMustOK(t, handler, "DescribeUserPool", map[string]any{"UserPoolId": poolID}, now)
	cognitoMustOK(t, handler, "DescribeUserPoolClient", map[string]any{
		"UserPoolId": poolID, "ClientId": clientID,
	}, now)
	cognitoMustOK(t, handler, "ListUserPools", map[string]any{"MaxResults": 10}, now)
	cognitoMustOK(t, handler, "ListUserPoolClients", map[string]any{"UserPoolId": poolID}, now)
	cognitoMustOK(t, handler, "UpdateUserPool", map[string]any{
		"UserPoolId": poolID, "AutoVerifiedAttributes": []string{"email"},
	}, now)

	missingPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.DescribeUserPool", "cognito-idp", map[string]any{
		"UserPoolId": "us-east-1_missing",
	}, now)
	if missingPool.Code == http.StatusOK {
		t.Fatalf("missing pool should fail")
	}
	missingClient := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.DescribeUserPoolClient", "cognito-idp", map[string]any{
		"UserPoolId": poolID, "ClientId": "missing-client",
	}, now)
	if missingClient.Code == http.StatusOK {
		t.Fatalf("missing client should fail")
	}

	cognitoMustOK(t, handler, "DeleteUserPoolClient", map[string]any{
		"UserPoolId": poolID, "ClientId": clientID,
	}, now)
	cognitoMustOK(t, handler, "DeleteUserPool", map[string]any{"UserPoolId": poolID}, now)
}

func TestSNSPermissionsAndSubscriptionAttrs(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	topicARN := snsCreateTopic(t, handler, "perm-topic-ops", now)

	add := mustSNSQuery(t, handler, strings.Join([]string{
		"Action=AddPermission",
		"Version=2010-03-31",
		"TopicArn=" + url.QueryEscape(topicARN),
		"Label=allow-pub",
		"AWSAccountId=" + testAccountID,
		"ActionName=Publish",
	}, "&"), testAccessKey, testSecret, now)
	if add.Code != http.StatusOK {
		t.Fatalf("AddPermission status=%d body=%q", add.Code, add.Body.String())
	}
	rm := mustSNSQuery(t, handler,
		"Action=RemovePermission&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+"&Label=allow-pub",
		testAccessKey, testSecret, now)
	if rm.Code != http.StatusOK {
		t.Fatalf("RemovePermission status=%d body=%q", rm.Code, rm.Body.String())
	}

	qARN := "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":sns-sub-ops-q"
	_ = mustSQSJSON(t, handler, "CreateQueue", map[string]any{"QueueName": "sns-sub-ops-q"}, now)
	sub := mustSNSQuery(t, handler,
		"Action=Subscribe&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Protocol=sqs&Endpoint="+url.QueryEscape(qARN),
		testAccessKey, testSecret, now)
	if sub.Code != http.StatusOK {
		t.Fatalf("Subscribe status=%d body=%q", sub.Code, sub.Body.String())
	}
	subARN := xmlTag(t, sub.Body.String(), "SubscriptionArn")
	if subARN != "" && subARN != "pending confirmation" {
		attrs := mustSNSQuery(t, handler,
			"Action=GetSubscriptionAttributes&Version=2010-03-31&SubscriptionArn="+url.QueryEscape(subARN),
			testAccessKey, testSecret, now)
		if attrs.Code != http.StatusOK {
			t.Fatalf("GetSubscriptionAttributes status=%d body=%q", attrs.Code, attrs.Body.String())
		}
	}

	set := mustSNSQuery(t, handler,
		"Action=SetTopicAttributes&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&AttributeName=DisplayName&AttributeValue=lab",
		testAccessKey, testSecret, now)
	if set.Code != http.StatusOK {
		t.Fatalf("SetTopicAttributes status=%d body=%q", set.Code, set.Body.String())
	}
	get := mustSNSQuery(t, handler,
		"Action=GetTopicAttributes&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN),
		testAccessKey, testSecret, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetTopicAttributes status=%d body=%q", get.Code, get.Body.String())
	}

	del := mustSNSQuery(t, handler,
		"Action=DeleteTopic&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN),
		testAccessKey, testSecret, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteTopic status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestSSMHistoryLabelsAndPathNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	put := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name": "/lab/hist", "Value": "v1", "Type": "String",
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutParameter status=%d body=%q", put.Code, put.Body.String())
	}
	put2 := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name": "/lab/hist", "Value": "v2", "Type": "String", "Overwrite": true,
	}, now)
	if put2.Code != http.StatusOK {
		t.Fatalf("PutParameter overwrite status=%d body=%q", put2.Code, put2.Body.String())
	}
	hist := mustSSMJSON(t, handler, "GetParameterHistory", map[string]any{
		"Name": "/lab/hist",
	}, now)
	if hist.Code != http.StatusOK {
		t.Fatalf("GetParameterHistory status=%d body=%q", hist.Code, hist.Body.String())
	}
	label := mustSSMJSON(t, handler, "LabelParameterVersion", map[string]any{
		"Name": "/lab/hist", "ParameterVersion": 2, "Labels": []string{"prod"},
	}, now)
	if label.Code != http.StatusOK {
		t.Fatalf("LabelParameterVersion status=%d body=%q", label.Code, label.Body.String())
	}
	byPath := mustSSMJSON(t, handler, "GetParametersByPath", map[string]any{
		"Path": "/lab", "Recursive": true,
	}, now)
	if byPath.Code != http.StatusOK {
		t.Fatalf("GetParametersByPath status=%d body=%q", byPath.Code, byPath.Body.String())
	}
	missingHist := mustSSMJSON(t, handler, "GetParameterHistory", map[string]any{
		"Name": "/lab/nope",
	}, now)
	if missingHist.Code == http.StatusOK {
		t.Fatalf("missing history should fail")
	}
	del := mustSSMJSON(t, handler, "DeleteParameter", map[string]any{
		"Name": "/lab/hist",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteParameter status=%d body=%q", del.Code, del.Body.String())
	}
}
