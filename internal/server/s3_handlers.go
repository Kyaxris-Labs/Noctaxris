package server

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	kmssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/kms"
	s3crypto "github.com/Kyaxris-Labs/Noctaxris/internal/services/s3"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	s3XMLNS           = "http://s3.amazonaws.com/doc/2006-03-01/"
	sseAES256         = "AES256"
	sseAWSKMS         = "aws:kms"
	headerSSESSE      = "x-amz-server-side-encryption"
	headerSSEKMSKeyID = "x-amz-server-side-encryption-aws-kms-key-id"
)

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
			"This S3 API is not implemented in Noctaxris Phase 5.", "Unknown")
		return
	}

	q := r.URL.Query()
	switch {
	case r.Method == http.MethodGet && bucket == "" && key == "":
		s.s3ListBuckets(w, r, requestID, eventID, verified, readOnly)
	case r.Method == http.MethodPut && bucket != "" && key == "" && q.Has("policy"):
		s.s3PutBucketPolicy(w, r, body, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodGet && bucket != "" && key == "" && q.Has("policy"):
		s.s3GetBucketPolicy(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodDelete && bucket != "" && key == "" && q.Has("policy"):
		s.s3DeleteBucketPolicy(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodGet && bucket != "" && key == "" && (q.Get("list-type") == "2" || q.Has("list-type")):
		s.s3ListObjectsV2(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodPut && bucket != "" && key == "":
		s.s3CreateBucket(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodDelete && bucket != "" && key == "":
		s.s3DeleteBucket(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodHead && bucket != "" && key == "":
		s.s3HeadBucket(w, r, requestID, eventID, verified, readOnly, bucket)
	case r.Method == http.MethodGet && bucket != "" && key == "":
		// Default bucket GET is ListObjectsV2 for lab convenience when list-type omitted.
		s.s3ListObjectsV2(w, r, requestID, eventID, verified, readOnly, bucket)
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
			"This S3 API is not implemented in Noctaxris Phase 5.", eventNameForRequest(r))
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

func (s *Server) authorizeS3(verified *authn.Verified, action, resource, bucketPolicy string) bool {
	identityDocs := s.identityDocs(verified.Principal)
	decision := authz.EvaluateS3(authz.S3Request{
		Caller: authz.RequestContext{
			Principal: verified.Principal,
			Action:    action,
			Resource:  resource,
			Region:    verified.Region,
			ConditionKeys: map[string]string{
				"aws:PrincipalAccount": verified.AccountID,
				"aws:RequestedRegion":  verified.Region,
			},
		},
		IdentityDocs:    identityDocs,
		BucketPolicyDoc: bucketPolicy,
	})
	if decision != authz.Allow {
		return false
	}
	if sessionDocs := s.sessionPolicyDocs(verified.AccessKeyID); len(sessionDocs) > 0 {
		return authz.EvaluateWithSession(authz.RequestContext{
			Principal: verified.Principal,
			Action:    action,
			Resource:  resource,
			Region:    verified.Region,
			ConditionKeys: map[string]string{
				"aws:PrincipalAccount": verified.AccountID,
				"aws:RequestedRegion":  verified.Region,
			},
		}, identityDocs, sessionDocs) == authz.Allow
	}
	return true
}

func (s *Server) bucketPolicyOrEmpty(accountID, bucket string) string {
	b, err := s.store.GetBucket(accountID, bucket)
	if err != nil {
		return ""
	}
	return b.BucketPolicy
}

func (s *Server) s3ListBuckets(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool) {
	if !s.authorizeS3(verified, catalog.ActionS3ListAllMyBuckets, "*", "") {
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
	if !s.authorizeS3(verified, catalog.ActionS3CreateBucket, store.BucketARN(bucket), "") {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "CreateBucket")
		return
	}
	_, err := s.store.CreateBucket(verified.AccountID, bucket)
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
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "CreateBucket", readOnly)
}

func (s *Server) s3DeleteBucket(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	policy := s.bucketPolicyOrEmpty(verified.AccountID, bucket)
	if !s.authorizeS3(verified, catalog.ActionS3DeleteBucket, store.BucketARN(bucket), policy) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "DeleteBucket")
		return
	}
	err := s.store.DeleteBucket(verified.AccountID, bucket)
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
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "DeleteBucket", readOnly)
}

func (s *Server) s3HeadBucket(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	policy := s.bucketPolicyOrEmpty(verified.AccountID, bucket)
	if !s.authorizeS3(verified, catalog.ActionS3ListBucket, store.BucketARN(bucket), policy) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "HeadBucket")
		return
	}
	if err := s.store.HeadBucket(verified.AccountID, bucket); err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "HeadBucket")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "HeadBucket", readOnly)
}

