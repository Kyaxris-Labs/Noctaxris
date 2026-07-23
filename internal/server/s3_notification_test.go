package server_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestS3BucketNotificationConfigurationCRUD(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	account := "000000000001"
	bucket := "notify-api-bucket"

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket, nil, "s3", now, nil)

	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "notify-api-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"s3.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + q.QueueARN + `","Condition":{"ArnLike":{"aws:SourceArn":"` + store.BucketARN(bucket) + `"}}}]}`
	if err := st.SetQueueAttributes(account, q.QueueName, map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}

	putBody := []byte(`<NotificationConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <QueueConfiguration>
    <Id>q-1</Id>
    <Queue>` + q.QueueARN + `</Queue>
    <Event>s3:ObjectCreated:*</Event>
    <Filter><S3Key><FilterRule><Name>prefix</Name><Value>inbox/</Value></FilterRule></S3Key></Filter>
  </QueueConfiguration>
  <EventBridgeConfiguration></EventBridgeConfiguration>
</NotificationConfiguration>`)
	putRec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"?notification", putBody, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutBucketNotificationConfiguration status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	getRec := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/"+bucket+"?notification", nil, "s3", now, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetBucketNotificationConfiguration status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	body := getRec.Body.String()
	if !strings.Contains(body, "<QueueConfiguration>") || !strings.Contains(body, q.QueueARN) {
		t.Fatalf("missing queue config in %q", body)
	}
	if !strings.Contains(body, "<EventBridgeConfiguration") {
		t.Fatalf("missing EventBridgeConfiguration in %q", body)
	}
	if !strings.Contains(body, "inbox/") {
		t.Fatalf("missing prefix filter in %q", body)
	}

	clearRec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"?notification",
		[]byte(`<NotificationConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></NotificationConfiguration>`),
		"s3", now, map[string]string{"Content-Type": "application/xml"})
	if clearRec.Code != http.StatusOK {
		t.Fatalf("clear notification status=%d body=%q", clearRec.Code, clearRec.Body.String())
	}
	emptyRec := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/"+bucket+"?notification", nil, "s3", now, nil)
	if emptyRec.Code != http.StatusOK {
		t.Fatalf("get cleared status=%d body=%q", emptyRec.Code, emptyRec.Body.String())
	}
	if strings.Contains(emptyRec.Body.String(), "<QueueConfiguration>") ||
		strings.Contains(emptyRec.Body.String(), "<EventBridgeConfiguration") {
		t.Fatalf("want empty notification config, got %q", emptyRec.Body.String())
	}
}

func TestS3BucketNotificationConfigurationRejectsUnvalidatedDestination(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	account := "000000000001"
	bucket := "notify-deny-api"

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket, nil, "s3", now, nil)
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "notify-deny-q", nil)
	if err != nil {
		t.Fatal(err)
	}

	putBody := []byte(`<NotificationConfiguration>
  <QueueConfiguration>
    <Queue>` + q.QueueARN + `</Queue>
    <Event>s3:ObjectCreated:Put</Event>
  </QueueConfiguration>
</NotificationConfiguration>`)
	putRec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"?notification", putBody, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if putRec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q want 400", putRec.Code, putRec.Body.String())
	}
	if !strings.Contains(putRec.Body.String(), "InvalidArgument") {
		t.Fatalf("body=%q", putRec.Body.String())
	}
}
