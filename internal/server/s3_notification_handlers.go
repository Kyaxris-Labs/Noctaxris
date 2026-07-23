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

type s3NotificationConfigurationXML struct {
	XMLName                      xml.Name                           `xml:"NotificationConfiguration"`
	XMLNS                        string                             `xml:"xmlns,attr,omitempty"`
	TopicConfigurations          []s3TopicConfigurationXML          `xml:"TopicConfiguration"`
	QueueConfigurations          []s3QueueConfigurationXML          `xml:"QueueConfiguration"`
	CloudFunctionConfigurations  []s3CloudFunctionConfigurationXML  `xml:"CloudFunctionConfiguration"`
	LambdaFunctionConfigurations []s3LambdaFunctionConfigurationXML `xml:"LambdaFunctionConfiguration"`
	EventBridgeConfiguration     *s3EventBridgeConfigurationXML     `xml:"EventBridgeConfiguration"`
}

type s3EventBridgeConfigurationXML struct {
	XMLName xml.Name `xml:"EventBridgeConfiguration"`
}

type s3TopicConfigurationXML struct {
	ID     string             `xml:"Id,omitempty"`
	Topic  string             `xml:"Topic"`
	Events []string           `xml:"Event"`
	Filter *s3NotifyFilterXML `xml:"Filter,omitempty"`
}

type s3QueueConfigurationXML struct {
	ID     string             `xml:"Id,omitempty"`
	Queue  string             `xml:"Queue"`
	Events []string           `xml:"Event"`
	Filter *s3NotifyFilterXML `xml:"Filter,omitempty"`
}

type s3CloudFunctionConfigurationXML struct {
	ID            string             `xml:"Id,omitempty"`
	CloudFunction string             `xml:"CloudFunction"`
	Events        []string           `xml:"Event"`
	Filter        *s3NotifyFilterXML `xml:"Filter,omitempty"`
}

type s3LambdaFunctionConfigurationXML struct {
	ID                string             `xml:"Id,omitempty"`
	LambdaFunctionArn string             `xml:"LambdaFunctionArn"`
	CloudFunction     string             `xml:"CloudFunction,omitempty"`
	Events            []string           `xml:"Event"`
	Filter            *s3NotifyFilterXML `xml:"Filter,omitempty"`
}

type s3NotifyFilterXML struct {
	S3Key s3NotifyS3KeyXML `xml:"S3Key"`
}

type s3NotifyS3KeyXML struct {
	FilterRules []s3NotifyFilterRuleXML `xml:"FilterRule"`
}

type s3NotifyFilterRuleXML struct {
	Name  string `xml:"Name"`
	Value string `xml:"Value"`
}

func (s *Server) s3PutBucketNotificationConfiguration(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	bucket string,
) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "PutBucketNotificationConfiguration")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3PutBucketNotification, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "PutBucketNotificationConfiguration")
		return
	}
	var req s3NotificationConfigurationXML
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "MalformedXML",
			"The XML you provided was not well-formed or did not validate against our published schema.", "PutBucketNotificationConfiguration")
		return
	}
	cfg, err := s3NotificationConfigFromXML(req)
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
			err.Error(), "PutBucketNotificationConfiguration")
		return
	}
	if err := s.store.PutBucketNotificationConfiguration(ref.accountID, bucket, cfg); err != nil {
		switch {
		case errors.Is(err, store.ErrNoSuchBucket):
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
				"The specified bucket does not exist", "PutBucketNotificationConfiguration")
		case errors.Is(err, store.ErrInvalidS3NotificationConfiguration):
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidArgument",
				err.Error(), "PutBucketNotificationConfiguration")
		default:
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", "PutBucketNotificationConfiguration")
		}
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "PutBucketNotificationConfiguration", readOnly)
}

func (s *Server) s3GetBucketNotificationConfiguration(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	bucket string,
) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "GetBucketNotificationConfiguration")
	if !ok {
		return
	}
	if !s.authorizeS3(verified, catalog.ActionS3GetBucketNotification, store.BucketARN(bucket), ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "GetBucketNotificationConfiguration")
		return
	}
	cfg, err := s.store.GetBucketNotificationConfiguration(ref.accountID, bucket)
	if errors.Is(err, store.ErrNoSuchBucket) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
			"The specified bucket does not exist", "GetBucketNotificationConfiguration")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "GetBucketNotificationConfiguration")
		return
	}
	out := s3NotificationConfigToXML(cfg)
	out.XMLNS = s3XMLNS
	s.writeS3XML(w, requestID, http.StatusOK, out)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "GetBucketNotificationConfiguration", readOnly)
}

