package store_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// Content TOCTOU is accepted: after unlock, a concurrent same-key Put/Delete may
// make GetObject disagree with the notification payload (lab best-effort).

func TestS3NotifyUnlockGetObjectAfterPut(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "notify-unlock-get"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "s3-unlock-get", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowS3QueueNotify(t, st, account, bucket, q.QueueName, q.QueueARN)
	if err := st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{
		QueueConfigs: []store.S3QueueConfig{{
			Events:   []string{"s3:ObjectCreated:Put"},
			QueueARN: q.QueueARN,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	body := []byte("readable-after-notify")
	if _, err := st.PutObject(account, bucket, "obj.txt", store.PutObjectMeta{
		Data: body, PlainSize: int64(len(body)), ETag: "etag-get",
	}); err != nil {
		t.Fatal(err)
	}

	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 notify, got %d", len(msgs))
	}
	if !strings.Contains(string(msgs[0].Body), `"eventName":"ObjectCreated:Put"`) {
		t.Fatalf("body=%s", msgs[0].Body)
	}

	meta, data, err := st.GetObject(account, bucket, "obj.txt")
	if err != nil {
		t.Fatal(err)
	}
	if meta.ETag != "etag-get" || !bytes.Equal(data, body) {
		t.Fatalf("GetObject after notify mismatch meta=%+v data=%q", meta, data)
	}
}

func TestS3NotifyUnlockDestinationDeny(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "notify-unlock-deny"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "s3-unlock-deny", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowS3QueueNotify(t, st, account, bucket, q.QueueName, q.QueueARN)
	if err := st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{
		QueueConfigs: []store.S3QueueConfig{{
			Events:   []string{"s3:ObjectCreated:Put"},
			QueueARN: q.QueueARN,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetQueueAttributes(account, q.QueueName, map[string]string{"Policy": ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(account, bucket, "a.txt", store.PutObjectMeta{Data: []byte("z"), PlainSize: 1}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("emit must re-check destination authz after unlock; got %d msgs", len(msgs))
	}
}

func TestS3NotifyUnlockConcurrentPut(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "notify-unlock-conc"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "s3-unlock-conc", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowS3QueueNotify(t, st, account, bucket, q.QueueName, q.QueueARN)
	if err := st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{
		QueueConfigs: []store.S3QueueConfig{{
			Events:   []string{"s3:ObjectCreated:Put"},
			QueueARN: q.QueueARN,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	const n = 16
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := bytes.Repeat([]byte{byte('A' + i%26)}, 32)
			_, err := st.PutObject(account, bucket, "same-key", store.PutObjectMeta{
				Data: body, PlainSize: int64(len(body)),
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("PutObject: %v", err)
		}
	}

	meta, data, err := st.GetObject(account, bucket, "same-key")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Size != 32 || len(data) != 32 {
		t.Fatalf("size=%d len(data)=%d", meta.Size, len(data))
	}
	if !bytes.Equal(bytes.Repeat(data[:1], 32), data) {
		t.Fatalf("blob not uniform after concurrent puts")
	}

	var total int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
		if err != nil {
			t.Fatal(err)
		}
		total += len(msgs)
		if total >= n {
			break
		}
		if len(msgs) == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if total < 1 {
		t.Fatalf("want at least one notify after concurrent puts, got %d", total)
	}
}

func TestS3NotifyUnlockVersionId(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "notify-unlock-ver"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	if err := st.SetBucketVersioning(account, bucket, store.VersioningEnabled); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "s3-unlock-ver", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowS3QueueNotify(t, st, account, bucket, q.QueueName, q.QueueARN)
	if err := st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{
		QueueConfigs: []store.S3QueueConfig{{
			Events:   []string{"s3:ObjectCreated:Put", "s3:ObjectRemoved:*"},
			QueueARN: q.QueueARN,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	_, versionID, err := st.PutObjectVersioned(account, bucket, "v.txt", store.PutObjectMeta{
		Data: []byte("v1"), PlainSize: 2, ETag: "ev1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if versionID == "" || versionID == "null" {
		t.Fatalf("want real version id, got %q", versionID)
	}

	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 put notify, got %d", len(msgs))
	}
	putBody := string(msgs[0].Body)
	if !strings.Contains(putBody, `"eventName":"ObjectCreated:Put"`) || !strings.Contains(putBody, `"versionId":"`+versionID+`"`) {
		t.Fatalf("put notify missing versionId body=%s", putBody)
	}

	marker, err := st.DeleteObjectVersioned(account, bucket, "v.txt", "")
	if err != nil {
		t.Fatal(err)
	}
	if !marker.DeleteMarker || marker.VersionID == "" {
		t.Fatalf("marker=%+v", marker)
	}
	msgs, err = st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 delete-marker notify, got %d", len(msgs))
	}
	delBody := string(msgs[0].Body)
	if !strings.Contains(delBody, `"eventName":"ObjectRemoved:DeleteMarkerCreated"`) ||
		!strings.Contains(delBody, `"versionId":"`+marker.VersionID+`"`) {
		t.Fatalf("delete-marker notify body=%s", delBody)
	}
}

func TestS3NotifyUnlockLockOrderWithReceive(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "notify-unlock-order"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "s3-unlock-order", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowS3QueueNotify(t, st, account, bucket, q.QueueName, q.QueueARN)
	if err := st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{
		QueueConfigs: []store.S3QueueConfig{{
			Events:   []string{"s3:ObjectCreated:Put"},
			QueueARN: q.QueueARN,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			if _, err := st.PutObject(account, bucket, "k", store.PutObjectMeta{
				Data: []byte{byte(i)}, PlainSize: 1,
			}); err != nil {
				t.Errorf("PutObject: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			if _, err := st.ReceiveMessages(account, q.QueueName, 10); err != nil {
				t.Errorf("ReceiveMessages: %v", err)
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("deadlock: Put+notify vs Receive did not finish")
	}
}
