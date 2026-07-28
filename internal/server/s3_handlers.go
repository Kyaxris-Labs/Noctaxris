package server

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	kmssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/kms"
	s3crypto "github.com/Kyaxris-Labs/Noctaxris/internal/services/s3"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	s3XMLNS                = "http://s3.amazonaws.com/doc/2006-03-01/"
	sseAES256              = "AES256"
	sseAWSKMS              = "aws:kms"
	headerSSESSE           = "x-amz-server-side-encryption"
	headerSSEKMSKeyID      = "x-amz-server-side-encryption-aws-kms-key-id"
	headerSSEKMSContext    = "x-amz-server-side-encryption-context"
	headerCopySource       = "x-amz-copy-source"
	headerMetadataDir      = "x-amz-metadata-directive"
)

// parseS3CannedACLHeader accepts private, public-read, public-read-write (lab subset).
func parseS3CannedACLHeader(raw string) (acl string, ok bool) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return "private", true
	}
	switch raw {
	case "private", "public-read", "public-read-write":
		return raw, true
	default:
		return "", false
	}
}

type s3ErrorXML struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	RequestID string   `xml:"RequestId"`
}

type s3ListAllMyBucketsResult struct {
	XMLName xml.Name      `xml:"ListAllMyBucketsResult"`
	XMLNS   string        `xml:"xmlns,attr"`
	Owner   s3Owner       `xml:"Owner"`
	Buckets s3BucketsWrap `xml:"Buckets"`
}

type s3Owner struct {
	ID          string `xml:"ID"`
	DisplayName string `xml:"DisplayName"`
}

type s3BucketsWrap struct {
	Bucket []s3BucketXML `xml:"Bucket"`
}

type s3BucketXML struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

type s3ListBucketResult struct {
	XMLName        xml.Name            `xml:"ListBucketResult"`
	XMLNS          string              `xml:"xmlns,attr"`
	Name           string              `xml:"Name"`
	Prefix         string              `xml:"Prefix"`
	Delimiter      string              `xml:"Delimiter,omitempty"`
	KeyCount       int                 `xml:"KeyCount"`
	IsTruncated    bool                `xml:"IsTruncated"`
	Contents       []s3ObjectXML       `xml:"Contents"`
	CommonPrefixes []s3CommonPrefixXML `xml:"CommonPrefixes"`
}

type s3ObjectXML struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
}

type s3CommonPrefixXML struct {
	Prefix string `xml:"Prefix"`
}

type s3InitiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	XMLNS    string   `xml:"xmlns,attr"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

type s3CompleteMultipartUploadXML struct {
	XMLName xml.Name            `xml:"CompleteMultipartUpload"`
	Parts   []s3CompletedPartXML `xml:"Part"`
}

type s3CompletedPartXML struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

type s3CompleteMultipartUploadResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	XMLNS    string   `xml:"xmlns,attr"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}

type s3ListPartsResult struct {
	XMLName            xml.Name         `xml:"ListPartsResult"`
	XMLNS              string           `xml:"xmlns,attr"`
	Bucket             string           `xml:"Bucket"`
	Key                string           `xml:"Key"`
	UploadID           string           `xml:"UploadId"`
	PartNumberMarker   int              `xml:"PartNumberMarker"`
	NextPartNumberMarker int            `xml:"NextPartNumberMarker,omitempty"`
	MaxParts           int              `xml:"MaxParts"`
	IsTruncated        bool             `xml:"IsTruncated"`
	Parts              []s3ListedPartXML `xml:"Part"`
}