func (s *Server) s3PutBucketPolicy(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	policy := s.bucketPolicyOrEmpty(verified.AccountID, bucket)
	if !s.authorizeS3(verified, catalog.ActionS3PutBucketPolicy, store.BucketARN(bucket), policy) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "PutBucketPolicy")
		return
	}
	if err := s.store.PutBucketPolicy(verified.AccountID, bucket, string(body)); err != nil {
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
	policy := s.bucketPolicyOrEmpty(verified.AccountID, bucket)
	if !s.authorizeS3(verified, catalog.ActionS3GetBucketPolicy, store.BucketARN(bucket), policy) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "GetBucketPolicy")
		return
	}
	doc, err := s.store.GetBucketPolicy(verified.AccountID, bucket)
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
	policy := s.bucketPolicyOrEmpty(verified.AccountID, bucket)
	if !s.authorizeS3(verified, catalog.ActionS3DeleteBucketPolicy, store.BucketARN(bucket), policy) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "DeleteBucketPolicy")
		return
	}
	if err := s.store.DeleteBucketPolicy(verified.AccountID, bucket); err != nil {
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

func (s *Server) s3ListObjectsV2(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	policy := s.bucketPolicyOrEmpty(verified.AccountID, bucket)
	if !s.authorizeS3(verified, catalog.ActionS3ListBucket, store.BucketARN(bucket), policy) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "ListObjectsV2")
		return
	}
	prefix := r.URL.Query().Get("prefix")
	delimiter := r.URL.Query().Get("delimiter")
	result, err := s.store.ListObjectsV2(verified.AccountID, bucket, prefix, delimiter)
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
	policy := s.bucketPolicyOrEmpty(verified.AccountID, bucket)
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3PutObject, resource, policy) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "PutObject")
		return
	}

	sse := strings.TrimSpace(r.Header.Get(headerSSESSE))
	kmsKeyParam := strings.TrimSpace(r.Header.Get(headerSSEKMSKeyID))
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	plainSum := md5.Sum(body)
	etag := hex.EncodeToString(plainSum[:])
	meta := store.PutObjectMeta{
		ContentType: contentType,
		Data:        body,
		PlainSize:   int64(len(body)),
		ETag:        etag,
	}

	switch strings.ToUpper(sse) {
	case "":
		// plaintext storage
	case sseAES256:
		dek := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, dek); err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", "PutObject")
			return
		}
		sealedDEK, err := s.store.SealWithMaster(dek)
		if err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", "PutObject")
			return
		}
		ct, err := s3crypto.EncryptAES256GCM(dek, body)
		if err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", "PutObject")
			return
		}
		meta.SSEAlgorithm = sseAES256
		meta.SealedDEK = sealedDEK
		meta.Data = ct
	case "AWS:KMS":
		if kmsKeyParam == "" {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				"x-amz-server-side-encryption-aws-kms-key-id is required for aws:kms", "PutObject")
			return
		}
		keyID, err := s.store.ResolveKeyID(verified.AccountID, kmsKeyParam)
		if err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				"KMS key not found", "PutObject")
			return
		}
		kmsKey, err := s.store.GetKey(keyID)
		if err != nil || kmsKey.AccountID != verified.AccountID {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				"KMS key not found", "PutObject")
			return
		}
		if !s.authorizeKMSOp(verified, catalog.ActionKMSGenerateDataKey, kmsKey) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
				"Access Denied", "PutObject")
			return
		}
		if kmsKey.KeyState != store.KeyStateEnabled {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				"KMS key is disabled", "PutObject")
			return
		}
		cmk, err := s.store.UnsealKeyMaterial(keyID)
		if err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", "PutObject")
			return
		}
		dek := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, dek); err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", "PutObject")
			return
		}
		sealedDEK, err := kmssvc.EncryptUnderCMK(cmk, keyID, dek)
		if err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", "PutObject")
			return
		}
		ct, err := s3crypto.EncryptAES256GCM(dek, body)
		if err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", "PutObject")
			return
		}
		meta.SSEAlgorithm = sseAWSKMS
		meta.KMSKeyID = kmsKey.ARN
		meta.SealedDEK = sealedDEK
		meta.Data = ct
	default:
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
			"Unsupported server-side encryption", "PutObject")
		return
	}

	obj, err := s.store.PutObject(verified.AccountID, bucket, key, meta)
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "PutObject")
		return
	}
	if errors.Is(err, store.ErrInvalidObjectKey) || errors.Is(err, store.ErrInvalidBucketName) {
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
	if obj.SSEAlgorithm != "" {
		w.Header().Set(headerSSESSE, obj.SSEAlgorithm)
	}
	if obj.KMSKeyID != "" {
		w.Header().Set(headerSSEKMSKeyID, obj.KMSKeyID)
	}
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "PutObject", readOnly)
}

