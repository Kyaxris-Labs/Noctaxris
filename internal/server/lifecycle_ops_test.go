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

func TestS3BucketLoggingLifecycle(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	src := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/log-src", nil, "s3", now, nil)
	if src.Code < 200 || src.Code >= 300 {
		t.Fatalf("create src bucket %d %s", src.Code, src.Body.String())
	}
	tgt := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/log-tgt", nil, "s3", now, nil)
	if tgt.Code < 200 || tgt.Code >= 300 {
		t.Fatalf("create tgt bucket %d %s", tgt.Code, tgt.Body.String())
	}

	badXML := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/log-src?logging",
		[]byte(`<not-xml`), "s3", now, nil)
	if badXML.Code == http.StatusOK {
		t.Fatalf("malformed logging XML should fail: %d %s", badXML.Code, badXML.Body.String())
	}

	put := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/log-src?logging",
		[]byte(`<BucketLoggingStatus xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><LoggingEnabled><TargetBucket>log-tgt</TargetBucket><TargetPrefix>access/</TargetPrefix></LoggingEnabled></BucketLoggingStatus>`),
		"s3", now, nil)
	if put.Code != http.StatusOK {
		t.Fatalf("PutBucketLogging %d %s", put.Code, put.Body.String())
	}
	get := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/log-src?logging", nil, "s3", now, nil)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "log-tgt") {
		t.Fatalf("GetBucketLogging %d %s", get.Code, get.Body.String())
	}
	disable := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/log-src?logging",
		[]byte(`<BucketLoggingStatus xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></BucketLoggingStatus>`),
		"s3", now, nil)
	if disable.Code != http.StatusOK {
		t.Fatalf("disable logging %d %s", disable.Code, disable.Body.String())
	}
	getOff := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/log-src?logging", nil, "s3", now, nil)
	if getOff.Code != http.StatusOK || strings.Contains(getOff.Body.String(), "LoggingEnabled") {
		t.Fatalf("GetBucketLogging disabled %d %s", getOff.Code, getOff.Body.String())
	}
}

