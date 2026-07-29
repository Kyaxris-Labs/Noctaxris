package server

import (
	"encoding/xml"
	"errors"
	"net/http"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

type s3CORSConfigurationXML struct {
	XMLName xml.Name       `xml:"CORSConfiguration"`
	XMLNS   string         `xml:"xmlns,attr"`
	Rules   []s3CORSRuleXML `xml:"CORSRule"`
}

type s3CORSRuleXML struct {
	ID             string   `xml:"ID,omitempty"`
	AllowedHeader  []string `xml:"AllowedHeader,omitempty"`
	AllowedMethod  []string `xml:"AllowedMethod"`
	AllowedOrigin  []string `xml:"AllowedOrigin"`
	ExposeHeader   []string `xml:"ExposeHeader,omitempty"`
	MaxAgeSeconds  *int     `xml:"MaxAgeSeconds,omitempty"`
}

type s3LifecycleConfigurationXML struct {
	XMLName xml.Name            `xml:"LifecycleConfiguration"`
	XMLNS   string              `xml:"xmlns,attr"`
	Rules   []s3LifecycleRuleXML `xml:"Rule"`
}

type s3LifecycleRuleXML struct {
	ID                             string                                         `xml:"ID,omitempty"`
	Status                         string                                         `xml:"Status"`
	Prefix                         string                                         `xml:"Prefix,omitempty"`
	Filter                         *s3LifecycleFilterXML                          `xml:"Filter,omitempty"`
	Expiration                     *s3LifecycleExpirationXML                      `xml:"Expiration,omitempty"`
	Transition                     []s3LifecycleTransitionXML                     `xml:"Transition,omitempty"`
	AbortIncompleteMultipartUpload *s3LifecycleAbortIncompleteMultipartUploadXML  `xml:"AbortIncompleteMultipartUpload,omitempty"`
	NoncurrentVersionExpiration    *s3LifecycleNoncurrentVersionExpirationXML     `xml:"NoncurrentVersionExpiration,omitempty"`
}

type s3LifecycleFilterXML struct {
	Prefix string              `xml:"Prefix,omitempty"`
	Tag    *s3LifecycleTagXML  `xml:"Tag,omitempty"`
}

type s3LifecycleTagXML struct {
	Key   string `xml:"Key"`
	Value string `xml:"Value"`
}

type s3LifecycleExpirationXML struct {
	Days                      *int   `xml:"Days,omitempty"`
	Date                      string `xml:"Date,omitempty"`
	ExpiredObjectDeleteMarker *bool  `xml:"ExpiredObjectDeleteMarker,omitempty"`
}

type s3LifecycleTransitionXML struct {
	Days         *int   `xml:"Days,omitempty"`
	Date         string `xml:"Date,omitempty"`
	StorageClass string `xml:"StorageClass,omitempty"`
}

type s3LifecycleAbortIncompleteMultipartUploadXML struct {
	DaysAfterInitiation *int `xml:"DaysAfterInitiation,omitempty"`
}

type s3LifecycleNoncurrentVersionExpirationXML struct {
	NoncurrentDays          *int `xml:"NoncurrentDays,omitempty"`
	NewerNoncurrentVersions *int `xml:"NewerNoncurrentVersions,omitempty"`
}

func (s *Server) s3PutBucketCors(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "PutBucketCors")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3PutBucketCors, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "PutBucketCors")
		return
	}
	var cfgXML s3CORSConfigurationXML
	if err := xml.Unmarshal(body, &cfgXML); err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "MalformedXML",
			"The XML you provided was not well-formed or did not validate against our published schema.", "PutBucketCors")
		return
	}
	cfg := store.S3CORSConfiguration{Rules: make([]store.S3CORSRule, 0, len(cfgXML.Rules))}
	for _, rule := range cfgXML.Rules {
		cfg.Rules = append(cfg.Rules, store.S3CORSRule{
			ID:             rule.ID,
			AllowedOrigins: rule.AllowedOrigin,
			AllowedMethods: rule.AllowedMethod,
			AllowedHeaders: rule.AllowedHeader,
			ExposeHeaders:  rule.ExposeHeader,
			MaxAgeSeconds:  rule.MaxAgeSeconds,
		})
	}
	if err := s.store.PutBucketCors(ref.accountID, bucket, cfg); err != nil {
		if errors.Is(err, store.ErrNoSuchBucket) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
				"The specified bucket does not exist", "PutBucketCors")
			return
		}
		if errors.Is(err, store.ErrInvalidCORSConfiguration) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidRequest",
				err.Error(), "PutBucketCors")
			return
		}
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "PutBucketCors")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "PutBucketCors", readOnly)
}