type s3ListedPartXML struct {
	PartNumber   int    `xml:"PartNumber"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
}

type s3ListMultipartUploadsResult struct {
	XMLName xml.Name                  `xml:"ListMultipartUploadsResult"`
	XMLNS   string                    `xml:"xmlns,attr"`
	Bucket  string                    `xml:"Bucket"`
	Prefix  string                    `xml:"Prefix"`
	Uploads []s3MultipartUploadListXML `xml:"Upload"`
}

type s3MultipartUploadListXML struct {
	Key          string `xml:"Key"`
	UploadID     string `xml:"UploadId"`
	Initiated    string `xml:"Initiated"`
	StorageClass string `xml:"StorageClass"`
}

type s3BucketEncryptionXML struct {
	XMLName xml.Name              `xml:"ServerSideEncryptionConfiguration"`
	XMLNS   string                `xml:"xmlns,attr"`
	Rules   []s3EncryptionRuleXML `xml:"Rule"`
}

type s3EncryptionRuleXML struct {
	Apply s3EncryptionApplyXML `xml:"ApplyServerSideEncryptionByDefault"`
}

type s3EncryptionApplyXML struct {
	SSEAlgorithm string `xml:"SSEAlgorithm"`
	KMSMasterKeyID string `xml:"KMSMasterKeyID,omitempty"`
}

type s3CopyObjectResult struct {
	XMLName      xml.Name `xml:"CopyObjectResult"`
	XMLNS        string   `xml:"xmlns,attr"`
	ETag         string   `xml:"ETag"`
	LastModified string   `xml:"LastModified"`
}

type s3SSECreateMeta struct {
	ContentType       string
	SSEAlgorithm      string
	KMSKeyID          string
	SealedDEK         []byte
	SSEKMSContextJSON string
}

func (s *Server) handleS3(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	bucket, key, ok := parseS3Path(r.URL.Path)
	if !ok {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotImplemented, "NotImplemented",
			"This S3 API is not implemented in Noctaxris.", "Unknown")
		return
	}

	q := r.URL.Query()
	uploadID := q.Get("uploadId")
	switch {
	case r.Method == http.MethodGet && bucket == "" && key == "":
		s.s3ListBuckets(w, r, requestID, eventID, verified, readOnly)
	case r.Method == http.MethodPut && bucket != "" && key == "" && q.Has("notification"):
		s.s3PutBucketNotificationConfiguration(w, r, body, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodGet && bucket != "" && key == "" && q.Has("notification"):
		s.s3GetBucketNotificationConfiguration(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodPut && bucket != "" && key == "" && q.Has("logging"):
		s.s3PutBucketLogging(w, r, body, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodGet && bucket != "" && key == "" && q.Has("logging"):
		s.s3GetBucketLogging(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodPut && bucket != "" && key == "" && q.Has("versioning"):
		s.s3PutBucketVersioning(w, r, body, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodGet && bucket != "" && key == "" && q.Has("versioning"):
		s.s3GetBucketVersioning(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodGet && bucket != "" && key == "" && q.Has("versions"):
		s.s3ListObjectVersions(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodPut && bucket != "" && key == "" && q.Has("encryption"):
		s.s3PutBucketEncryption(w, r, body, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodGet && bucket != "" && key == "" && q.Has("encryption"):
		s.s3GetBucketEncryption(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodDelete && bucket != "" && key == "" && q.Has("encryption"):
		s.s3DeleteBucketEncryption(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodPut && bucket != "" && key == "" && q.Has("policy"):
		s.s3PutBucketPolicy(w, r, body, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodGet && bucket != "" && key == "" && q.Has("policy"):
		s.s3GetBucketPolicy(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodDelete && bucket != "" && key == "" && q.Has("policy"):
		s.s3DeleteBucketPolicy(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodGet && bucket != "" && key == "" && (q.Get("list-type") == "2" || q.Has("list-type")):
		s.s3ListObjectsV2(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodGet && bucket != "" && key == "" && q.Has("uploads"):
		s.s3ListMultipartUploads(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodPut && bucket != "" && key == "":
		s.s3CreateBucket(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodDelete && bucket != "" && key == "":
		s.s3DeleteBucket(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodHead && bucket != "" && key == "":
		s.s3HeadBucket(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodGet && bucket != "" && key == "":
		// Default bucket GET is ListObjectsV2 for lab convenience when list-type omitted.
		s.s3ListObjectsV2(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodPost && bucket != "" && key != "" && q.Has("uploads"):
		s.s3CreateMultipartUpload(w, r, requestID, eventID, verified, readOnly, bucket, key)
	case r.Method == http.MethodPut && bucket != "" && key != "" && uploadID != "" && q.Has("partNumber"):
		s.s3UploadPart(w, r, body, requestID, eventID, verified, readOnly, bucket, key, uploadID)
	case r.Method == http.MethodPost && bucket != "" && key != "" && uploadID != "":
		s.s3CompleteMultipartUpload(w, r, body, requestID, eventID, verified, readOnly, bucket, key, uploadID)
	case r.Method == http.MethodDelete && bucket != "" && key != "" && uploadID != "":
		s.s3AbortMultipartUpload(w, r, requestID, eventID, verified, readOnly, bucket, key, uploadID)
	case r.Method == http.MethodGet && bucket != "" && key != "" && uploadID != "":
		s.s3ListParts(w, r, requestID, eventID, verified, readOnly, bucket, key, uploadID)
	case r.Method == http.MethodPut && bucket != "" && key != "" && strings.TrimSpace(r.Header.Get(headerCopySource)) != "":
		s.s3CopyObject(w, r, requestID, eventID, verified, readOnly, bucket, key)
	case r.Method == http.MethodPut && bucket != "" && key != "":
		s.s3PutObject(w, r, body, requestID, eventID, verified, readOnly, bucket, key)
	case r.Method == http.MethodGet && bucket != "" && key != "":
		s.s3GetObject(w, r, requestID, eventID, verified, readOnly, bucket, key)
	case r.Method == http.MethodDelete && bucket != "" && key != "":
		s.s3DeleteObject(w, r, requestID, eventID, verified, readOnly, bucket, key)
	case r.Method == http.MethodHead && bucket != "" && key != "":
		s.s3HeadObject(w, r, requestID, eventID, verified, readOnly, bucket, key)
	default:
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotImplemented, "NotImplemented",
			"This S3 API is not implemented in Noctaxris.", eventNameForRequest(r))
	}
}

func parseS3Path(path string) (bucket, key string, ok bool) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return "", "", true
	}
	parts := strings.SplitN(path, "/", 2)
	bucket = parts[0]
	if len(parts) == 2 {
		key = parts[1]
	}
	return bucket, key, true
}

func (s *Server) authorizeS3(verified *authn.Verified, action, resource, bucketPolicy, resourceAccountID string) bool {
	return s.authorizeDataplaneOR(verified, action, resource, resourceAccountID, func(caller authz.RequestContext, identityDocs []string, resourceAccountID string) authz.Decision {
		return authz.EvaluateS3(authz.S3Request{
			Caller:            caller,
			IdentityDocs:      identityDocs,
			BucketPolicyDoc:   bucketPolicy,
			ResourceAccountID: resourceAccountID,
		})
	})
}

type s3BucketRef struct {
	accountID string
	name      string
	policy    string
}

func (s *Server) s3ResolveBucket(name string) (s3BucketRef, error) {
	b, err := s.store.GetBucketByName(name)
	if err != nil {
		return s3BucketRef{}, err
	}
	return s3BucketRef{accountID: b.AccountID, name: b.Name, policy: b.BucketPolicy}, nil
}

func (s *Server) s3RequireBucket(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	bucket, opName string,
) (s3BucketRef, bool) {
	ref, err := s.s3ResolveBucket(bucket)
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", opName)
		return s3BucketRef{}, false
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", opName)
		return s3BucketRef{}, false
	}
	return ref, true
}

func (s *Server) s3ListBuckets(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool) {
	if !s.authorizeS3(verified, catalog.ActionS3ListAllMyBuckets, "*", "", "") {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "ListBuckets")
		return
	}
	buckets, err := s.store.ListBuckets(verified.AccountID)
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "ListBuckets")
		return
	}
	out := s3ListAllMyBucketsResult{
		XMLNS: s3XMLNS,
		Owner: s3Owner{ID: verified.AccountID, DisplayName: verified.AccountID},
	}
	for _, b := range buckets {
		out.Buckets.Bucket = append(out.Buckets.Bucket, s3BucketXML{Name: b.Name, CreationDate: b.CreationDate})
	}
	s.writeS3XML(w, requestID, http.StatusOK, out)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "ListBuckets", readOnly)
}

func (s *Server) s3CreateBucket(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	if !s.authorizeS3(verified, catalog.ActionS3CreateBucket, store.BucketARN(bucket), "", verified.AccountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "CreateBucket")
		return
	}
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}
	objectLock := createBucketObjectLockEnabled(r, body)
	_, err := s.store.CreateBucketWithOptions(verified.AccountID, bucket, store.CreateBucketOptions{
		ObjectLockEnabled: objectLock,
	})
	if errors.Is(err, store.ErrInvalidBucketName) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidBucketName",
			"The specified bucket is not valid.", "CreateBucket")
		return
	}
	if errors.Is(err, store.ErrBucketAlreadyExists) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusConflict, "BucketAlreadyExists",
			"The requested bucket name is not available.", "CreateBucket")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "CreateBucket")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Location", "/"+bucket)
	w.WriteHeader(http.StatusOK)
	_ = s.store.AppendS3BucketConfigHistory(verified.AccountID, bucket, false)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "CreateBucket", readOnly,
		WithAuditResources([]audit.Resource{{
			AccountID: verified.AccountID,
			Type:      "AWS::S3::Bucket",
			ARN:       store.BucketARN(bucket),
		}}),
		WithAuditRequestParameters(map[string]any{"bucketName": bucket}),
	)
}

func (s *Server) s3DeleteBucket(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, err := s.s3ResolveBucket(bucket)
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "DeleteBucket")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "DeleteBucket")
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3DeleteBucket, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "DeleteBucket")
		return
	}
	err = s.store.DeleteBucket(ref.accountID, bucket)
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "DeleteBucket")
		return
	}
	if errors.Is(err, store.ErrBucketNotEmpty) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusConflict, "BucketNotEmpty",
			"The bucket you tried to delete is not empty", "DeleteBucket")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "DeleteBucket")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusNoContent)
	_ = s.store.AppendS3BucketConfigHistory(ref.accountID, bucket, true)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "DeleteBucket", readOnly)
}

func (s *Server) s3HeadBucket(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, err := s.s3ResolveBucket(bucket)
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "HeadBucket")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "HeadBucket")
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3ListBucket, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "HeadBucket")
		return
	}
	if err := s.store.HeadBucket(ref.accountID, bucket); err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "HeadBucket")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "HeadBucket", readOnly)
}

func (s *Server) s3PutBucketPolicy(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, err := s.s3ResolveBucket(bucket)
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "PutBucketPolicy")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "PutBucketPolicy")
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3PutBucketPolicy, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "PutBucketPolicy")
		return
	}
	if err := authz.ValidateResourcePolicyDocument(string(body)); err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "MalformedPolicy",
			err.Error(), "PutBucketPolicy")
		return
	}
	if err := s.store.PutBucketPolicy(ref.accountID, bucket, string(body)); err != nil {
		if errors.Is(err, store.ErrNoSuchBucket) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
				"The specified bucket does not exist", "PutBucketPolicy")
			return
		}
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "PutBucketPolicy")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "PutBucketPolicy", readOnly)
}

func (s *Server) s3GetBucketPolicy(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "GetBucketPolicy")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3GetBucketPolicy, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "GetBucketPolicy")
		return
	}
	doc, err := s.store.GetBucketPolicy(ref.accountID, bucket)
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "GetBucketPolicy")
		return
	}
	if errors.Is(err, store.ErrNoSuchBucketPolicy) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucketPolicy",
			"The bucket policy does not exist", "GetBucketPolicy")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "GetBucketPolicy")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(doc))
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "GetBucketPolicy", readOnly)
}

func (s *Server) s3DeleteBucketPolicy(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "DeleteBucketPolicy")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3DeleteBucketPolicy, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "DeleteBucketPolicy")
		return
	}
	if err := s.store.DeleteBucketPolicy(ref.accountID, bucket); err != nil {
		if errors.Is(err, store.ErrNoSuchBucket) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
				"The specified bucket does not exist", "DeleteBucketPolicy")
			return
		}
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "DeleteBucketPolicy")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusNoContent)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "DeleteBucketPolicy", readOnly)
}

func (s *Server) s3PutBucketEncryption(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "PutBucketEncryption")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3PutEncryptionConfiguration, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "PutBucketEncryption")
		return
	}
	var req s3BucketEncryptionXML
	if err := xml.Unmarshal(body, &req); err != nil || len(req.Rules) == 0 {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "MalformedXML",
			"The XML you provided was not well-formed or did not validate against our published schema.", "PutBucketEncryption")
		return
	}
	apply := req.Rules[0].Apply
	enc := store.BucketEncryption{
		Algorithm: apply.SSEAlgorithm,
		KMSKeyID:  apply.KMSMasterKeyID,
	}
	if strings.EqualFold(enc.Algorithm, sseAWSKMS) && enc.KMSKeyID != "" {
		keyID, err := s.store.ResolveKeyID(verified.AccountID, enc.KMSKeyID)
		if err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				"KMS key not found", "PutBucketEncryption")
			return
		}
		kmsKey, err := s.store.GetKey(keyID)
		if err != nil || kmsKey.AccountID != verified.AccountID {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				"KMS key not found", "PutBucketEncryption")
			return
		}
		enc.KMSKeyID = kmsKey.ARN
	}
	if err := s.store.PutBucketEncryption(ref.accountID, bucket, enc); err != nil {
		switch {
		case errors.Is(err, store.ErrNoSuchBucket):
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
				"The specified bucket does not exist", "PutBucketEncryption")
		case errors.Is(err, store.ErrInvalidBucketEncryption):
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				"Invalid server-side encryption configuration", "PutBucketEncryption")
		default:
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", "PutBucketEncryption")
		}
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "PutBucketEncryption", readOnly)
}

func (s *Server) s3GetBucketEncryption(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "GetBucketEncryption")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3GetEncryptionConfiguration, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "GetBucketEncryption")
		return
	}
	enc, err := s.store.GetBucketEncryption(ref.accountID, bucket)
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "GetBucketEncryption")
		return
	}
	if errors.Is(err, store.ErrNoSuchBucketEncryption) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "ServerSideEncryptionConfigurationNotFoundError",
			"The server side encryption configuration was not found", "GetBucketEncryption")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "GetBucketEncryption")
		return
	}
	out := s3BucketEncryptionXML{
		XMLNS: s3XMLNS,
		Rules: []s3EncryptionRuleXML{{
			Apply: s3EncryptionApplyXML{
				SSEAlgorithm:   enc.Algorithm,
				KMSMasterKeyID: enc.KMSKeyID,
			},
		}},
	}
	s.writeS3XML(w, requestID, http.StatusOK, out)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "GetBucketEncryption", readOnly)
}

func (s *Server) s3DeleteBucketEncryption(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "DeleteBucketEncryption")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3DeleteEncryptionConfiguration, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "DeleteBucketEncryption")
		return
	}
	if err := s.store.DeleteBucketEncryption(ref.accountID, bucket); err != nil {
		if errors.Is(err, store.ErrNoSuchBucket) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
				"The specified bucket does not exist", "DeleteBucketEncryption")
			return
		}
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "DeleteBucketEncryption")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusNoContent)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "DeleteBucketEncryption", readOnly)
}

func (s *Server) s3ListObjectsV2(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "ListObjectsV2")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3ListBucket, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "ListObjectsV2")
		return
	}
	prefix := r.URL.Query().Get("prefix")
	delimiter := r.URL.Query().Get("delimiter")
	result, err := s.store.ListObjectsV2(ref.accountID, bucket, prefix, delimiter)
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "ListObjectsV2")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "ListObjectsV2")
		return
	}
	out := s3ListBucketResult{
		XMLNS:       s3XMLNS,
		Name:        bucket,
		Prefix:      prefix,
		Delimiter:   delimiter,
		KeyCount:    result.KeyCount,
		IsTruncated: result.IsTruncated,
	}
	for _, o := range result.Contents {
		out.Contents = append(out.Contents, s3ObjectXML{
			Key:          o.Key,
			LastModified: s3XMLLastModified(o.LastModified),
			ETag:         `"` + o.ETag + `"`,
			Size:         o.Size,
		})
	}
	for _, cp := range result.CommonPrefixes {
		out.CommonPrefixes = append(out.CommonPrefixes, s3CommonPrefixXML{Prefix: cp})
	}
	s.writeS3XML(w, requestID, http.StatusOK, out)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "ListObjectsV2", readOnly)
}

func (s *Server) s3PutObject(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket, key string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "PutObject")
	if !ok {
		return
	}
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3PutObject, resource, ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "PutObject")
		return
	}

	sse, kmsKeyParam := s.s3EffectiveSSE(r, ref.accountID, bucket)
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	cannedACL, aclOK := parseS3CannedACLHeader(r.Header.Get("x-amz-acl"))
	if !aclOK {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
			"Unsupported x-amz-acl value", "PutObject")
		return
	}

	plainSum := md5.Sum(body)
	etag := hex.EncodeToString(plainSum[:])
	meta := store.PutObjectMeta{
		ContentType: contentType,
		CannedACL:   cannedACL,
		Data:        body,
		PlainSize:   int64(len(body)),
		ETag:        etag,
	}
	s3ObjectLockFromRequest(r, &meta)
	if ok := s.s3EncryptPutMeta(w, r, requestID, eventID, verified, readOnly, &meta, sse, kmsKeyParam, resource, "PutObject"); !ok {
		return
	}

	obj, versionID, err := s.store.PutObjectVersioned(ref.accountID, bucket, key, meta)
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "PutObject")
		return
	}
	if errors.Is(err, store.ErrInvalidObjectKey) || errors.Is(err, store.ErrInvalidBucketName) || errors.Is(err, store.ErrObjectLockRequired) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
			err.Error(), "PutObject")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "PutObject")
		return
	}

	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("ETag", `"`+obj.ETag+`"`)
	if versionID != "" {
		w.Header().Set("x-amz-version-id", versionID)
	}
	if obj.SSEAlgorithm != "" {
		w.Header().Set(headerSSESSE, obj.SSEAlgorithm)
	}
	if obj.KMSKeyID != "" {
		w.Header().Set(headerSSEKMSKeyID, obj.KMSKeyID)
	}
	w.WriteHeader(http.StatusOK)
	_ = s.store.AppendS3ObjectConfigHistory(ref.accountID, bucket, key, false)
	s.emitS3ServerAccessLog(ref.accountID, bucket, "REST.PUT.OBJECT", key, requestID, r, verified, http.StatusOK, int64(len(body)), obj.Size)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "PutObject", readOnly,
		WithAuditResources([]audit.Resource{
			{AccountID: verified.AccountID, Type: "AWS::S3::Bucket", ARN: store.BucketARN(bucket)},
			{AccountID: verified.AccountID, Type: "AWS::S3::Object", ARN: store.ObjectARN(bucket, key)},
		}),
		WithAuditRequestParameters(map[string]any{
			"bucketName": bucket,
			"key":        key,
		}),
	)
}

func (s *Server) s3CopyObject(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, destBucket, destKey string) {
	srcBucket, srcKey, err := parseCopySource(r.Header.Get(headerCopySource))
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
			"Invalid copy source", "CopyObject")
		return
	}
	destRef, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, destBucket, "CopyObject")
	if !ok {
		return
	}
	srcRef, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, srcBucket, "CopyObject")
	if !ok {
		return
	}
	if srcRef.accountID != destRef.accountID {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidRequest",
			"CopyObject is same-account only in this lab", "CopyObject")
		return
	}
	srcResource := store.ObjectARN(srcBucket, srcKey)
	destResource := store.ObjectARN(destBucket, strings.TrimPrefix(destKey, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3GetObject, srcResource, srcRef.policy, srcRef.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "CopyObject")
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3PutObject, destResource, destRef.policy, destRef.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "CopyObject")
		return
	}
	srcMeta, srcData, err := s.store.GetObject(srcRef.accountID, srcBucket, srcKey)
	if errors.Is(err, store.ErrNoSuchKey) || errors.Is(err, store.ErrInvalidObjectKey) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchKey",
			"The specified key does not exist.", "CopyObject")
		return
	}
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "CopyObject")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "CopyObject")
		return
	}
	plain, err := s.decryptObjectPayload(verified, srcMeta, srcData)
	if err != nil {
		code, msg := "InternalError", "Internal error"
		status := http.StatusInternalServerError
		if errors.Is(err, errS3AccessDenied) {
			code, msg, status = "AccessDenied", "Access Denied", http.StatusForbidden
		}
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, status, code, msg, "CopyObject")
		return
	}
	contentType := srcMeta.ContentType
	if strings.EqualFold(strings.TrimSpace(r.Header.Get(headerMetadataDir)), "REPLACE") {
		contentType = r.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}
	plainSum := md5.Sum(plain)
	destMeta := store.PutObjectMeta{
		ContentType: contentType,
		Data:        plain,
		PlainSize:   int64(len(plain)),
		ETag:        hex.EncodeToString(plainSum[:]),
	}
	sse, kmsKeyParam := s.s3EffectiveSSE(r, destRef.accountID, destBucket)
	if ok := s.s3EncryptPutMeta(w, r, requestID, eventID, verified, readOnly, &destMeta, sse, kmsKeyParam, destResource, "CopyObject"); !ok {
		return
	}
	obj, err := s.store.CopyObject(destRef.accountID, srcBucket, srcKey, destBucket, destKey, destMeta)
	if mapErr := s.mapS3StoreError(w, r, requestID, eventID, verified, readOnly, err, "CopyObject"); mapErr {
		return
	}
	out := s3CopyObjectResult{
		XMLNS:        s3XMLNS,
		ETag:         `"` + obj.ETag + `"`,
		LastModified: s3XMLLastModified(obj.LastModified),
	}
	s.writeS3XML(w, requestID, http.StatusOK, out)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "CopyObject", readOnly)
}

func parseCopySource(header string) (bucket, key string, err error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", "", errors.New("empty copy source")
	}
	if i := strings.Index(header, "?"); i >= 0 {
		header = header[:i]
	}
	header = strings.TrimPrefix(header, "/")
	parts := strings.SplitN(header, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid copy source")
	}
	bucket, err = url.PathUnescape(parts[0])
	if err != nil {
		return "", "", err
	}
	key, err = url.PathUnescape(parts[1])
	if err != nil {
		return "", "", err
	}
	return bucket, key, nil
}

func (s *Server) s3GetObject(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket, key string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "GetObject")
	if !ok {
		return
	}
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	versionID := r.URL.Query().Get("versionId")
	if verified != nil && verified.Principal.Kind == identity.KindAnonymous {
		objectACL := ""
		if head, herr := s.store.HeadObject(ref.accountID, bucket, key); herr == nil {
			objectACL = head.CannedACL
		}
		if !s.authorizeAnonymousS3Get(verified, resource, ref.policy, ref.accountID, objectACL) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
				"Access Denied", "GetObject")
			return
		}
	} else if !s.authorizeS3(verified, catalog.ActionS3GetObject, resource, ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "GetObject")
		return
	}
	meta, vid, data, err := s.store.GetObjectVersion(ref.accountID, bucket, key, versionID)
	if errors.Is(err, store.ErrNoSuchKey) || errors.Is(err, store.ErrInvalidObjectKey) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchKey",
			"The specified key does not exist.", "GetObject")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "GetObject")
		return
	}
	plain, err := s.decryptObjectPayload(verified, meta, data)
	if err != nil {
		code, msg := "InternalError", "Internal error"
		status := http.StatusInternalServerError
		if errors.Is(err, errS3AccessDenied) {
			code, msg, status = "AccessDenied", "Access Denied", http.StatusForbidden
		}
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, status, code, msg, "GetObject")
		return
	}
	if meta.SSEAlgorithm == sseAWSKMS && meta.KMSKeyID != "" {
		keyID, rerr := s.store.ResolveKeyID(verified.AccountID, meta.KMSKeyID)
		if rerr == nil {
			if kmsKey, gerr := s.store.GetKey(keyID); gerr == nil {
				objectARN := store.ObjectARN(meta.Bucket, meta.Key)
				customerCtx, _ := parseStoredS3CustomerEncryptionContext(meta.SSEKMSContextJSON)
				encCtx := s3SSEKMSEncryptionContext(objectARN, customerCtx)
				s.writeSiblingKMSDecryptAudit(r, requestID, eventID, verified, kmsKey.ARN, encCtx)
			}
		}
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("ETag", `"`+meta.ETag+`"`)
	if vid != "" {
		w.Header().Set("x-amz-version-id", vid)
	}
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(plain)), 10))
	w.Header().Set("Last-Modified", s3HTTPLastModified(meta.LastModified))
	if meta.SSEAlgorithm != "" {
		w.Header().Set(headerSSESSE, meta.SSEAlgorithm)
	}
	if meta.KMSKeyID != "" {
		w.Header().Set(headerSSEKMSKeyID, meta.KMSKeyID)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(plain)
	s.emitS3ServerAccessLog(ref.accountID, bucket, "REST.GET.OBJECT", key, requestID, r, verified, http.StatusOK, int64(len(plain)), meta.Size)
	auditVid := versionID
	if auditVid == "" {
		auditVid = vid
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "GetObject", readOnly,
		s.s3ObjectDataAuditOpts(verified, bucket, key, auditVid)...,
	)
}

func (s *Server) s3HeadObject(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket, key string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "HeadObject")
	if !ok {
		return
	}
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	meta, err := s.store.HeadObject(ref.accountID, bucket, key)
	if verified != nil && verified.Principal.Kind == identity.KindAnonymous {
		objectACL := ""
		if err == nil {
			objectACL = meta.CannedACL
		}
		if !s.authorizeAnonymousS3Get(verified, resource, ref.policy, ref.accountID, objectACL) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
				"Access Denied", "HeadObject")
			return
		}
	} else if !s.authorizeS3(verified, catalog.ActionS3GetObject, resource, ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "HeadObject")
		return
	}
	if errors.Is(err, store.ErrNoSuchKey) || errors.Is(err, store.ErrInvalidObjectKey) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchKey",
			"The specified key does not exist.", "HeadObject")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "HeadObject")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("ETag", `"`+meta.ETag+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	w.Header().Set("Last-Modified", s3HTTPLastModified(meta.LastModified))
	if meta.SSEAlgorithm != "" {
		w.Header().Set(headerSSESSE, meta.SSEAlgorithm)
	}
	if meta.KMSKeyID != "" {
		w.Header().Set(headerSSEKMSKeyID, meta.KMSKeyID)
	}
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "HeadObject", readOnly)
}