func TestDynamoDBScanTTLAndTagsLifecycle(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "scan-ttl",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "sk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
			{"AttributeName": "sk", "KeyType": "RANGE"},
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable %d %s", create.Code, create.Body.String())
	}

	for _, item := range []map[string]any{
		{"pk": map[string]any{"S": "a"}, "sk": map[string]any{"S": "1"}, "n": map[string]any{"N": "1"}},
		{"pk": map[string]any{"S": "a"}, "sk": map[string]any{"S": "2"}, "n": map[string]any{"N": "2"}},
		{"pk": map[string]any{"S": "b"}, "sk": map[string]any{"S": "1"}, "n": map[string]any{"N": "3"}},
	} {
		put := mustDynamoJSON(t, handler, "PutItem", map[string]any{
			"TableName": "scan-ttl", "Item": item,
		}, now)
		if put.Code != http.StatusOK {
			t.Fatalf("PutItem %d %s", put.Code, put.Body.String())
		}
	}

	scan := mustDynamoJSON(t, handler, "Scan", map[string]any{
		"TableName":        "scan-ttl",
		"FilterExpression": "#n = :one",
		"ExpressionAttributeNames": map[string]any{
			"#n": "n",
		},
		"ExpressionAttributeValues": map[string]any{
			":one": map[string]any{"N": "1"},
		},
		"ProjectionExpression": "pk, sk",
		"Limit":                10,
	}, now)
	if scan.Code != http.StatusOK {
		t.Fatalf("Scan %d %s", scan.Code, scan.Body.String())
	}
	scanMiss := mustDynamoJSON(t, handler, "Scan", map[string]any{"TableName": "no-table"}, now)
	if scanMiss.Code == http.StatusOK {
		t.Fatalf("Scan missing should fail")
	}
	scanBadFilter := mustDynamoJSON(t, handler, "Scan", map[string]any{
		"TableName":        "scan-ttl",
		"FilterExpression": "<<<",
	}, now)
	if scanBadFilter.Code == http.StatusOK {
		t.Fatalf("Scan bad filter should fail")
	}

	ttl := mustDynamoJSON(t, handler, "UpdateTimeToLive", map[string]any{
		"TableName": "scan-ttl",
		"TimeToLiveSpecification": map[string]any{
			"AttributeName": "ttl",
			"Enabled":       true,
		},
	}, now)
	if ttl.Code != http.StatusOK {
		t.Fatalf("UpdateTimeToLive %d %s", ttl.Code, ttl.Body.String())
	}
	descTTL := mustDynamoJSON(t, handler, "DescribeTimeToLive", map[string]any{"TableName": "scan-ttl"}, now)
	if descTTL.Code != http.StatusOK || !strings.Contains(descTTL.Body.String(), "ttl") {
		t.Fatalf("DescribeTimeToLive %d %s", descTTL.Code, descTTL.Body.String())
	}
	ttlOff := mustDynamoJSON(t, handler, "UpdateTimeToLive", map[string]any{
		"TableName": "scan-ttl",
		"TimeToLiveSpecification": map[string]any{
			"AttributeName": "ttl",
			"Enabled":       false,
		},
	}, now)
	if ttlOff.Code != http.StatusOK {
		t.Fatalf("UpdateTimeToLive off %d %s", ttlOff.Code, ttlOff.Body.String())
	}
	ttlMiss := mustDynamoJSON(t, handler, "UpdateTimeToLive", map[string]any{
		"TableName": "missing",
		"TimeToLiveSpecification": map[string]any{
			"AttributeName": "ttl", "Enabled": true,
		},
	}, now)
	if ttlMiss.Code == http.StatusOK {
		t.Fatalf("UpdateTimeToLive missing should fail")
	}

	arn := "arn:aws:dynamodb:" + testRegion + ":" + testAccountID + ":table/scan-ttl"
	tag := mustDynamoJSON(t, handler, "TagResource", map[string]any{
		"ResourceArn": arn,
		"Tags":        []map[string]any{{"Key": "env", "Value": "lab"}},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagResource %d %s", tag.Code, tag.Body.String())
	}
	listTags := mustDynamoJSON(t, handler, "ListTagsOfResource", map[string]any{"ResourceArn": arn}, now)
	if listTags.Code != http.StatusOK || !strings.Contains(listTags.Body.String(), "env") {
		t.Fatalf("ListTagsOfResource %d %s", listTags.Code, listTags.Body.String())
	}
	untag := mustDynamoJSON(t, handler, "UntagResource", map[string]any{
		"ResourceArn": arn, "TagKeys": []string{"env"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource %d %s", untag.Code, untag.Body.String())
	}
	tagBad := mustDynamoJSON(t, handler, "TagResource", map[string]any{
		"ResourceArn": "arn:aws:dynamodb:" + testRegion + ":" + testAccountID + ":table/nope",
		"Tags":        []map[string]any{{"Key": "a", "Value": "b"}},
	}, now)
	if tagBad.Code == http.StatusOK {
		t.Fatalf("TagResource missing should fail")
	}

	tg := mustDynamoJSON(t, handler, "TransactGetItems", map[string]any{
		"TransactItems": []map[string]any{{
			"Get": map[string]any{
				"TableName": "scan-ttl",
				"Key": map[string]any{
					"pk": map[string]any{"S": "a"},
					"sk": map[string]any{"S": "1"},
				},
			},
		}},
	}, now)
	if tg.Code != http.StatusOK {
		t.Fatalf("TransactGetItems %d %s", tg.Code, tg.Body.String())
	}
	tgEmpty := mustDynamoJSON(t, handler, "TransactGetItems", map[string]any{}, now)
	if tgEmpty.Code == http.StatusOK {
		t.Fatalf("TransactGetItems empty should fail")
	}
	tgBad := mustDynamoJSON(t, handler, "TransactGetItems", map[string]any{
		"TransactItems": []map[string]any{{"Put": map[string]any{}}},
	}, now)
	if tgBad.Code == http.StatusOK {
		t.Fatalf("TransactGetItems without Get should fail")
	}
}

func TestLambdaListFunctionUrlConfigs(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-list-url-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-list-url-role"
	create := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "list-url-fn",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateFunction %d %s", create.Code, create.Body.String())
	}
	urlCfg := mustLambdaJSON(t, handler, "CreateFunctionUrlConfig", map[string]any{
		"FunctionName": "list-url-fn",
		"AuthType":     "AWS_IAM",
		"Cors":         map[string]any{"AllowOrigins": []string{"https://lab.example"}},
	}, now)
	if urlCfg.Code != http.StatusOK {
		t.Fatalf("CreateFunctionUrlConfig %d %s", urlCfg.Code, urlCfg.Body.String())
	}
	list := mustLambdaJSON(t, handler, "ListFunctionUrlConfigs", map[string]any{
		"FunctionName": "list-url-fn",
	}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "list-url-fn") {
		t.Fatalf("ListFunctionUrlConfigs %d %s", list.Code, list.Body.String())
	}
	listAll := mustLambdaJSON(t, handler, "ListFunctionUrlConfigs", map[string]any{}, now)
	if listAll.Code != http.StatusOK {
		t.Fatalf("ListFunctionUrlConfigs all %d %s", listAll.Code, listAll.Body.String())
	}
	dup := mustLambdaJSON(t, handler, "CreateFunctionUrlConfig", map[string]any{
		"FunctionName": "list-url-fn", "AuthType": "AWS_IAM",
	}, now)
	if dup.Code == http.StatusOK {
		t.Fatalf("CreateFunctionUrlConfig dup should fail")
	}
	miss := mustLambdaJSON(t, handler, "CreateFunctionUrlConfig", map[string]any{
		"FunctionName": "no-fn", "AuthType": "AWS_IAM",
	}, now)
	if miss.Code == http.StatusOK {
		t.Fatalf("CreateFunctionUrlConfig missing should fail")
	}
	empty := mustLambdaJSON(t, handler, "CreateFunctionUrlConfig", map[string]any{
		"FunctionName": "", "AuthType": "AWS_IAM",
	}, now)
	if empty.Code == http.StatusOK {
		t.Fatalf("CreateFunctionUrlConfig empty name should fail")
	}
}

func TestEventBridgeBusPermissionAndRuleLifecycle(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createBus := mustEventsJSON(t, handler, "CreateEventBus", map[string]any{"Name": "ops-bus"}, now)
	if createBus.Code != http.StatusOK {
		t.Fatalf("CreateEventBus %d %s", createBus.Code, createBus.Body.String())
	}
	dupBus := mustEventsJSON(t, handler, "CreateEventBus", map[string]any{"Name": "ops-bus"}, now)
	if dupBus.Code == http.StatusOK {
		t.Fatalf("CreateEventBus dup should fail")
	}
	desc := mustEventsJSON(t, handler, "DescribeEventBus", map[string]any{"Name": "ops-bus"}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeEventBus %d %s", desc.Code, desc.Body.String())
	}
	listBuses := mustEventsJSON(t, handler, "ListEventBuses", map[string]any{}, now)
	if listBuses.Code != http.StatusOK || !strings.Contains(listBuses.Body.String(), "ops-bus") {
		t.Fatalf("ListEventBuses %d %s", listBuses.Code, listBuses.Body.String())
	}

	put := mustEventsJSON(t, handler, "PutPermission", map[string]any{
		"EventBusName": "ops-bus",
		"Action":       "events:PutEvents",
		"Principal":    "*",
		"StatementId":  "allow-all",
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutPermission %d %s", put.Code, put.Body.String())
	}
	rm := mustEventsJSON(t, handler, "RemovePermission", map[string]any{
		"EventBusName": "ops-bus", "StatementId": "allow-all",
	}, now)
	if rm.Code != http.StatusOK {
		t.Fatalf("RemovePermission %d %s", rm.Code, rm.Body.String())
	}
	rmMiss := mustEventsJSON(t, handler, "RemovePermission", map[string]any{
		"EventBusName": "ops-bus", "StatementId": "missing",
	}, now)
	if rmMiss.Code == http.StatusOK {
		t.Fatalf("RemovePermission missing should fail")
	}

	rule := mustEventsJSON(t, handler, "PutRule", map[string]any{
		"Name":         "ops-rule",
		"EventBusName": "ops-bus",
		"EventPattern": `{"source":["noctaxris.ops"]}`,
		"State":        "ENABLED",
	}, now)
	if rule.Code != http.StatusOK {
		t.Fatalf("PutRule %d %s", rule.Code, rule.Body.String())
	}
	listRules := mustEventsJSON(t, handler, "ListRules", map[string]any{"EventBusName": "ops-bus"}, now)
	if listRules.Code != http.StatusOK || !strings.Contains(listRules.Body.String(), "ops-rule") {
		t.Fatalf("ListRules %d %s", listRules.Code, listRules.Body.String())
	}
	descRule := mustEventsJSON(t, handler, "DescribeRule", map[string]any{
		"Name": "ops-rule", "EventBusName": "ops-bus",
	}, now)
	if descRule.Code != http.StatusOK {
		t.Fatalf("DescribeRule %d %s", descRule.Code, descRule.Body.String())
	}
	disable := mustEventsJSON(t, handler, "DisableRule", map[string]any{
		"Name": "ops-rule", "EventBusName": "ops-bus",
	}, now)
	if disable.Code != http.StatusOK {
		t.Fatalf("DisableRule %d %s", disable.Code, disable.Body.String())
	}
	enable := mustEventsJSON(t, handler, "EnableRule", map[string]any{
		"Name": "ops-rule", "EventBusName": "ops-bus",
	}, now)
	if enable.Code != http.StatusOK {
		t.Fatalf("EnableRule %d %s", enable.Code, enable.Body.String())
	}

	q := mustSQSJSON(t, handler, "CreateQueue", map[string]any{"QueueName": "ops-bus-q"}, now)
	if q.Code != http.StatusOK {
		t.Fatalf("CreateQueue %d %s", q.Code, q.Body.String())
	}
	qARN := "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":ops-bus-q"
	targets := mustEventsJSON(t, handler, "PutTargets", map[string]any{
		"Rule": "ops-rule", "EventBusName": "ops-bus",
		"Targets": []map[string]any{{"Id": "1", "Arn": qARN}},
	}, now)
	if targets.Code != http.StatusOK {
		t.Fatalf("PutTargets %d %s", targets.Code, targets.Body.String())
	}
	listT := mustEventsJSON(t, handler, "ListTargetsByRule", map[string]any{
		"Rule": "ops-rule", "EventBusName": "ops-bus",
	}, now)
	if listT.Code != http.StatusOK || !strings.Contains(listT.Body.String(), qARN) {
		t.Fatalf("ListTargetsByRule %d %s", listT.Code, listT.Body.String())
	}
	rmT := mustEventsJSON(t, handler, "RemoveTargets", map[string]any{
		"Rule": "ops-rule", "EventBusName": "ops-bus", "Ids": []string{"1"},
	}, now)
	if rmT.Code != http.StatusOK {
		t.Fatalf("RemoveTargets %d %s", rmT.Code, rmT.Body.String())
	}
	delRule := mustEventsJSON(t, handler, "DeleteRule", map[string]any{
		"Name": "ops-rule", "EventBusName": "ops-bus",
	}, now)
	if delRule.Code != http.StatusOK {
		t.Fatalf("DeleteRule %d %s", delRule.Code, delRule.Body.String())
	}

	busARN := "arn:aws:events:" + testRegion + ":" + testAccountID + ":event-bus/ops-bus"
	tag := mustEventsJSON(t, handler, "TagResource", map[string]any{
		"ResourceARN": busARN,
		"Tags":        []map[string]any{{"Key": "team", "Value": "lab"}},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagResource %d %s", tag.Code, tag.Body.String())
	}
	listTags := mustEventsJSON(t, handler, "ListTagsForResource", map[string]any{"ResourceARN": busARN}, now)
	if listTags.Code != http.StatusOK {
		t.Fatalf("ListTagsForResource %d %s", listTags.Code, listTags.Body.String())
	}
	untag := mustEventsJSON(t, handler, "UntagResource", map[string]any{
		"ResourceARN": busARN, "TagKeys": []string{"team"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource %d %s", untag.Code, untag.Body.String())
	}

	delBus := mustEventsJSON(t, handler, "DeleteEventBus", map[string]any{"Name": "ops-bus"}, now)
	if delBus.Code != http.StatusOK {
		t.Fatalf("DeleteEventBus %d %s", delBus.Code, delBus.Body.String())
	}
	delMiss := mustEventsJSON(t, handler, "DeleteEventBus", map[string]any{"Name": "ops-bus"}, now)
	if delMiss.Code == http.StatusOK {
		t.Fatalf("DeleteEventBus missing should fail")
	}
}

func TestKMSGrantTagReEncryptAndPolicy(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustKMSJSON(t, handler, "CreateKey", map[string]any{"Description": "grant-key"}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateKey %d %s", create.Code, create.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	meta, _ := created["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)
	keyARN, _ := meta["Arn"].(string)

	create2 := mustKMSJSON(t, handler, "CreateKey", map[string]any{"Description": "dest-key"}, now)
	if create2.Code != http.StatusOK {
		t.Fatalf("CreateKey2 %d %s", create2.Code, create2.Body.String())
	}
	var created2 map[string]any
	_ = json.Unmarshal(create2.Body.Bytes(), &created2)
	meta2, _ := created2["KeyMetadata"].(map[string]any)
	destID, _ := meta2["KeyId"].(string)

	enc := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId": keyID, "Plaintext": base64.StdEncoding.EncodeToString([]byte("reenc")),
	}, now)
	if enc.Code != http.StatusOK {
		t.Fatalf("Encrypt %d %s", enc.Code, enc.Body.String())
	}
	var encOut map[string]any
	_ = json.Unmarshal(enc.Body.Bytes(), &encOut)
	blob, _ := encOut["CiphertextBlob"].(string)

	reenc := mustKMSJSON(t, handler, "ReEncrypt", map[string]any{
		"CiphertextBlob":   blob,
		"DestinationKeyId": destID,
	}, now)
	if reenc.Code != http.StatusOK {
		t.Fatalf("ReEncrypt %d %s", reenc.Code, reenc.Body.String())
	}

	grant := mustKMSJSON(t, handler, "CreateGrant", map[string]any{
		"KeyId":             keyID,
		"GranteePrincipal":  "arn:aws:iam::" + testAccountID + ":root",
		"Operations":        []string{"Encrypt", "Decrypt"},
		"Name":              "lab-grant",
	}, now)
	if grant.Code != http.StatusOK {
		t.Fatalf("CreateGrant %d %s", grant.Code, grant.Body.String())
	}
	var grantOut map[string]any
	_ = json.Unmarshal(grant.Body.Bytes(), &grantOut)
	grantID, _ := grantOut["GrantId"].(string)
	listG := mustKMSJSON(t, handler, "ListGrants", map[string]any{"KeyId": keyID}, now)
	if listG.Code != http.StatusOK || !strings.Contains(listG.Body.String(), grantID) {
		t.Fatalf("ListGrants %d %s", listG.Code, listG.Body.String())
	}
	retire := mustKMSJSON(t, handler, "RetireGrant", map[string]any{"GrantId": grantID}, now)
	if retire.Code != http.StatusOK {
		t.Fatalf("RetireGrant %d %s", retire.Code, retire.Body.String())
	}

	tag := mustKMSJSON(t, handler, "TagResource", map[string]any{
		"KeyId": keyID,
		"Tags":  []map[string]any{{"TagKey": "owner", "TagValue": "lab"}},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagResource %d %s", tag.Code, tag.Body.String())
	}
	listTags := mustKMSJSON(t, handler, "ListResourceTags", map[string]any{"KeyId": keyID}, now)
	if listTags.Code != http.StatusOK {
		t.Fatalf("ListResourceTags %d %s", listTags.Code, listTags.Body.String())
	}
	untag := mustKMSJSON(t, handler, "UntagResource", map[string]any{
		"KeyId": keyID, "TagKeys": []string{"owner"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource %d %s", untag.Code, untag.Body.String())
	}

	pol := mustKMSJSON(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"kms:*","Resource":"*"}]}`,
	}, now)
	if pol.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy %d %s", pol.Code, pol.Body.String())
	}
	getPol := mustKMSJSON(t, handler, "GetKeyPolicy", map[string]any{
		"KeyId": keyID, "PolicyName": "default",
	}, now)
	if getPol.Code != http.StatusOK {
		t.Fatalf("GetKeyPolicy %d %s", getPol.Code, getPol.Body.String())
	}

	alias := mustKMSJSON(t, handler, "CreateAlias", map[string]any{
		"AliasName": "alias/grant-key", "TargetKeyId": keyID,
	}, now)
	if alias.Code != http.StatusOK {
		t.Fatalf("CreateAlias %d %s", alias.Code, alias.Body.String())
	}
	updAlias := mustKMSJSON(t, handler, "UpdateAlias", map[string]any{
		"AliasName": "alias/grant-key", "TargetKeyId": destID,
	}, now)
	if updAlias.Code != http.StatusOK {
		t.Fatalf("UpdateAlias %d %s", updAlias.Code, updAlias.Body.String())
	}
	delAlias := mustKMSJSON(t, handler, "DeleteAlias", map[string]any{"AliasName": "alias/grant-key"}, now)
	if delAlias.Code != http.StatusOK {
		t.Fatalf("DeleteAlias %d %s", delAlias.Code, delAlias.Body.String())
	}

	rot := mustKMSJSON(t, handler, "EnableKeyRotation", map[string]any{"KeyId": keyID}, now)
	if rot.Code != http.StatusOK {
		t.Fatalf("EnableKeyRotation %d %s", rot.Code, rot.Body.String())
	}
	rotStat := mustKMSJSON(t, handler, "GetKeyRotationStatus", map[string]any{"KeyId": keyID}, now)
	if rotStat.Code != http.StatusOK {
		t.Fatalf("GetKeyRotationStatus %d %s", rotStat.Code, rotStat.Body.String())
	}
	disRot := mustKMSJSON(t, handler, "DisableKeyRotation", map[string]any{"KeyId": keyID}, now)
	if disRot.Code != http.StatusOK {
		t.Fatalf("DisableKeyRotation %d %s", disRot.Code, disRot.Body.String())
	}

	_ = keyARN
	listKeys := mustKMSJSON(t, handler, "ListKeys", map[string]any{}, now)
	if listKeys.Code != http.StatusOK {
		t.Fatalf("ListKeys %d %s", listKeys.Code, listKeys.Body.String())
	}
	listAliases := mustKMSJSON(t, handler, "ListAliases", map[string]any{}, now)
	if listAliases.Code != http.StatusOK {
		t.Fatalf("ListAliases %d %s", listAliases.Code, listAliases.Body.String())
	}
}

func TestIAMInstanceProfileBoundaryAndOIDCLifecycle(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_ = iamForm(t, handler, "Action=CreateUser&Version=2010-05-08&UserName=bound-user", now)
	trust := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	role := iamForm(t, handler, "Action=CreateRole&Version=2010-05-08&RoleName=bound-role&AssumeRolePolicyDocument="+trust, now)
	if role.Code != http.StatusOK {
		t.Fatalf("CreateRole %d %s", role.Code, role.Body.String())
	}
	polDoc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`)
	pol := iamForm(t, handler, "Action=CreatePolicy&Version=2010-05-08&PolicyName=bound-pol&PolicyDocument="+polDoc, now)
	if pol.Code != http.StatusOK {
		t.Fatalf("CreatePolicy %d %s", pol.Code, pol.Body.String())
	}
	policyARN := xmlTag(t, pol.Body.String(), "Arn")

	putUB := iamForm(t, handler, "Action=PutUserPermissionsBoundary&Version=2010-05-08&UserName=bound-user&PermissionsBoundary="+url.QueryEscape(policyARN), now)
	if putUB.Code != http.StatusOK {
		t.Fatalf("PutUserPermissionsBoundary %d %s", putUB.Code, putUB.Body.String())
	}
	getUB := iamForm(t, handler, "Action=GetUserPermissionsBoundary&Version=2010-05-08&UserName=bound-user", now)
	if getUB.Code != http.StatusOK {
		t.Fatalf("GetUserPermissionsBoundary %d %s", getUB.Code, getUB.Body.String())
	}
	delUB := iamForm(t, handler, "Action=DeleteUserPermissionsBoundary&Version=2010-05-08&UserName=bound-user", now)
	if delUB.Code != http.StatusOK {
		t.Fatalf("DeleteUserPermissionsBoundary %d %s", delUB.Code, delUB.Body.String())
	}

	putRB := iamForm(t, handler, "Action=PutRolePermissionsBoundary&Version=2010-05-08&RoleName=bound-role&PermissionsBoundary="+url.QueryEscape(policyARN), now)
	if putRB.Code != http.StatusOK {
		t.Fatalf("PutRolePermissionsBoundary %d %s", putRB.Code, putRB.Body.String())
	}
	getRB := iamForm(t, handler, "Action=GetRolePermissionsBoundary&Version=2010-05-08&RoleName=bound-role", now)
	if getRB.Code != http.StatusOK {
		t.Fatalf("GetRolePermissionsBoundary %d %s", getRB.Code, getRB.Body.String())
	}
	delRB := iamForm(t, handler, "Action=DeleteRolePermissionsBoundary&Version=2010-05-08&RoleName=bound-role", now)
	if delRB.Code != http.StatusOK {
		t.Fatalf("DeleteRolePermissionsBoundary %d %s", delRB.Code, delRB.Body.String())
	}

	ip := iamForm(t, handler, "Action=CreateInstanceProfile&Version=2010-05-08&InstanceProfileName=bound-ip", now)
	if ip.Code != http.StatusOK {
		t.Fatalf("CreateInstanceProfile %d %s", ip.Code, ip.Body.String())
	}
	add := iamForm(t, handler, "Action=AddRoleToInstanceProfile&Version=2010-05-08&InstanceProfileName=bound-ip&RoleName=bound-role", now)
	if add.Code != http.StatusOK {
		t.Fatalf("AddRoleToInstanceProfile %d %s", add.Code, add.Body.String())
	}
	getIP := iamForm(t, handler, "Action=GetInstanceProfile&Version=2010-05-08&InstanceProfileName=bound-ip", now)
	if getIP.Code != http.StatusOK || !strings.Contains(getIP.Body.String(), "bound-role") {
		t.Fatalf("GetInstanceProfile %d %s", getIP.Code, getIP.Body.String())
	}
	listIPRole := iamForm(t, handler, "Action=ListInstanceProfilesForRole&Version=2010-05-08&RoleName=bound-role", now)
	if listIPRole.Code != http.StatusOK || !strings.Contains(listIPRole.Body.String(), "bound-ip") {
		t.Fatalf("ListInstanceProfilesForRole %d %s", listIPRole.Code, listIPRole.Body.String())
	}
	rem := iamForm(t, handler, "Action=RemoveRoleFromInstanceProfile&Version=2010-05-08&InstanceProfileName=bound-ip&RoleName=bound-role", now)
	if rem.Code != http.StatusOK {
		t.Fatalf("RemoveRoleFromInstanceProfile %d %s", rem.Code, rem.Body.String())
	}
	delIP := iamForm(t, handler, "Action=DeleteInstanceProfile&Version=2010-05-08&InstanceProfileName=bound-ip", now)
	if delIP.Code != http.StatusOK {
		t.Fatalf("DeleteInstanceProfile %d %s", delIP.Code, delIP.Body.String())
	}

	oidc := iamForm(t, handler, "Action=CreateOpenIDConnectProvider&Version=2010-05-08&Url=https://oidc.ops.example&ClientIDList.member.1=aud1&ThumbprintList.member.1=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now)
	if oidc.Code != http.StatusOK {
		t.Fatalf("CreateOpenIDConnectProvider %d %s", oidc.Code, oidc.Body.String())
	}
	oidcARN := xmlTag(t, oidc.Body.String(), "OpenIDConnectProviderArn")
	getOIDC := iamForm(t, handler, "Action=GetOpenIDConnectProvider&Version=2010-05-08&OpenIDConnectProviderArn="+url.QueryEscape(oidcARN), now)
	if getOIDC.Code != http.StatusOK {
		t.Fatalf("GetOpenIDConnectProvider %d %s", getOIDC.Code, getOIDC.Body.String())
	}
	delOIDC := iamForm(t, handler, "Action=DeleteOpenIDConnectProvider&Version=2010-05-08&OpenIDConnectProviderArn="+url.QueryEscape(oidcARN), now)
	if delOIDC.Code != http.StatusOK {
		t.Fatalf("DeleteOpenIDConnectProvider %d %s", delOIDC.Code, delOIDC.Body.String())
	}
}
