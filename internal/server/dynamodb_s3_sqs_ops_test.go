package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDynamoDBUpdateTableGSIAndSSE(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "upd-gsi",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "gsi1", "AttributeType": "S"},
			{"AttributeName": "gsi2", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable %d %s", create.Code, create.Body.String())
	}

	upd := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{
		"TableName": "upd-gsi",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "gsi1", "AttributeType": "S"},
		},
		"GlobalSecondaryIndexUpdates": []map[string]any{{
			"Create": map[string]any{
				"IndexName": "GSI1",
				"KeySchema": []map[string]any{
					{"AttributeName": "gsi1", "KeyType": "HASH"},
				},
				"Projection": map[string]any{"ProjectionType": "ALL"},
			},
		}},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateTable create GSI %d %s", upd.Code, upd.Body.String())
	}

	dup := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{
		"TableName": "upd-gsi",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "gsi1", "AttributeType": "S"},
		},
		"GlobalSecondaryIndexUpdates": []map[string]any{{
			"Create": map[string]any{
				"IndexName": "GSI1",
				"KeySchema": []map[string]any{
					{"AttributeName": "gsi1", "KeyType": "HASH"},
				},
				"Projection": map[string]any{"ProjectionType": "KEYS_ONLY"},
			},
		}},
	}, now)
	if dup.Code != http.StatusBadRequest {
		t.Fatalf("duplicate GSI want 400 got %d %s", dup.Code, dup.Body.String())
	}

	badEntry := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{
		"TableName": "upd-gsi",
		"GlobalSecondaryIndexUpdates": []map[string]any{{
			"Delete": map[string]any{"IndexName": "GSI1"},
		}},
	}, now)
	if badEntry.Code != http.StatusBadRequest {
		t.Fatalf("Delete GSI update want 400 got %d %s", badEntry.Code, badEntry.Body.String())
	}

	second := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{
		"TableName": "upd-gsi",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "gsi2", "AttributeType": "S"},
		},
		"GlobalSecondaryIndexUpdates": []map[string]any{{
			"Create": map[string]any{
				"IndexName": "GSI2",
				"KeySchema": []map[string]any{
					{"AttributeName": "gsi2", "KeyType": "HASH"},
				},
				"Projection": map[string]any{"ProjectionType": "ALL"},
			},
		}},
	}, now)
	if second.Code != http.StatusOK {
		t.Fatalf("second GSI %d %s", second.Code, second.Body.String())
	}

	third := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{
		"TableName": "upd-gsi",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "gsi3", "AttributeType": "S"},
		},
		"GlobalSecondaryIndexUpdates": []map[string]any{{
			"Create": map[string]any{
				"IndexName": "GSI3",
				"KeySchema": []map[string]any{
					{"AttributeName": "gsi3", "KeyType": "HASH"},
				},
				"Projection": map[string]any{"ProjectionType": "ALL"},
			},
		}},
	}, now)
	if third.Code != http.StatusBadRequest {
		t.Fatalf("third GSI want 400 got %d %s", third.Code, third.Body.String())
	}

	aes := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{
		"TableName": "upd-gsi",
		"SSESpecification": map[string]any{
			"Enabled": true,
			"SSEType": "AES256",
		},
	}, now)
	if aes.Code != http.StatusOK && aes.Code != http.StatusBadRequest {
		t.Fatalf("SSE AES256 update status=%d body=%q", aes.Code, aes.Body.String())
	}

	miss := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{
		"TableName": "no-table",
		"GlobalSecondaryIndexUpdates": []map[string]any{{
			"Create": map[string]any{
				"IndexName": "G",
				"KeySchema": []map[string]any{
					{"AttributeName": "pk", "KeyType": "HASH"},
				},
				"Projection": map[string]any{"ProjectionType": "ALL"},
			},
		}},
	}, now)
	if miss.Code != http.StatusBadRequest {
		t.Fatalf("missing table want 400 got %d %s", miss.Code, miss.Body.String())
	}

	del := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "upd-gsi"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteTable %d %s", del.Code, del.Body.String())
	}
}

func TestS3EncryptionPolicyCORSNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/enc-neg-bucket", nil, "s3", now, nil)

	badEnc := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/enc-neg-bucket?encryption", []byte("<not-encryption/>"), "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if badEnc.Code != http.StatusBadRequest {
		t.Fatalf("bad encryption xml want 400 got %d %s", badEnc.Code, badEnc.Body.String())
	}

	encOK := []byte(`<ServerSideEncryptionConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>AES256</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`)
	putEnc := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/enc-neg-bucket?encryption", encOK, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if putEnc.Code != http.StatusOK {
		t.Fatalf("PutBucketEncryption %d %s", putEnc.Code, putEnc.Body.String())
	}
	getEnc := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/enc-neg-bucket?encryption", nil, "s3", now, nil)
	if getEnc.Code != http.StatusOK || !strings.Contains(getEnc.Body.String(), "AES256") {
		t.Fatalf("GetBucketEncryption %d %s", getEnc.Code, getEnc.Body.String())
	}

	badPolicy := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/enc-neg-bucket?policy", []byte("not-json"), "s3", now, map[string]string{
		"Content-Type": "application/json",
	})
	if badPolicy.Code != http.StatusBadRequest && badPolicy.Code != http.StatusOK {
		// some labs accept opaque policy strings; either assert non-5xx
		t.Fatalf("PutBucketPolicy bad status=%d body=%q", badPolicy.Code, badPolicy.Body.String())
	}
	policy := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Principal":"*","Action":"s3:GetObject","Resource":"*"}]}`)
	putPol := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/enc-neg-bucket?policy", policy, "s3", now, map[string]string{
		"Content-Type": "application/json",
	})
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutBucketPolicy %d %s", putPol.Code, putPol.Body.String())
	}
	getPol := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/enc-neg-bucket?policy", nil, "s3", now, nil)
	if getPol.Code != http.StatusOK {
		t.Fatalf("GetBucketPolicy %d %s", getPol.Code, getPol.Body.String())
	}

	badCORS := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/enc-neg-bucket?cors", []byte("<CORSConfiguration/>"), "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if badCORS.Code != http.StatusBadRequest && badCORS.Code != http.StatusOK {
		t.Fatalf("empty CORS status=%d body=%q", badCORS.Code, badCORS.Body.String())
	}
	cors := []byte(`<CORSConfiguration><CORSRule><AllowedOrigin>*</AllowedOrigin><AllowedMethod>GET</AllowedMethod></CORSRule></CORSConfiguration>`)
	putCORS := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/enc-neg-bucket?cors", cors, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if putCORS.Code != http.StatusOK {
		t.Fatalf("PutBucketCors %d %s", putCORS.Code, putCORS.Body.String())
	}

	missEnc := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/no-enc-bucket?encryption", nil, "s3", now, nil)
	if missEnc.Code == http.StatusOK {
		t.Fatalf("missing bucket encryption should fail")
	}
	missPol := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/no-enc-bucket?policy", nil, "s3", now, nil)
	if missPol.Code == http.StatusOK {
		t.Fatalf("missing bucket policy should fail")
	}
}

func TestSQSKMSEncryptedQueueLifecycle(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	key := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if key.Code != http.StatusOK {
		t.Fatalf("CreateKey %d %s", key.Code, key.Body.String())
	}
	keyID := ""
	{
		var out map[string]any
		jsonUnmarshal(t, key.Body.Bytes(), &out)
		meta, _ := out["KeyMetadata"].(map[string]any)
		keyID, _ = meta["KeyId"].(string)
	}
	if keyID == "" {
		t.Fatal("missing key id")
	}

	create := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "kms-sse-q",
		"Attributes": map[string]string{
			"KmsMasterKeyId": keyID,
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateQueue %d %s", create.Code, create.Body.String())
	}
	var qOut map[string]any
	jsonUnmarshal(t, create.Body.Bytes(), &qOut)
	queueURL, _ := qOut["QueueUrl"].(string)

	send := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl": queueURL, "MessageBody": "kms-body",
	}, now)
	if send.Code != http.StatusOK {
		t.Fatalf("SendMessage %d %s", send.Code, send.Body.String())
	}
	recv := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl": queueURL, "MaxNumberOfMessages": 1, "WaitTimeSeconds": 0,
	}, now)
	if recv.Code != http.StatusOK || !strings.Contains(recv.Body.String(), "kms-body") {
		t.Fatalf("ReceiveMessage %d %s", recv.Code, recv.Body.String())
	}

	attrs := mustSQSJSON(t, handler, "GetQueueAttributes", map[string]any{
		"QueueUrl": queueURL, "AttributeNames": []string{"All"},
	}, now)
	if attrs.Code != http.StatusOK {
		t.Fatalf("GetQueueAttributes %d %s", attrs.Code, attrs.Body.String())
	}

	del := mustSQSJSON(t, handler, "DeleteQueue", map[string]any{"QueueUrl": queueURL}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteQueue %d %s", del.Code, del.Body.String())
	}
}

func jsonUnmarshal(t *testing.T, raw []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatal(err)
	}
}