func (s *Server) s3DeleteObject(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket, key string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "DeleteObject")
	if !ok {
		return
	}
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3DeleteObject, resource, ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "DeleteObject")
		return
	}
	versionID := r.URL.Query().Get("versionId")
	delOpts := store.DeleteObjectOptions{BypassGovernanceRetention: s3BypassGovernanceRetention(r)}
	result, err := s.store.DeleteObjectVersionedWithOptions(ref.accountID, bucket, key, versionID, delOpts)
	if errors.Is(err, store.ErrObjectLockRetention) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Object is WORM protected", "DeleteObject")
		return
	}
	if errors.Is(err, store.ErrNoSuchKey) || errors.Is(err, store.ErrInvalidObjectKey) {
		w.Header().Set(requestIDHeader, requestID)
		w.WriteHeader(http.StatusNoContent)
		s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "DeleteObject", readOnly,
			s.s3ObjectDataAuditOpts(verified, bucket, key, versionID)...,
		)
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "DeleteObject")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	if result.DeleteMarker {
		w.Header().Set("x-amz-delete-marker", "true")
	}
	if result.VersionID != "" {
		w.Header().Set("x-amz-version-id", result.VersionID)
	}
	w.WriteHeader(http.StatusNoContent)
	_ = s.store.AppendS3ObjectConfigHistory(ref.accountID, bucket, key, true)
	s.emitS3ServerAccessLog(ref.accountID, bucket, "REST.DELETE.OBJECT", key, requestID, r, verified, http.StatusNoContent, 0, 0)
	auditVid := versionID
	if auditVid == "" && result.VersionID != "" {
		auditVid = result.VersionID
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "DeleteObject", readOnly,
		s.s3ObjectDataAuditOpts(verified, bucket, key, auditVid)...,
	)
}

