package sdk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestAthenaCloudTrailRecordsLikeJSONExtract(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	s3c := newS3(t, cfg)
	gluec := newGlue(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	bucket := strings.ToLower(prefix + "-athct")
	dbName := strings.ReplaceAll(prefix, "-", "_") + "_ctdb"
	if len(dbName) > 48 {
		dbName = dbName[:48]
	}
	tableName := "events"
	objKey := "trail/delivery.json"

	payload := map[string]any{
		"Records": []map[string]any{
			{
				"eventName": "AssumeRole", "eventID": "e1",
				"userIdentity": map[string]any{"type": "IAMUser", "userName": "alice"},
			},
			{
				"eventName": "PutObject", "eventID": "e2",
				"userIdentity": map[string]any{"type": "AWSService", "userName": "s3"},
			},
			{
				"eventName": "AssumeRoleWithSAML", "eventID": "e3",
				"userIdentity": map[string]any{"type": "IAMUser", "userName": "bob"},
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if _, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		_, _ = gluec.DeleteTable(ctx, &glue.DeleteTableInput{DatabaseName: aws.String(dbName), Name: aws.String(tableName)})
		_, _ = gluec.DeleteDatabase(ctx, &glue.DeleteDatabaseInput{Name: aws.String(dbName)})
		_, _ = s3c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(objKey)})
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})
	if _, err := s3c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(objKey), Body: bytes.NewReader(raw),
	}); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	if _, err := gluec.CreateDatabase(ctx, &glue.CreateDatabaseInput{
		DatabaseInput: &gluetypes.DatabaseInput{Name: aws.String(dbName)},
	}); err != nil {
		t.Fatalf("CreateDatabase: %v", err)
	}
	if _, err := gluec.CreateTable(ctx, &glue.CreateTableInput{
		DatabaseName: aws.String(dbName),
		TableInput: &gluetypes.TableInput{
			Name: aws.String(tableName),
			StorageDescriptor: &gluetypes.StorageDescriptor{
				Location: aws.String("s3://" + bucket + "/trail/"),
				Columns: []gluetypes.Column{
					{Name: aws.String("eventName"), Type: aws.String("string")},
					{Name: aws.String("eventID"), Type: aws.String("string")},
					{Name: aws.String("userIdentity"), Type: aws.String("string")},
				},
				InputFormat: aws.String("org.apache.hive.hcatalog.data.JsonSerDe"),
				SerdeInfo: &gluetypes.SerDeInfo{
					SerializationLibrary: aws.String("org.openx.data.jsonserde.JsonSerDe"),
				},
			},
		},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	unwrapRows := athenaQueryRows(t, "SELECT eventName, eventID FROM "+dbName+".events ORDER BY eventID", dbName)
	if len(unwrapRows) != 4 {
		t.Fatalf("Records unwrap want header+3 rows, got %#v", unwrapRows)
	}
	if cell(unwrapRows, 1, 0) != "AssumeRole" || cell(unwrapRows, 3, 0) != "AssumeRoleWithSAML" {
		t.Fatalf("unwrap rows=%#v", unwrapRows)
	}

	likeRows := athenaQueryRows(t, "SELECT eventName FROM "+dbName+".events WHERE eventName LIKE 'Assume%'", dbName)
	if len(likeRows) != 3 {
		t.Fatalf("LIKE want header+2 rows, got %#v", likeRows)
	}
	joined := cell(likeRows, 1, 0) + cell(likeRows, 2, 0)
	if !strings.Contains(joined, "AssumeRole") || !strings.Contains(joined, "AssumeRoleWithSAML") {
		t.Fatalf("LIKE rows=%#v", likeRows)
	}

	jxRows := athenaQueryRows(t,
		"SELECT eventName FROM "+dbName+".events WHERE json_extract(userIdentity, '$.type') = 'IAMUser'",
		dbName)
	if len(jxRows) != 3 {
		t.Fatalf("json_extract want header+2 rows, got %#v", jxRows)
	}
	jxJoined := cell(jxRows, 1, 0) + cell(jxRows, 2, 0)
	if !strings.Contains(jxJoined, "AssumeRole") || !strings.Contains(jxJoined, "AssumeRoleWithSAML") {
		t.Fatalf("json_extract rows=%#v", jxRows)
	}
}

func athenaQueryRows(t *testing.T, query, database string) [][]string {
	t.Helper()
	startStatus, startBody, startParsed := signedJSONTarget(t, "athena", "AmazonAthena.StartQueryExecution", map[string]any{
		"QueryString":           query,
		"QueryExecutionContext": map[string]any{"Database": database},
	})
	if startStatus != 200 {
		t.Fatalf("StartQueryExecution status=%d body=%s", startStatus, startBody)
	}
	qid, _ := startParsed["QueryExecutionId"].(string)
	if qid == "" {
		t.Fatalf("missing QueryExecutionId: %s", startBody)
	}

	getStatus, getBody, getParsed := signedJSONTarget(t, "athena", "AmazonAthena.GetQueryExecution", map[string]any{
		"QueryExecutionId": qid,
	})
	if getStatus != 200 {
		t.Fatalf("GetQueryExecution status=%d body=%s", getStatus, getBody)
	}
	qe, _ := getParsed["QueryExecution"].(map[string]any)
	status, _ := qe["Status"].(map[string]any)
	if state, _ := status["State"].(string); state != "SUCCEEDED" {
		t.Fatalf("query state=%v body=%s", status, getBody)
	}

	resStatus, resBody, resParsed := signedJSONTarget(t, "athena", "AmazonAthena.GetQueryResults", map[string]any{
		"QueryExecutionId": qid,
	})
	if resStatus != 200 {
		t.Fatalf("GetQueryResults status=%d body=%s", resStatus, resBody)
	}
	rs, _ := resParsed["ResultSet"].(map[string]any)
	rawRows, _ := rs["Rows"].([]any)
	out := make([][]string, 0, len(rawRows))
	for _, r := range rawRows {
		rm, _ := r.(map[string]any)
		data, _ := rm["Data"].([]any)
		row := make([]string, 0, len(data))
		for _, d := range data {
			dm, _ := d.(map[string]any)
			v, _ := dm["VarCharValue"].(string)
			row = append(row, v)
		}
		out = append(out, row)
	}
	return out
}

func cell(rows [][]string, r, c int) string {
	if r < 0 || r >= len(rows) || c < 0 || c >= len(rows[r]) {
		return ""
	}
	return rows[r][c]
}
