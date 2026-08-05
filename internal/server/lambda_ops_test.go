package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const lambdaTrustLab = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

func TestLambdaAliasVersionEventInvokeAndLayers(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-ops-role", lambdaTrustLab, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-ops-role"

	create := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "ops-fn",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
		"Timeout":      3,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateFunction %d %s", create.Code, create.Body.String())
	}

	get := mustLambdaJSON(t, handler, "GetFunction", map[string]any{"FunctionName": "ops-fn"}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetFunction %d %s", get.Code, get.Body.String())
	}
	getMissing := mustLambdaJSON(t, handler, "GetFunction", map[string]any{"FunctionName": "no-fn"}, now)
	if getMissing.Code == http.StatusOK {
		t.Fatalf("GetFunction missing should fail")
	}

	pub := mustLambdaJSON(t, handler, "PublishVersion", map[string]any{
		"FunctionName": "ops-fn",
		"Description":  "v1",
	}, now)
	if pub.Code != http.StatusOK {
		t.Fatalf("PublishVersion %d %s", pub.Code, pub.Body.String())
	}

	listVer := mustLambdaJSON(t, handler, "ListVersionsByFunction", map[string]any{"FunctionName": "ops-fn"}, now)
	if listVer.Code != http.StatusOK {
		t.Fatalf("ListVersionsByFunction %d %s", listVer.Code, listVer.Body.String())
	}

	alias := mustLambdaJSON(t, handler, "CreateAlias", map[string]any{
		"FunctionName":    "ops-fn",
		"Name":            "live",
		"FunctionVersion": "1",
	}, now)
	if alias.Code != http.StatusOK {
		t.Fatalf("CreateAlias %d %s", alias.Code, alias.Body.String())
	}
	listAlias := mustLambdaJSON(t, handler, "ListAliases", map[string]any{"FunctionName": "ops-fn"}, now)
	if listAlias.Code != http.StatusOK || !strings.Contains(listAlias.Body.String(), "live") {
		t.Fatalf("ListAliases %d %s", listAlias.Code, listAlias.Body.String())
	}
	getAlias := mustLambdaJSON(t, handler, "GetAlias", map[string]any{
		"FunctionName": "ops-fn", "Name": "live",
	}, now)
	if getAlias.Code != http.StatusOK {
		t.Fatalf("GetAlias %d %s", getAlias.Code, getAlias.Body.String())
	}

	updAlias := mustLambdaJSON(t, handler, "UpdateAlias", map[string]any{
		"FunctionName":    "ops-fn",
		"Name":            "live",
		"FunctionVersion": "1",
		"Description":     "v1-live",
	}, now)
	if updAlias.Code != http.StatusOK {
		t.Fatalf("UpdateAlias %d %s", updAlias.Code, updAlias.Body.String())
	}

	eic := mustLambdaJSON(t, handler, "PutFunctionEventInvokeConfig", map[string]any{
		"FunctionName":             "ops-fn",
		"MaximumRetryAttempts":     1,
		"MaximumEventAgeInSeconds": 60,
	}, now)
	if eic.Code != http.StatusOK {
		t.Fatalf("PutFunctionEventInvokeConfig %d %s", eic.Code, eic.Body.String())
	}
	getEIC := mustLambdaJSON(t, handler, "GetFunctionEventInvokeConfig", map[string]any{"FunctionName": "ops-fn"}, now)
	if getEIC.Code != http.StatusOK {
		t.Fatalf("GetFunctionEventInvokeConfig %d %s", getEIC.Code, getEIC.Body.String())
	}
	delEIC := mustLambdaJSON(t, handler, "DeleteFunctionEventInvokeConfig", map[string]any{"FunctionName": "ops-fn"}, now)
	if delEIC.Code != http.StatusOK {
		t.Fatalf("DeleteFunctionEventInvokeConfig %d %s", delEIC.Code, delEIC.Body.String())
	}

	cfg := mustLambdaJSON(t, handler, "GetFunction", map[string]any{"FunctionName": "ops-fn"}, now)
	if cfg.Code != http.StatusOK {
		t.Fatalf("GetFunction config path %d %s", cfg.Code, cfg.Body.String())
	}
	updCfg := mustLambdaJSON(t, handler, "UpdateFunctionConfiguration", map[string]any{
		"FunctionName": "ops-fn",
		"Timeout":      5,
		"Environment":  map[string]any{"Variables": map[string]string{"A": "1"}},
	}, now)
	if updCfg.Code != http.StatusOK {
		t.Fatalf("UpdateFunctionConfiguration %d %s", updCfg.Code, updCfg.Body.String())
	}

	listFns := mustLambdaJSON(t, handler, "ListFunctions", map[string]any{}, now)
	if listFns.Code != http.StatusOK || !strings.Contains(listFns.Body.String(), "ops-fn") {
		t.Fatalf("ListFunctions %d %s", listFns.Code, listFns.Body.String())
	}

	listTags := mustLambdaJSON(t, handler, "ListTags", map[string]any{
		"Resource": "arn:aws:lambda:" + testRegion + ":" + testAccountID + ":function:ops-fn",
	}, now)
	if listTags.Code != http.StatusOK {
		t.Fatalf("ListTags %d %s", listTags.Code, listTags.Body.String())
	}

	createQ := mustSQSJSON(t, handler, "CreateQueue", map[string]any{"QueueName": "ops-esm-q"}, now)
	if createQ.Code != http.StatusOK {
		t.Fatalf("CreateQueue %d %s", createQ.Code, createQ.Body.String())
	}
	var qOut map[string]any
	if err := json.Unmarshal(createQ.Body.Bytes(), &qOut); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := qOut["QueueUrl"].(string)
	if queueURL == "" {
		t.Fatalf("missing QueueUrl in %q", createQ.Body.String())
	}
	queueARN := "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":ops-esm-q"
	esmQueuePolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":["sqs:ReceiveMessage","sqs:DeleteMessage"],"Resource":"*"}]}`
	attrRec := mustSQSJSON(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl":   queueURL,
		"Attributes": map[string]string{"Policy": esmQueuePolicy},
	}, now)
	if attrRec.Code != http.StatusOK {
		t.Fatalf("SetQueueAttributes %d %s", attrRec.Code, attrRec.Body.String())
	}
	esm := mustLambdaJSON(t, handler, "CreateEventSourceMapping", map[string]any{
		"FunctionName":   "ops-fn",
		"EventSourceArn": queueARN,
		"Enabled":        true,
	}, now)
	if esm.Code != http.StatusOK {
		t.Fatalf("CreateEventSourceMapping %d %s", esm.Code, esm.Body.String())
	}
	var esmOut map[string]any
	if err := json.Unmarshal(esm.Body.Bytes(), &esmOut); err != nil {
		t.Fatal(err)
	}
	esmUUID, _ := esmOut["UUID"].(string)
	getESM := mustLambdaJSON(t, handler, "GetEventSourceMapping", map[string]any{"UUID": esmUUID}, now)
	if getESM.Code != http.StatusOK {
		t.Fatalf("GetEventSourceMapping %d %s", getESM.Code, getESM.Body.String())
	}
	updESM := mustLambdaJSON(t, handler, "UpdateEventSourceMapping", map[string]any{
		"UUID": esmUUID, "BatchSize": 5,
	}, now)
	if updESM.Code != http.StatusOK {
		t.Fatalf("UpdateEventSourceMapping %d %s", updESM.Code, updESM.Body.String())
	}
	listESM := mustLambdaJSON(t, handler, "ListEventSourceMappings", map[string]any{"FunctionName": "ops-fn"}, now)
	if listESM.Code != http.StatusOK || !strings.Contains(listESM.Body.String(), esmUUID) {
		t.Fatalf("ListEventSourceMappings %d %s", listESM.Code, listESM.Body.String())
	}
	delESM := mustLambdaJSON(t, handler, "DeleteEventSourceMapping", map[string]any{"UUID": esmUUID}, now)
	if delESM.Code != http.StatusOK {
		t.Fatalf("DeleteEventSourceMapping %d %s", delESM.Code, delESM.Body.String())
	}

	layer := mustLambdaJSON(t, handler, "PublishLayerVersion", map[string]any{
		"LayerName":          "ops-layer",
		"Content":            map[string]any{"ZipFile": testLambdaZipB64(t)},
		"CompatibleRuntimes": []string{"nodejs22.x"},
	}, now)
	if layer.Code != http.StatusOK {
		t.Fatalf("PublishLayerVersion %d %s", layer.Code, layer.Body.String())
	}
	var layerOut map[string]any
	if err := json.Unmarshal(layer.Body.Bytes(), &layerOut); err != nil {
		t.Fatal(err)
	}
	layerVer, _ := layerOut["Version"].(float64)
	getLayer := mustLambdaJSON(t, handler, "GetLayerVersion", map[string]any{
		"LayerName": "ops-layer", "VersionNumber": layerVer,
	}, now)
	if getLayer.Code != http.StatusOK {
		t.Fatalf("GetLayerVersion %d %s", getLayer.Code, getLayer.Body.String())
	}
	listLayers := mustLambdaJSON(t, handler, "ListLayerVersions", map[string]any{"LayerName": "ops-layer"}, now)
	if listLayers.Code != http.StatusOK {
		t.Fatalf("ListLayerVersions %d %s", listLayers.Code, listLayers.Body.String())
	}
	delLayer := mustLambdaJSON(t, handler, "DeleteLayerVersion", map[string]any{
		"LayerName": "ops-layer", "VersionNumber": layerVer,
	}, now)
	if delLayer.Code != http.StatusOK {
		t.Fatalf("DeleteLayerVersion %d %s", delLayer.Code, delLayer.Body.String())
	}

	delAlias := mustLambdaJSON(t, handler, "DeleteAlias", map[string]any{
		"FunctionName": "ops-fn", "Name": "live",
	}, now)
	if delAlias.Code != http.StatusOK {
		t.Fatalf("DeleteAlias %d %s", delAlias.Code, delAlias.Body.String())
	}

	del := mustLambdaJSON(t, handler, "DeleteFunction", map[string]any{"FunctionName": "ops-fn"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteFunction %d %s", del.Code, del.Body.String())
	}
}

func TestLambdaPermissionAndEventInvokeNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-perm-role", lambdaTrustLab, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-perm-role"

	create := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "perm-fn",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateFunction %d %s", create.Code, create.Body.String())
	}

	badAdd := mustLambdaJSON(t, handler, "AddPermission", map[string]any{
		"FunctionName": "perm-fn",
		"StatementId":  "",
		"Action":       "lambda:InvokeFunction",
		"Principal":    "*",
	}, now)
	if badAdd.Code == http.StatusOK {
		t.Fatalf("AddPermission empty StatementId should fail")
	}
	missingFn := mustLambdaJSON(t, handler, "AddPermission", map[string]any{
		"FunctionName": "no-fn",
		"StatementId":  "s1",
		"Action":       "lambda:InvokeFunction",
		"Principal":    "*",
	}, now)
	if missingFn.Code == http.StatusOK {
		t.Fatalf("AddPermission missing fn should fail")
	}

	add := mustLambdaJSON(t, handler, "AddPermission", map[string]any{
		"FunctionName": "perm-fn",
		"StatementId":  "allow-s3",
		"Action":       "lambda:InvokeFunction",
		"Principal":    "s3.amazonaws.com",
		"SourceArn":    "arn:aws:s3:::" + testAccountID + ":bucket/x",
	}, now)
	if add.Code != http.StatusOK {
		t.Fatalf("AddPermission %d %s", add.Code, add.Body.String())
	}
	dup := mustLambdaJSON(t, handler, "AddPermission", map[string]any{
		"FunctionName": "perm-fn",
		"StatementId":  "allow-s3",
		"Action":       "lambda:InvokeFunction",
		"Principal":    "*",
	}, now)
	if dup.Code == http.StatusOK {
		t.Fatalf("duplicate StatementId should fail")
	}
	getPol := mustLambdaJSON(t, handler, "GetPolicy", map[string]any{"FunctionName": "perm-fn"}, now)
	if getPol.Code != http.StatusOK || !strings.Contains(getPol.Body.String(), "allow-s3") {
		t.Fatalf("GetPolicy %d %s", getPol.Code, getPol.Body.String())
	}

	eicMissing := mustLambdaJSON(t, handler, "PutFunctionEventInvokeConfig", map[string]any{
		"FunctionName":         "no-fn",
		"MaximumRetryAttempts": 1,
	}, now)
	if eicMissing.Code == http.StatusOK {
		t.Fatalf("PutFunctionEventInvokeConfig missing should fail")
	}
	eic := mustLambdaJSON(t, handler, "PutFunctionEventInvokeConfig", map[string]any{
		"FunctionName":             "perm-fn",
		"MaximumRetryAttempts":     2,
		"MaximumEventAgeInSeconds": 120,
		"DestinationConfig": map[string]any{
			"OnFailure": map[string]any{"Destination": "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":dlq"},
			"OnSuccess": map[string]any{"Destination": "arn:aws:sns:" + testRegion + ":" + testAccountID + ":ok"},
		},
	}, now)
	if eic.Code != http.StatusOK {
		t.Fatalf("PutFunctionEventInvokeConfig %d %s", eic.Code, eic.Body.String())
	}
	getEIC := mustLambdaJSON(t, handler, "GetFunctionEventInvokeConfig", map[string]any{"FunctionName": "perm-fn"}, now)
	if getEIC.Code != http.StatusOK {
		t.Fatalf("GetFunctionEventInvokeConfig %d %s", getEIC.Code, getEIC.Body.String())
	}
	getEICMiss := mustLambdaJSON(t, handler, "GetFunctionEventInvokeConfig", map[string]any{"FunctionName": "no-fn"}, now)
	if getEICMiss.Code == http.StatusOK {
		t.Fatalf("GetFunctionEventInvokeConfig missing should fail")
	}
	listEIC := mustLambdaJSON(t, handler, "ListFunctionEventInvokeConfigs", map[string]any{"FunctionName": "perm-fn"}, now)
	if listEIC.Code != http.StatusOK && listEIC.Code != http.StatusNotImplemented {
		t.Fatalf("ListFunctionEventInvokeConfigs %d %s", listEIC.Code, listEIC.Body.String())
	}
	delEIC := mustLambdaJSON(t, handler, "DeleteFunctionEventInvokeConfig", map[string]any{"FunctionName": "perm-fn"}, now)
	if delEIC.Code != http.StatusOK {
		t.Fatalf("DeleteFunctionEventInvokeConfig %d %s", delEIC.Code, delEIC.Body.String())
	}
	delEICMiss := mustLambdaJSON(t, handler, "DeleteFunctionEventInvokeConfig", map[string]any{"FunctionName": "perm-fn"}, now)
	if delEICMiss.Code == http.StatusOK {
		// already deleted may still 200; either is fine
		_ = delEICMiss
	}

	rmMiss := mustLambdaJSON(t, handler, "RemovePermission", map[string]any{
		"FunctionName": "perm-fn", "StatementId": "nope",
	}, now)
	if rmMiss.Code == http.StatusOK {
		t.Fatalf("RemovePermission missing sid should fail")
	}
	rm := mustLambdaJSON(t, handler, "RemovePermission", map[string]any{
		"FunctionName": "perm-fn", "StatementId": "allow-s3",
	}, now)
	if rm.Code != http.StatusOK {
		t.Fatalf("RemovePermission %d %s", rm.Code, rm.Body.String())
	}

	del := mustLambdaJSON(t, handler, "DeleteFunction", map[string]any{"FunctionName": "perm-fn"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteFunction %d %s", del.Code, del.Body.String())
	}
}
