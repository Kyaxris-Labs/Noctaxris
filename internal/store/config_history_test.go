package store_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestConfigHistoryOnlyWhenRecording(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "hist-a"); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendS3BucketConfigHistory(account, "hist-a", false); err != nil {
		t.Fatal(err)
	}
	items, err := st.GetResourceConfigHistory(account, "AWS::S3::Bucket", "hist-a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no history before recording: %#v", items)
	}
	if _, err := st.PutConfigRecorder(account, "default", "", "ALL"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "hist-bucket"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutConfigDeliveryChannel(account, "default", "hist-a", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.StartConfigRecorder(account, "default"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "hist-live"); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendS3BucketConfigHistory(account, "hist-live", false); err != nil {
		t.Fatal(err)
	}
	items, err = st.GetResourceConfigHistory(account, "AWS::S3::Bucket", "hist-live", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ConfigurationItemStatus != store.ConfigItemStatusOK {
		t.Fatalf("create history: %#v", items)
	}
	if err := st.DeleteBucket(account, "hist-live"); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendS3BucketConfigHistory(account, "hist-live", true); err != nil {
		t.Fatal(err)
	}
	items, err = st.GetResourceConfigHistory(account, "AWS::S3::Bucket", "hist-live", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected create+delete items: %#v", items)
	}
	if items[0].ConfigurationItemStatus != store.ConfigItemStatusOK {
		t.Fatalf("first item status: %#v", items[0])
	}
	if items[1].ConfigurationItemStatus != store.ConfigItemStatusResourceDeleted {
		t.Fatalf("second item status: %#v", items[1])
	}
}

func TestConfigObjectHistoryPutDelete(t *testing.T) {
	st := openTestStore(t)
	account := "000000000003"
	if _, err := st.CreateBucket(account, "obj-hist-delivery"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "obj-hist-data"); err != nil {
		t.Fatal(err)
	}
	key := "docs/readme.txt"
	resourceID := store.ConfigS3ObjectResourceID("obj-hist-data", key)

	if _, err := st.PutObject(account, "obj-hist-data", key, store.PutObjectMeta{
		Data: []byte("before-recording"), PlainSize: 16, ContentType: "text/plain", ETag: "etag-pre",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendS3ObjectConfigHistory(account, "obj-hist-data", key, false); err != nil {
		t.Fatal(err)
	}
	items, err := st.GetResourceConfigHistory(account, "AWS::S3::Object", resourceID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no object history before recording: %#v", items)
	}

	if _, err := st.PutConfigRecorder(account, "default", "", "ALL"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutConfigDeliveryChannel(account, "default", "obj-hist-delivery", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.StartConfigRecorder(account, "default"); err != nil {
		t.Fatal(err)
	}

	if _, err := st.PutObject(account, "obj-hist-data", key, store.PutObjectMeta{
		Data: []byte("hello-config"), PlainSize: 12, ContentType: "text/plain", ETag: "etag-ok",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendS3ObjectConfigHistory(account, "obj-hist-data", key, false); err != nil {
		t.Fatal(err)
	}
	items, err = st.GetResourceConfigHistory(account, "AWS::S3::Object", resourceID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ConfigurationItemStatus != store.ConfigItemStatusOK {
		t.Fatalf("put history: %#v", items)
	}
	if items[0].ResourceType != "AWS::S3::Object" || items[0].ResourceID != resourceID {
		t.Fatalf("resource identity: %#v", items[0])
	}
	if !strings.Contains(items[0].Configuration, `"bucket":"obj-hist-data"`) ||
		!strings.Contains(items[0].Configuration, `"key":"docs/readme.txt"`) {
		t.Fatalf("configuration body: %q", items[0].Configuration)
	}

	if err := st.DeleteObject(account, "obj-hist-data", key); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendS3ObjectConfigHistory(account, "obj-hist-data", key, true); err != nil {
		t.Fatal(err)
	}
	items, err = st.GetResourceConfigHistory(account, "AWS::S3::Object", resourceID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected put+delete items: %#v", items)
	}
	if items[1].ConfigurationItemStatus != store.ConfigItemStatusResourceDeleted {
		t.Fatalf("delete item status: %#v", items[1])
	}
}

func TestConfigHistoryChronologicalOrder(t *testing.T) {
	st := openTestStore(t)
	account := "000000000002"
	if _, err := st.PutConfigRecorder(account, "default", "", "ALL"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "order-bucket"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutConfigDeliveryChannel(account, "default", "order-bucket", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.StartConfigRecorder(account, "default"); err != nil {
		t.Fatal(err)
	}
	t1 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	if err := st.AppendConfigHistoryItem(account, store.ConfigConfigurationItem{
		ResourceType:                 "AWS::S3::Bucket",
		ResourceID:                   "order-bucket",
		ConfigurationItemStatus:      store.ConfigItemStatusOK,
		ConfigurationItemCaptureTime: t2.Format(time.RFC3339),
		Configuration:                `{"name":"order-bucket"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendConfigHistoryItem(account, store.ConfigConfigurationItem{
		ResourceType:                 "AWS::S3::Bucket",
		ResourceID:                   "order-bucket",
		ConfigurationItemStatus:      store.ConfigItemStatusResourceDeleted,
		ConfigurationItemCaptureTime: t1.Format(time.RFC3339),
		Configuration:                `{"name":"order-bucket"}`,
	}); err != nil {
		t.Fatal(err)
	}
	items, err := st.GetResourceConfigHistory(account, "AWS::S3::Bucket", "order-bucket", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items: %#v", items)
	}
	if items[0].ConfigurationItemCaptureTime != t1.Format(time.RFC3339) {
		t.Fatalf("oldest first: got %q want %q", items[0].ConfigurationItemCaptureTime, t1.Format(time.RFC3339))
	}
	if items[1].ConfigurationItemCaptureTime != t2.Format(time.RFC3339) {
		t.Fatalf("newer second: got %q want %q", items[1].ConfigurationItemCaptureTime, t2.Format(time.RFC3339))
	}
}