func (s *Server) s3ObjectDataAuditOpts(verified *authn.Verified, bucket, key, versionID string) []SuccessAuditOption {
	accountID := ""
	if verified != nil {
		accountID = verified.AccountID
	}
	params := map[string]any{
		"bucketName": bucket,
		"key":        strings.TrimPrefix(key, "/"),
	}
	if versionID != "" {
		params["versionId"] = versionID
	}
	return []SuccessAuditOption{
		WithAuditResources([]audit.Resource{
			{AccountID: accountID, Type: "AWS::S3::Bucket", ARN: store.BucketARN(bucket)},
			{AccountID: accountID, Type: "AWS::S3::Object", ARN: store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))},
		}),
		WithAuditRequestParameters(params),
	}
}

func (s *Server) s3CreateMultipartUpload(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket, key string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "CreateMultipartUpload")
	if !ok {
		return
	}
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3CreateMultipartUpload, resource, ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "CreateMultipartUpload")
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	sseMeta, ok := s.s3BuildSSECreateMeta(w, r, requestID, eventID, verified, readOnly, bucket, resource, "CreateMultipartUpload")
	if !ok {
		return
	}
	sseMeta.ContentType = contentType
	upload, err := s.store.CreateMultipartUpload(ref.accountID, bucket, key, store.CreateMultipartUploadMeta{
		ContentType:       sseMeta.ContentType,
		SSEAlgorithm:      sseMeta.SSEAlgorithm,
		KMSKeyID:          sseMeta.KMSKeyID,
		SealedDEK:         sseMeta.SealedDEK,
		SSEKMSContextJSON: sseMeta.SSEKMSContextJSON,
	})
	if mapErr := s.mapS3StoreError(w, r, requestID, eventID, verified, readOnly, err, "CreateMultipartUpload"); mapErr {
		return
	}
	if upload.SSEAlgorithm != "" {
		w.Header().Set(headerSSESSE, upload.SSEAlgorithm)
	}
	if upload.KMSKeyID != "" {
		w.Header().Set(headerSSEKMSKeyID, upload.KMSKeyID)
	}
	out := s3InitiateMultipartUploadResult{
		XMLNS:    s3XMLNS,
		Bucket:   upload.Bucket,
		Key:      upload.Key,
		UploadID: upload.UploadID,
	}
	s.writeS3XML(w, requestID, http.StatusOK, out)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "CreateMultipartUpload", readOnly)
}

