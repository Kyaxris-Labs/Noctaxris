package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAppSyncAPIKeyResolverNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSAppSync.CreateGraphqlApi", "appsync", map[string]any{
		"name":               "neg-api",
		"authenticationType": "API_KEY",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateGraphqlApi %d %s", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	gql, _ := apiResp["graphqlApi"].(map[string]any)
	apiID, _ := gql["apiId"].(string)

	emptyCreate := mustJSONTarget(t, handler, "AWSAppSync.CreateGraphqlApi", "appsync", map[string]any{
		"name": "", "authenticationType": "API_KEY",
	}, now)
	if emptyCreate.Code == http.StatusOK {
		t.Fatalf("empty name should fail")
	}

	schema := mustJSONTarget(t, handler, "AWSAppSync.StartSchemaCreation", "appsync", map[string]any{
		"apiId": apiID, "definition": "type Query { ping: String }",
	}, now)
	if schema.Code != http.StatusOK {
		t.Fatalf("StartSchemaCreation %d %s", schema.Code, schema.Body.String())
	}
	schemaMiss := mustJSONTarget(t, handler, "AWSAppSync.StartSchemaCreation", "appsync", map[string]any{
		"apiId": "missing", "definition": "type Query { x: String }",
	}, now)
	if schemaMiss.Code == http.StatusOK {
		t.Fatalf("StartSchemaCreation missing should fail")
	}
	statusMiss := mustJSONTarget(t, handler, "AWSAppSync.GetSchemaCreationStatus", "appsync", map[string]any{
		"apiId": "missing",
	}, now)
	if statusMiss.Code == http.StatusOK {
		t.Fatalf("GetSchemaCreationStatus missing should fail")
	}

	key := mustJSONTarget(t, handler, "AWSAppSync.CreateApiKey", "appsync", map[string]any{"apiId": apiID}, now)
	if key.Code != http.StatusOK {
		t.Fatalf("CreateApiKey %d %s", key.Code, key.Body.String())
	}
	var keyResp map[string]any
	_ = json.Unmarshal(key.Body.Bytes(), &keyResp)
	apiKey, _ := keyResp["apiKey"].(map[string]any)
	keyID, _ := apiKey["id"].(string)
	keyMiss := mustJSONTarget(t, handler, "AWSAppSync.CreateApiKey", "appsync", map[string]any{"apiId": "missing"}, now)
	if keyMiss.Code == http.StatusOK {
		t.Fatalf("CreateApiKey missing api should fail")
	}
	listKeys := mustJSONTarget(t, handler, "AWSAppSync.ListApiKeys", "appsync", map[string]any{"apiId": apiID}, now)
	if listKeys.Code != http.StatusOK {
		t.Fatalf("ListApiKeys %d %s", listKeys.Code, listKeys.Body.String())
	}
	delKeyMiss := mustJSONTarget(t, handler, "AWSAppSync.DeleteApiKey", "appsync", map[string]any{
		"apiId": apiID, "id": "missing-key",
	}, now)
	if delKeyMiss.Code == http.StatusOK {
		t.Fatalf("DeleteApiKey missing should fail")
	}

	ds := mustJSONTarget(t, handler, "AWSAppSync.CreateDataSource", "appsync", map[string]any{
		"apiId": apiID, "name": "PingDS", "type": "AWS_LAMBDA",
		"lambdaConfig": map[string]any{
			"lambdaFunctionArn": "arn:aws:lambda:us-east-1:" + testAccountID + ":function:appsync-ping",
		},
	}, now)
	if ds.Code != http.StatusOK {
		t.Fatalf("CreateDataSource %d %s", ds.Code, ds.Body.String())
	}
	dsDup := mustJSONTarget(t, handler, "AWSAppSync.CreateDataSource", "appsync", map[string]any{
		"apiId": apiID, "name": "PingDS", "type": "AWS_LAMBDA",
		"lambdaConfig": map[string]any{
			"lambdaFunctionArn": "arn:aws:lambda:us-east-1:" + testAccountID + ":function:appsync-ping",
		},
	}, now)
	if dsDup.Code == http.StatusOK {
		t.Fatalf("dup data source should fail")
	}
	dsMissAPI := mustJSONTarget(t, handler, "AWSAppSync.CreateDataSource", "appsync", map[string]any{
		"apiId": "missing", "name": "X", "type": "AWS_LAMBDA",
		"lambdaConfig": map[string]any{
			"lambdaFunctionArn": "arn:aws:lambda:us-east-1:" + testAccountID + ":function:x",
		},
	}, now)
	if dsMissAPI.Code == http.StatusOK {
		t.Fatalf("CreateDataSource missing api should fail")
	}
	getDSMiss := mustJSONTarget(t, handler, "AWSAppSync.GetDataSource", "appsync", map[string]any{
		"apiId": apiID, "name": "nope",
	}, now)
	if getDSMiss.Code == http.StatusOK {
		t.Fatalf("GetDataSource missing should fail")
	}
	listDS := mustJSONTarget(t, handler, "AWSAppSync.ListDataSources", "appsync", map[string]any{"apiId": apiID}, now)
	if listDS.Code != http.StatusOK || !strings.Contains(listDS.Body.String(), "PingDS") {
		t.Fatalf("ListDataSources %d %s", listDS.Code, listDS.Body.String())
	}
	updDSMiss := mustJSONTarget(t, handler, "AWSAppSync.UpdateDataSource", "appsync", map[string]any{
		"apiId": apiID, "name": "nope", "type": "AWS_LAMBDA",
		"lambdaConfig": map[string]any{
			"lambdaFunctionArn": "arn:aws:lambda:us-east-1:" + testAccountID + ":function:x",
		},
	}, now)
	if updDSMiss.Code == http.StatusOK {
		t.Fatalf("UpdateDataSource missing should fail")
	}

	res := mustJSONTarget(t, handler, "AWSAppSync.CreateResolver", "appsync", map[string]any{
		"apiId": apiID, "typeName": "Query", "fieldName": "ping", "dataSourceName": "PingDS",
	}, now)
	if res.Code != http.StatusOK {
		t.Fatalf("CreateResolver %d %s", res.Code, res.Body.String())
	}
	resDup := mustJSONTarget(t, handler, "AWSAppSync.CreateResolver", "appsync", map[string]any{
		"apiId": apiID, "typeName": "Query", "fieldName": "ping", "dataSourceName": "PingDS",
	}, now)
	if resDup.Code == http.StatusOK {
		t.Fatalf("dup resolver should fail")
	}
	getResMiss := mustJSONTarget(t, handler, "AWSAppSync.GetResolver", "appsync", map[string]any{
		"apiId": apiID, "typeName": "Query", "fieldName": "missing",
	}, now)
	if getResMiss.Code == http.StatusOK {
		t.Fatalf("GetResolver missing should fail")
	}
	listRes := mustJSONTarget(t, handler, "AWSAppSync.ListResolvers", "appsync", map[string]any{
		"apiId": apiID, "typeName": "Query",
	}, now)
	if listRes.Code != http.StatusOK {
		t.Fatalf("ListResolvers %d %s", listRes.Code, listRes.Body.String())
	}
	updRes := mustJSONTarget(t, handler, "AWSAppSync.UpdateResolver", "appsync", map[string]any{
		"apiId": apiID, "typeName": "Query", "fieldName": "ping", "dataSourceName": "PingDS",
	}, now)
	if updRes.Code != http.StatusOK {
		t.Fatalf("UpdateResolver %d %s", updRes.Code, updRes.Body.String())
	}
	delRes := mustJSONTarget(t, handler, "AWSAppSync.DeleteResolver", "appsync", map[string]any{
		"apiId": apiID, "typeName": "Query", "fieldName": "ping",
	}, now)
	if delRes.Code != http.StatusOK {
		t.Fatalf("DeleteResolver %d %s", delRes.Code, delRes.Body.String())
	}
	delResMiss := mustJSONTarget(t, handler, "AWSAppSync.DeleteResolver", "appsync", map[string]any{
		"apiId": apiID, "typeName": "Query", "fieldName": "ping",
	}, now)
	if delResMiss.Code == http.StatusOK {
		t.Fatalf("DeleteResolver twice should fail")
	}

	delDS := mustJSONTarget(t, handler, "AWSAppSync.DeleteDataSource", "appsync", map[string]any{
		"apiId": apiID, "name": "PingDS",
	}, now)
	if delDS.Code != http.StatusOK {
		t.Fatalf("DeleteDataSource %d %s", delDS.Code, delDS.Body.String())
	}
	_ = mustJSONTarget(t, handler, "AWSAppSync.DeleteApiKey", "appsync", map[string]any{
		"apiId": apiID, "id": keyID,
	}, now)
	delAPI := mustJSONTarget(t, handler, "AWSAppSync.DeleteGraphqlApi", "appsync", map[string]any{"apiId": apiID}, now)
	if delAPI.Code != http.StatusOK {
		t.Fatalf("DeleteGraphqlApi %d %s", delAPI.Code, delAPI.Body.String())
	}
}