func s3NotificationConfigFromXML(req s3NotificationConfigurationXML) (store.S3NotificationConfig, error) {
	cfg := store.S3NotificationConfig{
		EventBridgeEnabled: req.EventBridgeConfiguration != nil,
	}
	for _, lc := range req.LambdaFunctionConfigurations {
		arn := strings.TrimSpace(lc.LambdaFunctionArn)
		if arn == "" {
			arn = strings.TrimSpace(lc.CloudFunction)
		}
		prefix, suffix := s3NotifyFilterParts(lc.Filter)
		cfg.LambdaConfigs = append(cfg.LambdaConfigs, store.S3LambdaFunctionConfig{
			ID:           strings.TrimSpace(lc.ID),
			Events:       lc.Events,
			FilterPrefix: prefix,
			FilterSuffix: suffix,
			FunctionARN:  arn,
		})
	}
	for _, cc := range req.CloudFunctionConfigurations {
		prefix, suffix := s3NotifyFilterParts(cc.Filter)
		cfg.LambdaConfigs = append(cfg.LambdaConfigs, store.S3LambdaFunctionConfig{
			ID:           strings.TrimSpace(cc.ID),
			Events:       cc.Events,
			FilterPrefix: prefix,
			FilterSuffix: suffix,
			FunctionARN:  strings.TrimSpace(cc.CloudFunction),
		})
	}
	for _, qc := range req.QueueConfigurations {
		prefix, suffix := s3NotifyFilterParts(qc.Filter)
		cfg.QueueConfigs = append(cfg.QueueConfigs, store.S3QueueConfig{
			ID:           strings.TrimSpace(qc.ID),
			Events:       qc.Events,
			FilterPrefix: prefix,
			FilterSuffix: suffix,
			QueueARN:     strings.TrimSpace(qc.Queue),
		})
	}
	for _, tc := range req.TopicConfigurations {
		prefix, suffix := s3NotifyFilterParts(tc.Filter)
		cfg.TopicConfigs = append(cfg.TopicConfigs, store.S3TopicConfig{
			ID:           strings.TrimSpace(tc.ID),
			Events:       tc.Events,
			FilterPrefix: prefix,
			FilterSuffix: suffix,
			TopicARN:     strings.TrimSpace(tc.Topic),
		})
	}
	return cfg, nil
}

func s3NotificationConfigToXML(cfg store.S3NotificationConfig) s3NotificationConfigurationXML {
	out := s3NotificationConfigurationXML{}
	for _, lc := range cfg.LambdaConfigs {
		out.LambdaFunctionConfigurations = append(out.LambdaFunctionConfigurations, s3LambdaFunctionConfigurationXML{
			ID:                lc.ID,
			LambdaFunctionArn: lc.FunctionARN,
			Events:            lc.Events,
			Filter:            s3NotifyFilterXMLFromParts(lc.FilterPrefix, lc.FilterSuffix),
		})
	}
	for _, qc := range cfg.QueueConfigs {
		out.QueueConfigurations = append(out.QueueConfigurations, s3QueueConfigurationXML{
			ID:     qc.ID,
			Queue:  qc.QueueARN,
			Events: qc.Events,
			Filter: s3NotifyFilterXMLFromParts(qc.FilterPrefix, qc.FilterSuffix),
		})
	}
	for _, tc := range cfg.TopicConfigs {
		out.TopicConfigurations = append(out.TopicConfigurations, s3TopicConfigurationXML{
			ID:     tc.ID,
			Topic:  tc.TopicARN,
			Events: tc.Events,
			Filter: s3NotifyFilterXMLFromParts(tc.FilterPrefix, tc.FilterSuffix),
		})
	}
	if cfg.EventBridgeEnabled {
		out.EventBridgeConfiguration = &s3EventBridgeConfigurationXML{}
	}
	return out
}

func s3NotifyFilterParts(filter *s3NotifyFilterXML) (prefix, suffix string) {
	if filter == nil {
		return "", ""
	}
	for _, rule := range filter.S3Key.FilterRules {
		switch strings.ToLower(strings.TrimSpace(rule.Name)) {
		case "prefix":
			prefix = rule.Value
		case "suffix":
			suffix = rule.Value
		}
	}
	return prefix, suffix
}

func s3NotifyFilterXMLFromParts(prefix, suffix string) *s3NotifyFilterXML {
	if prefix == "" && suffix == "" {
		return nil
	}
	var rules []s3NotifyFilterRuleXML
	if prefix != "" {
		rules = append(rules, s3NotifyFilterRuleXML{Name: "prefix", Value: prefix})
	}
	if suffix != "" {
		rules = append(rules, s3NotifyFilterRuleXML{Name: "suffix", Value: suffix})
	}
	return &s3NotifyFilterXML{S3Key: s3NotifyS3KeyXML{FilterRules: rules}}
}