func (s *Server) s3GetBucketCors(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "GetBucketCors")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3GetBucketCors, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "GetBucketCors")
		return
	}
	cfg, err := s.store.GetBucketCors(ref.accountID, bucket)
	if errors.Is(err, store.ErrNoSuchCORSConfiguration) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchCORSConfiguration",
			"The CORS configuration does not exist", "GetBucketCors")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "GetBucketCors")
		return
	}
	out := s3CORSConfigurationXML{
		XMLNS: "http://s3.amazonaws.com/doc/2006-03-01/",
		Rules: make([]s3CORSRuleXML, 0, len(cfg.Rules)),
	}
	for _, rule := range cfg.Rules {
		out.Rules = append(out.Rules, s3CORSRuleXML{
			ID:            rule.ID,
			AllowedHeader: rule.AllowedHeaders,
			AllowedMethod: rule.AllowedMethods,
			AllowedOrigin: rule.AllowedOrigins,
			ExposeHeader:  rule.ExposeHeaders,
			MaxAgeSeconds: rule.MaxAgeSeconds,
		})
	}
	s.writeS3XML(w, requestID, http.StatusOK, out)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "GetBucketCors", readOnly)
}

func (s *Server) s3DeleteBucketCors(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "DeleteBucketCors")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3DeleteBucketCors, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "DeleteBucketCors")
		return
	}
	if err := s.store.DeleteBucketCors(ref.accountID, bucket); err != nil {
		if errors.Is(err, store.ErrNoSuchBucket) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
				"The specified bucket does not exist", "DeleteBucketCors")
			return
		}
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "DeleteBucketCors")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusNoContent)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "DeleteBucketCors", readOnly)
}

func lifecycleXMLToStore(cfgXML s3LifecycleConfigurationXML) store.S3LifecycleConfiguration {
	cfg := store.S3LifecycleConfiguration{Rules: make([]store.S3LifecycleRule, 0, len(cfgXML.Rules))}
	for _, rule := range cfgXML.Rules {
		out := store.S3LifecycleRule{
			ID:     rule.ID,
			Status: rule.Status,
			Prefix: rule.Prefix,
		}
		if rule.Filter != nil {
			f := &store.S3LifecycleFilter{Prefix: rule.Filter.Prefix}
			if rule.Filter.Tag != nil {
				f.TagKey = rule.Filter.Tag.Key
				f.TagVal = rule.Filter.Tag.Value
			}
			out.Filter = f
		}
		if rule.Expiration != nil {
			out.Expiration = &store.S3LifecycleExpiration{
				Days:                      rule.Expiration.Days,
				Date:                      rule.Expiration.Date,
				ExpiredObjectDeleteMarker: rule.Expiration.ExpiredObjectDeleteMarker,
			}
		}
		if len(rule.Transition) > 0 {
			out.Transitions = make([]store.S3LifecycleTransition, 0, len(rule.Transition))
			for _, tr := range rule.Transition {
				out.Transitions = append(out.Transitions, store.S3LifecycleTransition{
					Days: tr.Days, Date: tr.Date, StorageClass: tr.StorageClass,
				})
			}
		}
		if rule.AbortIncompleteMultipartUpload != nil {
			out.AbortIncompleteMultipartUpload = &store.S3LifecycleAbortIncompleteMultipartUpload{
				DaysAfterInitiation: rule.AbortIncompleteMultipartUpload.DaysAfterInitiation,
			}
		}
		if rule.NoncurrentVersionExpiration != nil {
			out.NoncurrentVersionExpiration = &store.S3LifecycleNoncurrentVersionExpiration{
				NoncurrentDays:          rule.NoncurrentVersionExpiration.NoncurrentDays,
				NewerNoncurrentVersions: rule.NoncurrentVersionExpiration.NewerNoncurrentVersions,
			}
		}
		cfg.Rules = append(cfg.Rules, out)
	}
	return cfg
}