func (s *Server) s3UploadPart(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket, key, uploadID string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "UploadPart")
	if !ok {
		return
	}
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3UploadPart, resource, ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "UploadPart")
		return
	}
	partNumber, err := strconv.Atoi(r.URL.Query().Get("partNumber"))
	if err != nil || partNumber < 1 {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
			"Invalid part number", "UploadPart")
		return
	}
	if _, err := s.store.GetMultipartUpload(ref.accountID, bucket, key, uploadID); errors.Is(err, store.ErrNoSuchUpload) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchUpload",
			"The specified multipart upload does not exist.", "UploadPart")
		return
	} else if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "UploadPart")
		return
	}
	part, err := s.store.UploadPart(ref.accountID, bucket, key, uploadID, partNumber, store.UploadPartMeta{Data: body})
	if mapErr := s.mapS3StoreError(w, r, requestID, eventID, verified, readOnly, err, "UploadPart"); mapErr {
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("ETag", `"`+part.ETag+`"`)
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "UploadPart", readOnly)
}

func (s *Server) s3CompleteMultipartUpload(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket, key, uploadID string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "CompleteMultipartUpload")
	if !ok {
		return
	}
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3CompleteMultipartUpload, resource, ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "CompleteMultipartUpload")
		return
	}
	var req s3CompleteMultipartUploadXML
	if len(body) > 0 {
		if err := xml.Unmarshal(body, &req); err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "MalformedXML",
				"The XML you provided was not well-formed or did not validate against our published schema.", "CompleteMultipartUpload")
			return
		}
	}
	completed := make([]store.CompletedPartInput, 0, len(req.Parts))
	for _, p := range req.Parts {
		completed = append(completed, store.CompletedPartInput{PartNumber: p.PartNumber, ETag: p.ETag})
	}
	result, err := s.store.CompleteMultipartUpload(ref.accountID, bucket, key, uploadID, completed)
	if errors.Is(err, store.ErrNoSuchUpload) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchUpload",
			"The specified multipart upload does not exist.", "CompleteMultipartUpload")
		return
	}
	if errors.Is(err, store.ErrInvalidPart) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidPart",
			"One or more of the specified parts could not be found.", "CompleteMultipartUpload")
		return
	}
	if errors.Is(err, store.ErrEntityTooSmall) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "EntityTooSmall",
			"Your proposed upload is smaller than the minimum allowed size", "CompleteMultipartUpload")
		return
	}
	if mapErr := s.mapS3StoreError(w, r, requestID, eventID, verified, readOnly, err, "CompleteMultipartUpload"); mapErr {
		return
	}
	putMeta := store.PutObjectMeta{
		ContentType:           result.Upload.ContentType,
		Data:                  result.Data,
		PlainSize:             int64(len(result.Data)),
		ETag:                  result.ETag,
		SSEAlgorithm:          result.Upload.SSEAlgorithm,
		KMSKeyID:              result.Upload.KMSKeyID,
		SealedDEK:             result.Upload.SealedDEK,
		SSEKMSContextJSON:     result.Upload.SSEKMSContextJSON,
		NotificationEventName: "ObjectCreated:CompleteMultipartUpload",
	}
	if putMeta.SSEAlgorithm != "" {
		encrypted, encErr := s.s3EncryptObjectPayload(verified, putMeta.SSEAlgorithm, putMeta.KMSKeyID, resource, putMeta.SSEKMSContextJSON, putMeta.SealedDEK, result.Data)
		if encErr != nil {
			code, msg := "InternalError", "Internal error"
			status := http.StatusInternalServerError
			if errors.Is(encErr, errS3AccessDenied) {
				code, msg, status = "AccessDenied", "Access Denied", http.StatusForbidden
			}
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, status, code, msg, "CompleteMultipartUpload")
			return
		}
		putMeta.Data = encrypted
	}
	obj, versionID, err := s.store.PutObjectVersioned(ref.accountID, bucket, key, putMeta)
	if mapErr := s.mapS3StoreError(w, r, requestID, eventID, verified, readOnly, err, "CompleteMultipartUpload"); mapErr {
		return
	}
	if err := s.store.CleanupMultipartUpload(uploadID); err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "CompleteMultipartUpload")
		return
	}
	out := s3CompleteMultipartUploadResult{
		XMLNS:    s3XMLNS,
		Location: fmt.Sprintf("/%s/%s", bucket, obj.Key),
		Bucket:   obj.Bucket,
		Key:      obj.Key,
		ETag:     `"` + obj.ETag + `"`,
	}
	if versionID != "" {
		w.Header().Set("x-amz-version-id", versionID)
	}
	s.writeS3XML(w, requestID, http.StatusOK, out)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "CompleteMultipartUpload", readOnly)
}