func (s *Server) s3GetObject(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket, key string) {
	policy := s.bucketPolicyOrEmpty(verified.AccountID, bucket)
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3GetObject, resource, policy) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "GetObject")
		return
	}
	meta, data, err := s.store.GetObject(verified.AccountID, bucket, key)
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
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("ETag", `"`+meta.ETag+`"`)
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
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "GetObject", readOnly)
}

func (s *Server) s3HeadObject(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket, key string) {
	policy := s.bucketPolicyOrEmpty(verified.AccountID, bucket)
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3GetObject, resource, policy) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "HeadObject")
		return
	}
	meta, err := s.store.HeadObject(verified.AccountID, bucket, key)
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
	policy := s.bucketPolicyOrEmpty(verified.AccountID, bucket)
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3DeleteObject, resource, policy) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "DeleteObject")
		return
	}
	err := s.store.DeleteObject(verified.AccountID, bucket, key)
	if errors.Is(err, store.ErrNoSuchKey) || errors.Is(err, store.ErrInvalidObjectKey) {
		// AWS DeleteObject is idempotent for missing keys.
		w.Header().Set(requestIDHeader, requestID)
		w.WriteHeader(http.StatusNoContent)
		s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "DeleteObject", readOnly)
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "DeleteObject")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusNoContent)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "DeleteObject", readOnly)
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
		if !s.authorizeKMSOp(verified, catalog.ActionKMSDecrypt, kmsKey) {
			return nil, errS3AccessDenied
		}
		cmk, err := s.store.UnsealKeyMaterial(keyID)
		if err != nil {
			return nil, err
		}
		dek, err := kmssvc.DecryptUnderCMK(cmk, meta.SealedDEK)
		if err != nil {
			return nil, err
		}
		return s3crypto.DecryptAES256GCM(dek, data)
	default:
		return nil, fmt.Errorf("unsupported sse algorithm %q", meta.SSEAlgorithm)
	}
}

func (s *Server) authorizeKMSOp(verified *authn.Verified, action string, key store.Key) bool {
	identityDocs := s.identityDocs(verified.Principal)
	grantSatisfied := false
	if ok, gerr := s.store.FindMatchingGrant(key.KeyID, verified.Principal.ARN(), action); gerr == nil {
		grantSatisfied = ok
	}
	decision := authz.EvaluateKMS(authz.KMSRequest{
		Caller: authz.RequestContext{
			Principal: verified.Principal,
			Action:    action,
			Resource:  key.ARN,
			Region:    verified.Region,
			ConditionKeys: map[string]string{
				"aws:PrincipalAccount": verified.AccountID,
				"aws:RequestedRegion":  verified.Region,
			},
		},
		IdentityDocs:   identityDocs,
		KeyPolicyDoc:   key.KeyPolicy,
		GrantSatisfied: grantSatisfied,
	})
	return decision == authz.Allow
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
