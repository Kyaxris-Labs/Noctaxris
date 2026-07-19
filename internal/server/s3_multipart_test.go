package server_test

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestS3MultipartUploadPutGet(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mp-lab-bucket", nil, "s3", now, nil)

	createRec := mustS3(t, handler, http.MethodPost, "http://127.0.0.1:4566/mp-lab-bucket/big.bin?uploads", nil, "s3", now, map[string]string{
		"Content-Type": "application/octet-stream",
	})
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateMultipartUpload status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	uploadID := parseMultipartUploadID(t, createRec.Body.Bytes())
	if uploadID == "" {
		t.Fatal("missing upload id")
	}

	part1 := bytes.Repeat([]byte("a"), store.MinMultipartPartSize)
	part2 := []byte("tail")
	p1Rec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mp-lab-bucket/big.bin?partNumber=1&uploadId="+uploadID, part1, "s3", now, nil)
	if p1Rec.Code != http.StatusOK {
		t.Fatalf("UploadPart1 status=%d body=%q", p1Rec.Code, p1Rec.Body.String())
	}
	etag1 := strings.Trim(p1Rec.Header().Get("ETag"), `"`)
	p2Rec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mp-lab-bucket/big.bin?partNumber=2&uploadId="+uploadID, part2, "s3", now, nil)
	if p2Rec.Code != http.StatusOK {
		t.Fatalf("UploadPart2 status=%d body=%q", p2Rec.Code, p2Rec.Body.String())
	}
	etag2 := strings.Trim(p2Rec.Header().Get("ETag"), `"`)

	completeBody := []byte(`<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>"` + etag1 + `"</ETag></Part><Part><PartNumber>2</PartNumber><ETag>"` + etag2 + `"</ETag></Part></CompleteMultipartUpload>`)
	completeRec := mustS3(t, handler, http.MethodPost, "http://127.0.0.1:4566/mp-lab-bucket/big.bin?uploadId="+uploadID, completeBody, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if completeRec.Code != http.StatusOK {
		t.Fatalf("CompleteMultipartUpload status=%d body=%q", completeRec.Code, completeRec.Body.String())
	}

	getRec := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/mp-lab-bucket/big.bin", nil, "s3", now, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetObject status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	want := append(part1, part2...)
	if getRec.Body.String() != string(want) {
		t.Fatalf("body len=%d want %d", getRec.Body.Len(), len(want))
	}
}

func TestS3MultipartAbort(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/abort-mp-bucket", nil, "s3", now, nil)
	createRec := mustS3(t, handler, http.MethodPost, "http://127.0.0.1:4566/abort-mp-bucket/x.bin?uploads", nil, "s3", now, nil)
	uploadID := parseMultipartUploadID(t, createRec.Body.Bytes())
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/abort-mp-bucket/x.bin?partNumber=1&uploadId="+uploadID, []byte("x"), "s3", now, nil)

	abortRec := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/abort-mp-bucket/x.bin?uploadId="+uploadID, nil, "s3", now, nil)
	if abortRec.Code != http.StatusNoContent {
		t.Fatalf("AbortMultipartUpload status=%d body=%q", abortRec.Code, abortRec.Body.String())
	}
	listRec := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/abort-mp-bucket?uploads", nil, "s3", now, nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListMultipartUploads status=%d body=%q", listRec.Code, listRec.Body.String())
	}
	if strings.Contains(listRec.Body.String(), uploadID) {
		t.Fatalf("upload still listed: %q", listRec.Body.String())
	}
}

func TestS3MultipartListParts(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/parts-bucket", nil, "s3", now, nil)
	createRec := mustS3(t, handler, http.MethodPost, "http://127.0.0.1:4566/parts-bucket/z.bin?uploads", nil, "s3", now, nil)
	uploadID := parseMultipartUploadID(t, createRec.Body.Bytes())
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/parts-bucket/z.bin?partNumber=1&uploadId="+uploadID, bytes.Repeat([]byte("z"), store.MinMultipartPartSize), "s3", now, nil)

	listRec := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/parts-bucket/z.bin?uploadId="+uploadID, nil, "s3", now, nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListParts status=%d body=%q", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), "<PartNumber>1</PartNumber>") {
		t.Fatalf("missing part in %q", listRec.Body.String())
	}
}

func parseMultipartUploadID(t *testing.T, body []byte) string {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "UploadId" {
			var id string
			if err := dec.DecodeElement(&id, &se); err != nil {
				t.Fatal(err)
			}
			return id
		}
	}
	return ""
}