func (s *Server) s3AbortMultipartUpload(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket, key, uploadID string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "AbortMultipartUpload")
	if !ok {
		return
	}
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3AbortMultipartUpload, resource, ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "AbortMultipartUpload")
		return
	}
	err := s.store.AbortMultipartUpload(ref.accountID, bucket, key, uploadID)
	if errors.Is(err, store.ErrNoSuchUpload) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchUpload",
			"The specified multipart upload does not exist.", "AbortMultipartUpload")
		return
	}
	if mapErr := s.mapS3StoreError(w, r, requestID, eventID, verified, readOnly, err, "AbortMultipartUpload"); mapErr {
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusNoContent)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "AbortMultipartUpload", readOnly)
}

func (s *Server) s3ListParts(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket, key, uploadID string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "ListParts")
	if !ok {
		return
	}
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3ListParts, resource, ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "ListParts")
		return
	}
	upload, err := s.store.GetMultipartUpload(ref.accountID, bucket, key, uploadID)
	if errors.Is(err, store.ErrNoSuchUpload) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchUpload",
			"The specified multipart upload does not exist.", "ListParts")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "ListParts")
		return
	}
	marker, _ := strconv.Atoi(r.URL.Query().Get("part-number-marker"))
	maxParts, _ := strconv.Atoi(r.URL.Query().Get("max-parts"))
	parts, truncated, err := s.store.ListParts(ref.accountID, bucket, key, uploadID, marker, maxParts)
	if mapErr := s.mapS3StoreError(w, r, requestID, eventID, verified, readOnly, err, "ListParts"); mapErr {
		return
	}
	out := s3ListPartsResult{
		XMLNS:              s3XMLNS,
		Bucket:             bucket,
		Key:                upload.Key,
		UploadID:           uploadID,
		PartNumberMarker:   marker,
		MaxParts:           maxParts,
		IsTruncated:        truncated,
	}
	if truncated && len(parts) > 0 {
		out.NextPartNumberMarker = parts[len(parts)-1].PartNumber
	}
	if out.MaxParts == 0 {
		out.MaxParts = 1000
	}
	for _, p := range parts {
		out.Parts = append(out.Parts, s3ListedPartXML{
			PartNumber:   p.PartNumber,
			LastModified: s3XMLLastModified(upload.Initiated),
			ETag:         `"` + p.ETag + `"`,
			Size:         p.Size,
		})
	}
	s.writeS3XML(w, requestID, http.StatusOK, out)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "ListParts", readOnly)
}