func TestSQSSetAttributesPurgeAndBatchNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "attrs-ops",
		"Attributes": map[string]string{
			"VisibilityTimeout": "30",
			"DelaySeconds":      "0",
		},
		"tags": map[string]string{"env": "lab"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateQueue %d %s", create.Code, create.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	queueURL, _ := created["QueueUrl"].(string)

	set := mustSQSJSON(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl": queueURL,
		"Attributes": map[string]string{
			"VisibilityTimeout":      "15",
			"ReceiveMessageWaitTimeSeconds": "1",
		},
	}, now)
	if set.Code != http.StatusOK {
		t.Fatalf("SetQueueAttributes %d %s", set.Code, set.Body.String())
	}
	setBad := mustSQSJSON(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl": "http://127.0.0.1:4566/000000000001/missing-q",
		"Attributes": map[string]string{"VisibilityTimeout": "15"},
	}, now)
	if setBad.Code == http.StatusOK {
		t.Fatalf("SetQueueAttributes missing queue should fail")
	}
	get := mustSQSJSON(t, handler, "GetQueueAttributes", map[string]any{
		"QueueUrl": queueURL, "AttributeNames": []string{"All"},
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "VisibilityTimeout") {
		t.Fatalf("GetQueueAttributes %d %s", get.Code, get.Body.String())
	}

	batch := mustSQSJSON(t, handler, "SendMessageBatch", map[string]any{
		"QueueUrl": queueURL,
		"Entries": []map[string]any{
			{"Id": "1", "MessageBody": "a", "DelaySeconds": 0},
			{"Id": "2", "MessageBody": ""},
			{"Id": "3", "MessageBody": "c"},
		},
	}, now)
	if batch.Code != http.StatusOK {
		t.Fatalf("SendMessageBatch %d %s", batch.Code, batch.Body.String())
	}
	emptyBatch := mustSQSJSON(t, handler, "SendMessageBatch", map[string]any{
		"QueueUrl": queueURL, "Entries": []map[string]any{},
	}, now)
	if emptyBatch.Code == http.StatusOK {
		t.Fatalf("empty batch should fail")
	}

	recv := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl": queueURL, "MaxNumberOfMessages": 10, "WaitTimeSeconds": 0,
	}, now)
	if recv.Code != http.StatusOK {
		t.Fatalf("ReceiveMessage %d %s", recv.Code, recv.Body.String())
	}
	var recvOut map[string]any
	_ = json.Unmarshal(recv.Body.Bytes(), &recvOut)
	entries := []map[string]any{}
	if msgs, ok := recvOut["Messages"].([]any); ok {
		for i, m := range msgs {
			mm := m.(map[string]any)
			entries = append(entries, map[string]any{
				"Id":            string(rune('a' + i)),
				"ReceiptHandle": mm["ReceiptHandle"],
			})
		}
	}
	entries = append(entries, map[string]any{"Id": "bad", "ReceiptHandle": "missing"})
	delBatch := mustSQSJSON(t, handler, "DeleteMessageBatch", map[string]any{
		"QueueUrl": queueURL, "Entries": entries,
	}, now)
	if delBatch.Code != http.StatusOK {
		t.Fatalf("DeleteMessageBatch %d %s", delBatch.Code, delBatch.Body.String())
	}

	purge := mustSQSJSON(t, handler, "PurgeQueue", map[string]any{"QueueUrl": queueURL}, now)
	if purge.Code != http.StatusOK {
		t.Fatalf("PurgeQueue %d %s", purge.Code, purge.Body.String())
	}
	urlMiss := mustSQSJSON(t, handler, "GetQueueUrl", map[string]any{"QueueName": "no-such-queue"}, now)
	if urlMiss.Code == http.StatusOK {
		t.Fatalf("GetQueueUrl missing should fail")
	}
	del := mustSQSJSON(t, handler, "DeleteQueue", map[string]any{"QueueUrl": queueURL}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteQueue %d %s", del.Code, del.Body.String())
	}
	delAgain := mustSQSJSON(t, handler, "DeleteQueue", map[string]any{"QueueUrl": queueURL}, now)
	if delAgain.Code == http.StatusOK {
		t.Fatalf("DeleteQueue twice should fail")
	}
}

