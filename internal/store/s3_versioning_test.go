package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestS3DeleteMarkerCreateAndGet(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	bucket := "dm-bucket"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	if err := st.SetBucketVersioning(account, bucket, store.VersioningEnabled); err != nil {
		t.Fatal(err)
	}
	_, vid, err := st.PutObjectVersioned(account, bucket, "obj.txt", store.PutObjectMeta{
		Data: []byte("payload"), PlainSize: 7, ETag: "abc",
	})
	if err != nil || vid == "" {
		t.Fatalf("put versioned err=%v vid=%q", err, vid)
	}
	result, err := st.DeleteObjectVersioned(account, bucket, "obj.txt", "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.DeleteMarker || result.VersionID == "" {
		t.Fatalf("result=%+v want delete marker", result)
	}
	_, _, _, err = st.GetObjectVersion(account, bucket, "obj.txt", "")
	if !errors.Is(err, store.ErrNoSuchKey) {
		t.Fatalf("get latest after marker err=%v want ErrNoSuchKey", err)
	}
	_, _, data, err := st.GetObjectVersion(account, bucket, "obj.txt", vid)
	if err != nil || string(data) != "payload" {
		t.Fatalf("get old version data=%q err=%v", data, err)
	}
}

func TestS3DeleteObjectVersionByID(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	bucket := "delver-bucket"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	if err := st.SetBucketVersioning(account, bucket, store.VersioningEnabled); err != nil {
		t.Fatal(err)
	}
	_, id1, err := st.PutObjectVersioned(account, bucket, "k", store.PutObjectMeta{Data: []byte("1"), PlainSize: 1, ETag: "e1"})
	if err != nil {
		t.Fatal(err)
	}
	_, id2, err := st.PutObjectVersioned(account, bucket, "k", store.PutObjectMeta{Data: []byte("2"), PlainSize: 1, ETag: "e2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteObjectVersioned(account, bucket, "k", id1); err != nil {
		t.Fatal(err)
	}
	_, _, data, err := st.GetObjectVersion(account, bucket, "k", id1)
	if !errors.Is(err, store.ErrNoSuchKey) {
		t.Fatalf("deleted version get err=%v", err)
	}
	_, vid, data, err := st.GetObjectVersion(account, bucket, "k", "")
	if err != nil || vid != id2 || string(data) != "2" {
		t.Fatalf("latest vid=%q data=%q err=%v", vid, data, err)
	}
}

func TestS3ListObjectVersionsIncludesDeleteMarker(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	bucket := "list-dm"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	if err := st.SetBucketVersioning(account, bucket, store.VersioningEnabled); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutObjectVersioned(account, bucket, "x", store.PutObjectMeta{Data: []byte("a"), PlainSize: 1, ETag: "e"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteObjectVersioned(account, bucket, "x", ""); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListObjectVersions(account, bucket, "")
	if err != nil {
		t.Fatal(err)
	}
	var markers, versions int
	for _, v := range list {
		if v.IsDeleteMarker {
			markers++
			if !v.IsLatest {
				t.Fatalf("delete marker should be latest: %+v", v)
			}
		} else {
			versions++
		}
	}
	if markers != 1 || versions != 1 {
		t.Fatalf("markers=%d versions=%d list=%+v", markers, versions, list)
	}
}

func TestS3DeleteDeleteMarkerRestoresObject(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	bucket := "restore-dm"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	if err := st.SetBucketVersioning(account, bucket, store.VersioningEnabled); err != nil {
		t.Fatal(err)
	}
	_, objVID, err := st.PutObjectVersioned(account, bucket, "r", store.PutObjectMeta{Data: []byte("ok"), PlainSize: 2, ETag: "e"})
	if err != nil {
		t.Fatal(err)
	}
	marker, err := st.DeleteObjectVersioned(account, bucket, "r", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteObjectVersioned(account, bucket, "r", marker.VersionID); err != nil {
		t.Fatal(err)
	}
	_, vid, data, err := st.GetObjectVersion(account, bucket, "r", "")
	if err != nil || vid != objVID || string(data) != "ok" {
		t.Fatalf("restored vid=%q data=%q err=%v", vid, data, err)
	}
}