func (s *Server) s3ListMultipartUploads(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "ListMultipartUploads")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3ListMultipartUploads, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "ListMultipartUploads")
		return
	}
	prefix := r.URL.Query().Get("prefix")
	uploads, err := s.store.ListMultipartUploads(ref.accountID, bucket, prefix)
	if mapErr := s.mapS3StoreError(w, r, requestID, eventID, verified, readOnly, err, "ListMultipartUploads"); mapErr {
		return
	}
	out := s3ListMultipartUploadsResult{
		XMLNS:  s3XMLNS,
		Bucket: bucket,
		Prefix: prefix,
	}
	for _, u := range uploads {
		out.Uploads = append(out.Uploads, s3MultipartUploadListXML{
			Key:          u.Key,
			UploadID:     u.UploadID,
			Initiated:    s3XMLLastModified(u.Initiated),
			StorageClass: "STANDARD",
		})
	}
	s.writeS3XML(w, requestID, http.StatusOK, out)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "ListMultipartUploads", readOnly)
}

func (s *Server) s3BuildSSECreateMeta(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	bucket, objectARN, eventName string,
) (s3SSECreateMeta, bool) {
	sse, kmsKeyParam := s.s3EffectiveSSE(r, verified.AccountID, bucket)
	prepared, ok := s.s3PrepareSSE(w, r, requestID, eventID, verified, readOnly, sse, kmsKeyParam, objectARN, eventName)
	if !ok {
		return s3SSECreateMeta{}, false
	}
	return s3SSECreateMeta{
		SSEAlgorithm:      prepared.Algorithm,
		KMSKeyID:          prepared.KMSKeyID,
		SealedDEK:         prepared.SealedDEK,
		SSEKMSContextJSON: prepared.SSEKMSContextJSON,
	}, true
}

func (s *Server) s3EffectiveSSE(r *http.Request, accountID, bucket string) (sse, kmsKeyParam string) {
	sse = strings.TrimSpace(r.Header.Get(headerSSESSE))
	kmsKeyParam = strings.TrimSpace(r.Header.Get(headerSSEKMSKeyID))
	if sse != "" {
		return sse, kmsKeyParam
	}
	enc, err := s.store.GetBucketEncryption(accountID, bucket)
	if err == nil {
		return enc.Algorithm, enc.KMSKeyID
	}
	return "", ""
}

type s3SSEPrepared struct {
	Algorithm         string
	KMSKeyID          string
	SealedDEK         []byte
	SSEKMSContextJSON string
	dek               []byte
}

func (s *Server) s3PrepareSSE(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	sse, kmsKeyParam, objectARN, eventName string,
) (s3SSEPrepared, bool) {
	switch strings.ToUpper(sse) {
	case "":
		return s3SSEPrepared{}, true
	case sseAES256:
		dek := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, dek); err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", eventName)
			return s3SSEPrepared{}, false
		}
		sealedDEK, err := s.store.SealWithMaster(dek)
		if err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", eventName)
			return s3SSEPrepared{}, false
		}
		return s3SSEPrepared{Algorithm: sseAES256, SealedDEK: sealedDEK, dek: dek}, true
	case "AWS:KMS":
		if kmsKeyParam == "" {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				"x-amz-server-side-encryption-aws-kms-key-id is required for aws:kms", eventName)
			return s3SSEPrepared{}, false
		}
		customerCtx, ctxErr := parseS3EncryptionContextHeader(r.Header.Get(headerSSEKMSContext))
		if ctxErr != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				"x-amz-server-side-encryption-context is invalid", eventName)
			return s3SSEPrepared{}, false
		}
		encCtx := s3SSEKMSEncryptionContext(objectARN, customerCtx)
		keyID, err := s.store.ResolveKeyID(verified.AccountID, kmsKeyParam)
		if err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				"KMS key not found", eventName)
			return s3SSEPrepared{}, false
		}
		kmsKey, err := s.store.GetKey(keyID)
		if err != nil || kmsKey.AccountID != verified.AccountID {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				"KMS key not found", eventName)
			return s3SSEPrepared{}, false
		}
		if !s.authorizeKMSOp(verified, catalog.ActionKMSGenerateDataKey, kmsKey, encCtx) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
				"Access Denied", eventName)
			return s3SSEPrepared{}, false
		}
		if kmsKey.KeyState != store.KeyStateEnabled {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				"KMS key is disabled", eventName)
			return s3SSEPrepared{}, false
		}
		cmk, err := s.store.UnsealKeyMaterial(keyID)
		if err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", eventName)
			return s3SSEPrepared{}, false
		}
		dek := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, dek); err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", eventName)
			return s3SSEPrepared{}, false
		}
		sealedDEK, err := kmssvc.EncryptUnderCMK(cmk, keyID, dek, encCtx)
		if err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", eventName)
			return s3SSEPrepared{}, false
		}
		ctxJSON, jsonErr := encodeS3CustomerEncryptionContext(customerCtx)
		if jsonErr != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", eventName)
			return s3SSEPrepared{}, false
		}
		return s3SSEPrepared{
			Algorithm: sseAWSKMS, KMSKeyID: kmsKey.ARN, SealedDEK: sealedDEK,
			SSEKMSContextJSON: ctxJSON, dek: dek,
		}, true
	default:
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
			"Unsupported server-side encryption", eventName)
		return s3SSEPrepared{}, false
	}
}

