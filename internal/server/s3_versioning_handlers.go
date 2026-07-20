package server

import (
	"encoding/xml"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

type s3VersioningConfigurationXML struct {
	XMLName xml.Name `xml:"VersioningConfiguration"`
	XMLNS   string   `xml:"xmlns,attr"`
	Status  string   `xml:"Status"`
}

type s3ListVersionsResultXML struct {
	XMLName     xml.Name                 `xml:"ListVersionsResult"`
	XMLNS       string                   `xml:"xmlns,attr"`
	Name        string                   `xml:"Name"`
	Prefix      string                   `xml:"Prefix"`
	IsTruncated bool                     `xml:"IsTruncated"`
	Versions    []s3ObjectVersionEntryXML `xml:"Version"`
}

type s3ObjectVersionEntryXML struct {
	Key          string `xml:"Key"`
	VersionId    string `xml:"VersionId"`
	IsLatest     bool   `xml:"IsLatest"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

func (s *Server) s3PutBucketVersioning(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "PutBucketVersioning")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3PutBucketVersioning, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "PutBucketVersioning")
		return
	}
	var cfg s3VersioningConfigurationXML
	if err := xml.Unmarshal(body, &cfg); err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "MalformedXML",
			"The XML you provided was not well-formed", "PutBucketVersioning")
		return
	}
	status := strings.TrimSpace(cfg.Status)
	if status != store.VersioningEnabled && status != store.VersioningSuspended {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "IllegalVersioningConfigurationException",
			"Status must be Enabled or Suspended", "PutBucketVersioning")
		return
	}
	if err := s.store.SetBucketVersioning(ref.accountID, bucket, status); err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "PutBucketVersioning")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "PutBucketVersioning", readOnly)
}

func (s *Server) s3GetBucketVersioning(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "GetBucketVersioning")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3GetBucketVersioning, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "GetBucketVersioning")
		return
	}
	status, err := s.store.GetBucketVersioning(ref.accountID, bucket)
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "GetBucketVersioning")
		return
	}
	out := s3VersioningConfigurationXML{XMLNS: "http://s3.amazonaws.com/doc/2006-03-01/", Status: status}
	payload, err := xml.Marshal(out)
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "GetBucketVersioning")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append([]byte(xml.Header), payload...))
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "GetBucketVersioning", readOnly)
}

func (s *Server) s3ListObjectVersions(w http.ResponseWriter, r *http.Request, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "ListObjectVersions")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3ListBucketVersions, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "ListObjectVersions")
		return
	}
	prefix := r.URL.Query().Get("prefix")
	versions, err := s.store.ListObjectVersions(ref.accountID, bucket, prefix)
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "ListObjectVersions")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "ListObjectVersions")
		return
	}
	out := s3ListVersionsResultXML{
		XMLNS:  "http://s3.amazonaws.com/doc/2006-03-01/",
		Name:   bucket,
		Prefix: prefix,
	}
	for _, v := range versions {
		out.Versions = append(out.Versions, s3ObjectVersionEntryXML{
			Key: v.Key, VersionId: v.VersionID, IsLatest: v.IsLatest,
			LastModified: v.LastModified, ETag: `"` + v.ETag + `"`, Size: v.Size, StorageClass: "STANDARD",
		})
	}
	payload, err := xml.Marshal(out)
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "ListObjectVersions")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append([]byte(xml.Header), payload...))
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "ListObjectVersions", readOnly)
}
