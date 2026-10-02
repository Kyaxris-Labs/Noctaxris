package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDynamoStreamsGetRecordsBindsIteratorAccount(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "stream-bind",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"BillingMode": "PAY_PER_REQUEST",
		"StreamSpecification": map[string]any{
			"StreamEnabled":  true,
			"StreamViewType": "NEW_AND_OLD_IMAGES",
		},
	}, now)
	table, err := st.GetTable(testAccountID, "stream-bind")
	if err != nil {
		t.Fatal(err)
	}
	streamARN := store.DynamoStreamARN(testRegion, testAccountID, table.TableName, table.StreamLabel)
	itRec := mustJSONTarget(t, handler, "DynamoDBStreams_20120810.GetShardIterator", "dynamodb", map[string]any{
		"StreamArn":         streamARN,
		"ShardId":           store.LabDynamoStreamShardID,
		"ShardIteratorType": "TRIM_HORIZON",
	}, now)
	if itRec.Code != http.StatusOK {
		t.Fatalf("GetShardIterator status=%d body=%q", itRec.Code, itRec.Body.String())
	}
	var itOut map[string]any
	if err := json.Unmarshal(itRec.Body.Bytes(), &itOut); err != nil {
		t.Fatal(err)
	}
	iterator, _ := itOut["ShardIterator"].(string)
	if iterator == "" {
		t.Fatal("empty ShardIterator")
	}

	otherAccount := "000000000099"
	if err := st.EnsureRoot(otherAccount, "AKIAROOTACCT00099", "secret-other"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(otherAccount, "other-reader"); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(otherAccount, "other-reader")
	if err != nil {
		t.Fatal(err)
	}
	allow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"dynamodbstreams:GetRecords","Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(otherAccount, "StreamsGetAll", allow)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(otherAccount, "other-reader", polARN); err != nil {
		t.Fatal(err)
	}

	deny := mustJSONTargetCreds(t, handler, "DynamoDBStreams_20120810.GetRecords", "dynamodb", map[string]any{
		"ShardIterator": iterator,
	}, now, akid, secret)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("cross-account GetRecords status=%d body=%q", deny.Code, deny.Body.String())
	}
}