func parseS3EncryptionContextHeader(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(decoded, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func encodeS3CustomerEncryptionContext(customerCtx map[string]string) (string, error) {
	if len(customerCtx) == 0 {
		return "", nil
	}
	b, err := json.Marshal(customerCtx)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parseStoredS3CustomerEncryptionContext(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Server) s3EncryptPutMeta(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	meta *store.PutObjectMeta,
	sse, kmsKeyParam, objectARN, eventName string,
) bool {
	prepared, ok := s.s3PrepareSSE(w, r, requestID, eventID, verified, readOnly, sse, kmsKeyParam, objectARN, eventName)
	if !ok {
		return false
	}
	if prepared.Algorithm == "" {
		return true
	}
	ct, err := s3crypto.EncryptAES256GCM(prepared.dek, meta.Data)
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", eventName)
		return false
	}
	meta.SSEAlgorithm = prepared.Algorithm
	meta.KMSKeyID = prepared.KMSKeyID
	meta.SealedDEK = prepared.SealedDEK
	meta.SSEKMSContextJSON = prepared.SSEKMSContextJSON
	meta.Data = ct
	return true
}

func (s *Server) s3EncryptObjectPayload(verified *authn.Verified, sseAlgorithm, kmsKeyID, objectARN, sseKMSContextJSON string, sealedDEK, plain []byte) ([]byte, error) {
	switch sseAlgorithm {
	case sseAES256:
		dek, err := s.store.UnsealWithMaster(sealedDEK)
		if err != nil {
			return nil, err
		}
		return s3crypto.EncryptAES256GCM(dek, plain)
	case sseAWSKMS:
		keyID, err := s.store.ResolveKeyID(verified.AccountID, kmsKeyID)
		if err != nil {
			return nil, err
		}
		kmsKey, err := s.store.GetKey(keyID)
		if err != nil {
			return nil, err
		}
		customerCtx, err := parseStoredS3CustomerEncryptionContext(sseKMSContextJSON)
		if err != nil {
			return nil, err
		}
		encCtx := s3SSEKMSEncryptionContext(objectARN, customerCtx)
		if !s.authorizeKMSOp(verified, catalog.ActionKMSDecrypt, kmsKey, encCtx) {
			return nil, errS3AccessDenied
		}
		dek, err := s.store.DecryptBlobWithKeyContext(keyID, sealedDEK, encCtx)
		if err != nil {
			return nil, err
		}
		return s3crypto.EncryptAES256GCM(dek, plain)
	default:
		return nil, fmt.Errorf("unsupported sse algorithm %q", sseAlgorithm)
	}
}

func (s *Server) mapS3StoreError(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	err error,
	eventName string,
) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrNoSuchBucket):
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", eventName)
	case errors.Is(err, store.ErrInvalidObjectKey), errors.Is(err, store.ErrInvalidBucketName):
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
			err.Error(), eventName)
	default:
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", eventName)
	}
	return true
}

var errS3AccessDenied = errors.New("s3 access denied")

func (s *Server) decryptObjectPayload(verified *authn.Verified, meta store.ObjectMeta, data []byte) ([]byte, error) {
	switch meta.SSEAlgorithm {
	case "":
		return data, nil
	case sseAES256:
		dek, err := s.store.UnsealWithMaster(meta.SealedDEK)
		if err != nil {
			return nil, err
		}
		return s3crypto.DecryptAES256GCM(dek, data)
	case sseAWSKMS:
		keyID, err := s.store.ResolveKeyID(verified.AccountID, meta.KMSKeyID)
		if err != nil {
			return nil, err
		}
		kmsKey, err := s.store.GetKey(keyID)
		if err != nil {
			return nil, err
		}
		objectARN := store.ObjectARN(meta.Bucket, meta.Key)
		customerCtx, err := parseStoredS3CustomerEncryptionContext(meta.SSEKMSContextJSON)
		if err != nil {
			return nil, err
		}
		encCtx := s3SSEKMSEncryptionContext(objectARN, customerCtx)
		if !s.authorizeKMSOp(verified, catalog.ActionKMSDecrypt, kmsKey, encCtx) {
			return nil, errS3AccessDenied
		}
		dek, err := s.store.DecryptBlobWithKeyContext(keyID, meta.SealedDEK, encCtx)
		if err != nil {
			return nil, err
		}
		return s3crypto.DecryptAES256GCM(dek, data)
	default:
		return nil, fmt.Errorf("unsupported sse algorithm %q", meta.SSEAlgorithm)
	}
}

func (s *Server) authorizeKMSOp(verified *authn.Verified, action string, key store.Key, encCtx map[string]string) bool {
	grantSatisfied := false
	if ok, gerr := s.store.FindMatchingGrant(key.KeyID, verified.Principal.ARN(), action); gerr == nil {
		grantSatisfied = ok
	}
	return s.authorizeDataplaneKMS(verified, action, key.ARN, key.KeyPolicy, grantSatisfied, encCtx)
}

func s3SSEKMSEncryptionContext(objectARN string, customerCtx map[string]string) map[string]string {
	out := make(map[string]string, 1+len(customerCtx))
	for k, v := range customerCtx {
		out[k] = v
	}
	// AWS S3 SSE-KMS default context (object ARN when Bucket Keys are off).
	out["aws:s3:arn"] = objectARN
	return out
}

func (s *Server) writeS3XML(w http.ResponseWriter, requestID string, status int, payload any) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	data, err := xml.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(data)
}

func (s *Server) writeS3Error(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	status int,
	code, message, eventName string,
) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	resp := s3ErrorXML{Code: code, Message: message, RequestID: requestID}
	data, err := xml.Marshal(resp)
	if err == nil {
		_, _ = w.Write([]byte(xml.Header))
		_, _ = w.Write(data)
	}
	if eventName == "" {
		eventName = eventNameForRequest(r)
	}
	ev := audit.Event{
		EventVersion:       eventVersion,
		EventTime:          s.now().UTC().Format(time.RFC3339),
		EventSource:        "s3.amazonaws.com",
		EventName:          eventName,
		SourceIPAddress:    clientIP(r),
		UserAgent:          r.UserAgent(),
		RequestID:          requestID,
		EventID:            eventID,
		EventType:          "AwsApiCall",
		RecipientAccountID: s.cfg.AccountID,
		ReadOnly:           readOnly,
		ErrorCode:          code,
		ErrorMessage:       message,
		RequestParameters: map[string]any{
			"httpMethod": r.Method,
			"path":       r.URL.Path,
		},
	}
	if verified != nil {
		ev.RecipientAccountID = verified.AccountID
		ev.AWSRegion = verified.Region
		ev.UserIdentity = map[string]any{
			"type":        "IAMUser",
			"accountId":   verified.AccountID,
			"accessKeyId": verified.AccessKeyID,
		}
	}
	_ = s.audit.Write(context.Background(), ev)
}

func s3HTTPLastModified(stored string) string {
	t, err := time.Parse(time.RFC3339, stored)
	if err != nil {
		return time.Now().UTC().Format(http.TimeFormat)
	}
	return t.UTC().Format(http.TimeFormat)
}

func s3XMLLastModified(stored string) string {
	t, err := time.Parse(time.RFC3339, stored)
	if err != nil {
		return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	}
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}