func lifecycleStoreToXML(cfg store.S3LifecycleConfiguration) s3LifecycleConfigurationXML {
	out := s3LifecycleConfigurationXML{
		XMLNS: "http://s3.amazonaws.com/doc/2006-03-01/",
		Rules: make([]s3LifecycleRuleXML, 0, len(cfg.Rules)),
	}
	for _, rule := range cfg.Rules {
		x := s3LifecycleRuleXML{ID: rule.ID, Status: rule.Status, Prefix: rule.Prefix}
		if rule.Filter != nil {
			f := &s3LifecycleFilterXML{Prefix: rule.Filter.Prefix}
			if rule.Filter.TagKey != "" {
				f.Tag = &s3LifecycleTagXML{Key: rule.Filter.TagKey, Value: rule.Filter.TagVal}
			}
			x.Filter = f
		}
		if rule.Expiration != nil {
			x.Expiration = &s3LifecycleExpirationXML{
				Days: rule.Expiration.Days, Date: rule.Expiration.Date,
				ExpiredObjectDeleteMarker: rule.Expiration.ExpiredObjectDeleteMarker,
			}
		}
		for _, tr := range rule.Transitions {
			x.Transition = append(x.Transition, s3LifecycleTransitionXML{
				Days: tr.Days, Date: tr.Date, StorageClass: tr.StorageClass,
			})
		}
		if rule.AbortIncompleteMultipartUpload != nil {
			x.AbortIncompleteMultipartUpload = &s3LifecycleAbortIncompleteMultipartUploadXML{
				DaysAfterInitiation: rule.AbortIncompleteMultipartUpload.DaysAfterInitiation,
			}
		}
		if rule.NoncurrentVersionExpiration != nil {
			x.NoncurrentVersionExpiration = &s3LifecycleNoncurrentVersionExpirationXML{
				NoncurrentDays:          rule.NoncurrentVersionExpiration.NoncurrentDays,
				NewerNoncurrentVersions: rule.NoncurrentVersionExpiration.NewerNoncurrentVersions,
			}
		}
		out.Rules = append(out.Rules, x)
	}
	return out
}

func (s *Server) s3PutLifecycleConfiguration(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "PutLifecycleConfiguration")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3PutLifecycleConfiguration, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "PutLifecycleConfiguration")
		return
	}
	var cfgXML s3LifecycleConfigurationXML
	if err := xml.Unmarshal(body, &cfgXML); err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "MalformedXML",
			"The XML you provided was not well-formed or did not validate against our published schema.", "PutLifecycleConfiguration")
		return
	}
	if err := s.store.PutLifecycleConfiguration(ref.accountID, bucket, lifecycleXMLToStore(cfgXML)); err != nil {
		if errors.Is(err, store.ErrNoSuchBucket) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
				"The specified bucket does not exist", "PutLifecycleConfiguration")
			return
		}
		if errors.Is(err, store.ErrInvalidLifecycleConfiguration) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidRequest",
				err.Error(), "PutLifecycleConfiguration")
			return
		}
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "PutLifecycleConfiguration")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "PutLifecycleConfiguration", readOnly)
}

func (s *Server) s3GetLifecycleConfiguration(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "GetLifecycleConfiguration")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3GetLifecycleConfiguration, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "GetLifecycleConfiguration")
		return
	}
	cfg, err := s.store.GetLifecycleConfiguration(ref.accountID, bucket)
	if errors.Is(err, store.ErrNoSuchLifecycleConfiguration) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchLifecycleConfiguration",
			"The lifecycle configuration does not exist", "GetLifecycleConfiguration")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "GetLifecycleConfiguration")
		return
	}
	s.writeS3XML(w, requestID, http.StatusOK, lifecycleStoreToXML(cfg))
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "GetLifecycleConfiguration", readOnly)
}

func (s *Server) s3DeleteLifecycleConfiguration(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "DeleteLifecycleConfiguration")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3DeleteLifecycleConfiguration, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "DeleteLifecycleConfiguration")
		return
	}
	if err := s.store.DeleteLifecycleConfiguration(ref.accountID, bucket); err != nil {
		if errors.Is(err, store.ErrNoSuchBucket) {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
				"The specified bucket does not exist", "DeleteLifecycleConfiguration")
			return
		}
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "DeleteLifecycleConfiguration")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusNoContent)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "DeleteLifecycleConfiguration", readOnly)
}
