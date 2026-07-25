package server

import (
	"encoding/xml"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

type s3BucketLoggingStatusXML struct {
	XMLName        xml.Name                   `xml:"BucketLoggingStatus"`
	LoggingEnabled *s3BucketLoggingEnabledXML `xml:"LoggingEnabled"`
}

type s3BucketLoggingEnabledXML struct {
	TargetBucket string `xml:"TargetBucket"`
	TargetPrefix string `xml:"TargetPrefix"`
}

func (s *Server) s3PutBucketLogging(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "PutBucketLogging")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3PutBucketLogging, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "PutBucketLogging")
		return
	}
	var status s3BucketLoggingStatusXML
	if len(body) > 0 {
		if err := xml.Unmarshal(body, &status); err != nil {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "MalformedXML",
				"The XML you provided was not well-formed.", "PutBucketLogging")
			return
		}
	}
	cfg := store.S3AccessLoggingConfig{}
	if status.LoggingEnabled != nil {
		cfg.Enabled = true
		cfg.TargetBucket = strings.TrimSpace(status.LoggingEnabled.TargetBucket)
		cfg.TargetPrefix = strings.TrimSpace(status.LoggingEnabled.TargetPrefix)
	}
	if err := s.store.PutBucketLogging(ref.accountID, bucket, cfg); err != nil {
		if errors.Is(err, store.ErrNoSuchBucket) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
				"The specified bucket does not exist", "PutBucketLogging")
			return
		}
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
			err.Error(), "PutBucketLogging")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "PutBucketLogging", readOnly)
}

func (s *Server) s3GetBucketLogging(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "GetBucketLogging")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3GetBucketLogging, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "GetBucketLogging")
		return
	}
	cfg, err := s.store.GetBucketLogging(ref.accountID, bucket)
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "GetBucketLogging")
		return
	}
	out := s3BucketLoggingStatusXML{XMLName: xml.Name{Local: "BucketLoggingStatus"}}
	if cfg.Enabled {
		out.LoggingEnabled = &s3BucketLoggingEnabledXML{
			TargetBucket: cfg.TargetBucket,
			TargetPrefix: cfg.TargetPrefix,
		}
	}
	s.writeS3XML(w, requestID, http.StatusOK, out)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "GetBucketLogging", readOnly)
}

func (s *Server) emitS3ServerAccessLog(accountID, bucket, operation, key, requestID string, r *http.Request, verified *authn.Verified, httpStatus int, bytesSent, objectSize int64) {
	if s == nil || s.store == nil {
		return
	}
	remoteIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remoteIP); err == nil {
		remoteIP = host
	}
	requester := "-"
	if verified != nil {
		if arn := verified.Principal.ARN(); arn != "" {
			requester = arn
		}
	}
	_ = s.store.AppendS3ServerAccessLog(accountID, bucket, store.S3ServerAccessLogInput{
		Operation:  operation,
		Key:        key,
		RequestID:  requestID,
		RemoteIP:   remoteIP,
		BytesSent:  bytesSent,
		ObjectSize: objectSize,
		Requester:  requester,
		HTTPStatus: httpStatus,
	})
}

func createBucketObjectLockEnabled(r *http.Request, body []byte) bool {
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("x-amz-bucket-object-lock-enabled")), "true") {
		return true
	}
	if len(body) == 0 {
		return false
	}
	return strings.Contains(string(body), "<ObjectLockEnabledForBucket>true</ObjectLockEnabledForBucket>")
}

func s3ObjectLockFromRequest(r *http.Request, meta *store.PutObjectMeta) {
	if meta == nil || r == nil {
		return
	}
	if mode := strings.TrimSpace(r.Header.Get("x-amz-object-lock-mode")); mode != "" {
		meta.ObjectLockMode = mode
	}
	if retain := strings.TrimSpace(r.Header.Get("x-amz-object-lock-retain-until-date")); retain != "" {
		meta.ObjectLockRetainUntil = retain
	}
}

func s3BypassGovernanceRetention(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("x-amz-bypass-governance-retention")), "true")
}