func TestLogsStreamEventsAndFilterOps(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	group := "/lab/stream-ops"

	if rec := mustLogsJSON(t, handler, "CreateLogGroup", map[string]any{"logGroupName": group}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreateLogGroup %d %s", rec.Code, rec.Body.String())
	}
	dup := mustLogsJSON(t, handler, "CreateLogGroup", map[string]any{"logGroupName": group}, now)
	if dup.Code == http.StatusOK {
		t.Fatalf("dup log group should fail")
	}
	stream := mustLogsJSON(t, handler, "CreateLogStream", map[string]any{
		"logGroupName": group, "logStreamName": "s1",
	}, now)
	if stream.Code != http.StatusOK {
		t.Fatalf("CreateLogStream %d %s", stream.Code, stream.Body.String())
	}
	streamMiss := mustLogsJSON(t, handler, "CreateLogStream", map[string]any{
		"logGroupName": "/lab/missing", "logStreamName": "s1",
	}, now)
	if streamMiss.Code == http.StatusOK {
		t.Fatalf("CreateLogStream missing group should fail")
	}

	put := mustLogsJSON(t, handler, "PutLogEvents", map[string]any{
		"logGroupName":  group,
		"logStreamName": "s1",
		"logEvents": []map[string]any{
			{"timestamp": float64(now.UnixMilli()), "message": "hello ERROR"},
			{"timestamp": float64(now.UnixMilli() + 1), "message": "ok"},
		},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutLogEvents %d %s", put.Code, put.Body.String())
	}
	descStreams := mustLogsJSON(t, handler, "DescribeLogStreams", map[string]any{"logGroupName": group}, now)
	if descStreams.Code != http.StatusOK || !strings.Contains(descStreams.Body.String(), "s1") {
		t.Fatalf("DescribeLogStreams %d %s", descStreams.Code, descStreams.Body.String())
	}
	descGroups := mustLogsJSON(t, handler, "DescribeLogGroups", map[string]any{}, now)
	if descGroups.Code != http.StatusOK || !strings.Contains(descGroups.Body.String(), group) {
		t.Fatalf("DescribeLogGroups %d %s", descGroups.Code, descGroups.Body.String())
	}
	get := mustLogsJSON(t, handler, "GetLogEvents", map[string]any{
		"logGroupName": group, "logStreamName": "s1",
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "hello") {
		t.Fatalf("GetLogEvents %d %s", get.Code, get.Body.String())
	}
	filter := mustLogsJSON(t, handler, "FilterLogEvents", map[string]any{
		"logGroupName": group, "filterPattern": "ERROR",
	}, now)
	if filter.Code != http.StatusOK {
		t.Fatalf("FilterLogEvents %d %s", filter.Code, filter.Body.String())
	}
	putRet := mustLogsJSON(t, handler, "PutRetentionPolicy", map[string]any{
		"logGroupName": group, "retentionInDays": 14,
	}, now)
	if putRet.Code != http.StatusOK {
		t.Fatalf("PutRetentionPolicy %d %s", putRet.Code, putRet.Body.String())
	}

	delStream := mustLogsJSON(t, handler, "DeleteLogStream", map[string]any{
		"logGroupName": group, "logStreamName": "s1",
	}, now)
	if delStream.Code != http.StatusOK {
		t.Fatalf("DeleteLogStream %d %s", delStream.Code, delStream.Body.String())
	}
	delGroup := mustLogsJSON(t, handler, "DeleteLogGroup", map[string]any{"logGroupName": group}, now)
	if delGroup.Code != http.StatusOK {
		t.Fatalf("DeleteLogGroup %d %s", delGroup.Code, delGroup.Body.String())
	}
	delMiss := mustLogsJSON(t, handler, "DeleteLogGroup", map[string]any{"logGroupName": group}, now)
	if delMiss.Code == http.StatusOK {
		t.Fatalf("DeleteLogGroup twice should fail")
	}
}

func TestGlueDatabaseTableCrawlerOps(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	db := mustJSONTarget(t, handler, "AWSGlue.CreateDatabase", "glue", map[string]any{
		"DatabaseInput": map[string]any{"Name": "ops_db", "Description": "lab"},
	}, now)
	if db.Code != http.StatusOK {
		t.Fatalf("CreateDatabase %d %s", db.Code, db.Body.String())
	}
	dup := mustJSONTarget(t, handler, "AWSGlue.CreateDatabase", "glue", map[string]any{
		"DatabaseInput": map[string]any{"Name": "ops_db"},
	}, now)
	if dup.Code == http.StatusOK {
		t.Fatalf("dup database should fail")
	}
	getDB := mustJSONTarget(t, handler, "AWSGlue.GetDatabase", "glue", map[string]any{"Name": "ops_db"}, now)
	if getDB.Code != http.StatusOK {
		t.Fatalf("GetDatabase %d %s", getDB.Code, getDB.Body.String())
	}
	listDB := mustJSONTarget(t, handler, "AWSGlue.GetDatabases", "glue", map[string]any{}, now)
	if listDB.Code != http.StatusOK || !strings.Contains(listDB.Body.String(), "ops_db") {
		t.Fatalf("GetDatabases %d %s", listDB.Code, listDB.Body.String())
	}

	tbl := mustJSONTarget(t, handler, "AWSGlue.CreateTable", "glue", map[string]any{
		"DatabaseName": "ops_db",
		"TableInput": map[string]any{
			"Name": "ops_tbl",
			"StorageDescriptor": map[string]any{
				"Columns": []map[string]any{{"Name": "id", "Type": "string"}},
				"Location": "s3://lab/ops/",
			},
		},
	}, now)
	if tbl.Code != http.StatusOK {
		t.Fatalf("CreateTable %d %s", tbl.Code, tbl.Body.String())
	}
	getTbl := mustJSONTarget(t, handler, "AWSGlue.GetTable", "glue", map[string]any{
		"DatabaseName": "ops_db", "Name": "ops_tbl",
	}, now)
	if getTbl.Code != http.StatusOK {
		t.Fatalf("GetTable %d %s", getTbl.Code, getTbl.Body.String())
	}
	listTbl := mustJSONTarget(t, handler, "AWSGlue.GetTables", "glue", map[string]any{"DatabaseName": "ops_db"}, now)
	if listTbl.Code != http.StatusOK {
		t.Fatalf("GetTables %d %s", listTbl.Code, listTbl.Body.String())
	}
	missTbl := mustJSONTarget(t, handler, "AWSGlue.GetTable", "glue", map[string]any{
		"DatabaseName": "ops_db", "Name": "missing",
	}, now)
	if missTbl.Code == http.StatusOK {
		t.Fatalf("GetTable missing should fail")
	}

	mustCreateIAMRole(t, handler, "glue-ops-role", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"glue.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/glue-ops-role"

	bkt := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/glue-ops-bkt", nil, "s3", now, nil)
	if bkt.Code < 200 || bkt.Code >= 300 {
		t.Fatalf("create glue bucket %d", bkt.Code)
	}
	_ = mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/glue-ops-bkt/data/rows.csv", []byte("id,name\n1,a\n"), "s3", now, map[string]string{
		"Content-Type": "text/csv",
	})

	crawler := mustJSONTarget(t, handler, "AWSGlue.CreateCrawler", "glue", map[string]any{
		"Name": "ops-crawler",
		"Role": roleARN,
		"Targets": map[string]any{
			"S3Targets": []map[string]any{{"Path": "s3://glue-ops-bkt/data/"}},
		},
		"DatabaseName": "ops_db",
	}, now)
	if crawler.Code != http.StatusOK {
		t.Fatalf("CreateCrawler %d %s", crawler.Code, crawler.Body.String())
	}
	getCrawler := mustJSONTarget(t, handler, "AWSGlue.GetCrawler", "glue", map[string]any{"Name": "ops-crawler"}, now)
	if getCrawler.Code != http.StatusOK {
		t.Fatalf("GetCrawler %d %s", getCrawler.Code, getCrawler.Body.String())
	}
	listCrawler := mustJSONTarget(t, handler, "AWSGlue.ListCrawlers", "glue", map[string]any{}, now)
	if listCrawler.Code != http.StatusOK {
		t.Fatalf("ListCrawlers %d %s", listCrawler.Code, listCrawler.Body.String())
	}
	start := mustJSONTarget(t, handler, "AWSGlue.StartCrawler", "glue", map[string]any{"Name": "ops-crawler"}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartCrawler %d %s", start.Code, start.Body.String())
	}
	_ = mustJSONTarget(t, handler, "AWSGlue.StopCrawler", "glue", map[string]any{"Name": "ops-crawler"}, now)
	delCrawler := mustJSONTarget(t, handler, "AWSGlue.DeleteCrawler", "glue", map[string]any{"Name": "ops-crawler"}, now)
	if delCrawler.Code != http.StatusOK {
		t.Fatalf("DeleteCrawler %d %s", delCrawler.Code, delCrawler.Body.String())
	}

	delTbl := mustJSONTarget(t, handler, "AWSGlue.DeleteTable", "glue", map[string]any{
		"DatabaseName": "ops_db", "Name": "ops_tbl",
	}, now)
	if delTbl.Code != http.StatusOK {
		t.Fatalf("DeleteTable %d %s", delTbl.Code, delTbl.Body.String())
	}
	delDB := mustJSONTarget(t, handler, "AWSGlue.DeleteDatabase", "glue", map[string]any{"Name": "ops_db"}, now)
	if delDB.Code != http.StatusOK {
		t.Fatalf("DeleteDatabase %d %s", delDB.Code, delDB.Body.String())
	}
}
