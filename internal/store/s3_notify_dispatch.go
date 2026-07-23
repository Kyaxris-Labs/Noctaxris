package store

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// emitS3EventNotifications fans out after a successful object mutation.
// Failures are logged and skipped (AWS best-effort lab shape); callers must not
// surface dispatch errors on Put/Delete/Complete paths.
func (s *Store) emitS3EventNotifications(accountID, region, bucket, key, eventName, versionID string, size int64, etag string) {
	if s == nil {
		return
	}
	accountID = strings.TrimSpace(accountID)
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	eventName = strings.TrimPrefix(strings.TrimSpace(eventName), "s3:")
	if accountID == "" || bucket == "" || key == "" || eventName == "" {
		return
	}
	if region == "" {
		region = DefaultEventsRegion
	}

	cfg, err := s.GetBucketNotificationConfiguration(accountID, bucket)
	if err != nil {
		log.Printf("s3 notify: get config account=%s bucket=%s err=%v", accountID, bucket, err)
		return
	}
	if isEmptyS3NotificationConfig(cfg) {
		return
	}

	bucketARN := BucketARN(bucket)
	for _, lc := range cfg.LambdaConfigs {
		if !s3EventMatches(lc.Events, eventName) || !s3KeyMatchesFilter(key, lc.FilterPrefix, lc.FilterSuffix) {
			continue
		}
		body, buildErr := buildS3NotificationRecordsJSON(accountID, region, bucket, bucketARN, key, eventName, versionID, size, etag, lc.ID)
		if buildErr != nil {
			log.Printf("s3 notify: build lambda event bucket=%s key=%s err=%v", bucket, key, buildErr)
			continue
		}
		s.deliverS3NotificationLambda(accountID, bucketARN, lc.FunctionARN, body)
	}
	for _, qc := range cfg.QueueConfigs {
		if !s3EventMatches(qc.Events, eventName) || !s3KeyMatchesFilter(key, qc.FilterPrefix, qc.FilterSuffix) {
			continue
		}
		body, buildErr := buildS3NotificationRecordsJSON(accountID, region, bucket, bucketARN, key, eventName, versionID, size, etag, qc.ID)
		if buildErr != nil {
			log.Printf("s3 notify: build queue event bucket=%s key=%s err=%v", bucket, key, buildErr)
			continue
		}
		s.deliverS3NotificationQueue(accountID, bucketARN, qc.QueueARN, body)
	}
	for _, tc := range cfg.TopicConfigs {
		if !s3EventMatches(tc.Events, eventName) || !s3KeyMatchesFilter(key, tc.FilterPrefix, tc.FilterSuffix) {
			continue
		}
		body, buildErr := buildS3NotificationRecordsJSON(accountID, region, bucket, bucketARN, key, eventName, versionID, size, etag, tc.ID)
		if buildErr != nil {
			log.Printf("s3 notify: build topic event bucket=%s key=%s err=%v", bucket, key, buildErr)
			continue
		}
		s.deliverS3NotificationTopic(accountID, bucketARN, tc.TopicARN, body)
	}
	if cfg.EventBridgeEnabled {
		s.deliverS3NotificationEventBridge(accountID, region, bucket, bucketARN, key, eventName, versionID, size, etag)
	}
}

func s3EventMatches(cfgEvents []string, eventName string) bool {
	eventName = strings.TrimPrefix(strings.TrimSpace(eventName), "s3:")
	if eventName == "" {
		return false
	}
	full := "s3:" + eventName
	for _, raw := range cfgEvents {
		ev := strings.TrimSpace(raw)
		if ev == "" {
			continue
		}
		if ev == full {
			return true
		}
		switch ev {
		case "s3:ObjectCreated:*":
			if strings.HasPrefix(eventName, "ObjectCreated:") {
				return true
			}
		case "s3:ObjectRemoved:*":
			if strings.HasPrefix(eventName, "ObjectRemoved:") {
				return true
			}
		}
	}
	return false
}

func s3KeyMatchesFilter(key, prefix, suffix string) bool {
	if prefix != "" && !strings.HasPrefix(key, prefix) {
		return false
	}
	if suffix != "" && !strings.HasSuffix(key, suffix) {
		return false
	}
	return true
}

func (s *Store) deliverS3NotificationLambda(bucketAccount, bucketARN, functionARN, body string) {
	if !s.s3NotificationDestinationAuthorized(bucketAccount, bucketARN, functionARN, actionLambdaInvokeFunction) {
		log.Printf("s3 notify: lambda authz denied target=%s source=%s", functionARN, bucketARN)
		return
	}
	owner := resourceOwnerAccountFromARN(functionARN)
	name, qualifier := ParseFunctionQualifier(functionARN)
	if owner == "" || name == "" {
		log.Printf("s3 notify: invalid lambda arn %s", functionARN)
		return
	}
	if _, err := s.EnqueueAsyncInvoke(owner, name, qualifier, body); err != nil {
		log.Printf("s3 notify: enqueue async invoke target=%s err=%v", functionARN, err)
	}
}

func (s *Store) deliverS3NotificationQueue(bucketAccount, bucketARN, queueARN, body string) {
	if !s.s3NotificationDestinationAuthorized(bucketAccount, bucketARN, queueARN, actionSQSSendMessage) {
		log.Printf("s3 notify: queue authz denied target=%s source=%s", queueARN, bucketARN)
		return
	}
	owner, err := queueAccountFromARN(queueARN)
	if err != nil {
		log.Printf("s3 notify: queue arn parse %s: %v", queueARN, err)
		return
	}
	name, err := queueNameFromARN(queueARN)
	if err != nil {
		log.Printf("s3 notify: queue name parse %s: %v", queueARN, err)
		return
	}
	if _, err := s.SendMessage(owner, name, []byte(body), false, nil, "", nil); err != nil {
		log.Printf("s3 notify: send message target=%s err=%v", queueARN, err)
	}
}

// deliverS3NotificationTopic uses in-process Publish only (HTTP subscribers stay
// on the existing SNS allowlist / catcher path; no new open egress).
func (s *Store) deliverS3NotificationTopic(bucketAccount, bucketARN, topicARN, body string) {
	if !s.s3NotificationDestinationAuthorized(bucketAccount, bucketARN, topicARN, actionSNSPublish) {
		log.Printf("s3 notify: topic authz denied target=%s source=%s", topicARN, bucketARN)
		return
	}
	topic, err := s.GetTopicByARN(topicARN)
	if err != nil {
		log.Printf("s3 notify: get topic %s: %v", topicARN, err)
		return
	}
	if _, err := s.Publish(topic.AccountID, topic.TopicName, body, "", nil); err != nil {
		log.Printf("s3 notify: publish topic=%s err=%v", topicARN, err)
	}
}

func (s *Store) deliverS3NotificationEventBridge(accountID, region, bucket, bucketARN, key, eventName, versionID string, size int64, etag string) {
	detailType, reason, ok := s3EventBridgeDetailType(eventName)
	if !ok {
		return
	}
	detail, err := buildS3EventBridgeDetailJSON(bucket, key, reason, versionID, size, etag)
	if err != nil {
		log.Printf("s3 notify: build eventbridge detail bucket=%s key=%s err=%v", bucket, key, err)
		return
	}
	_, err = s.PutEvents(accountID, []PutEventsEntry{{
		Source:       "aws.s3",
		DetailType:   detailType,
		Detail:       detail,
		EventBusName: DefaultEventBusName,
	}})
	if err != nil {
		log.Printf("s3 notify: put events bucket=%s key=%s err=%v", bucket, key, err)
		return
	}
	_ = region
	_ = bucketARN
}

func s3EventBridgeDetailType(eventName string) (detailType, reason string, ok bool) {
	switch {
	case strings.HasPrefix(eventName, "ObjectCreated:"):
		reason = strings.TrimPrefix(eventName, "ObjectCreated:")
		switch reason {
		case "Put":
			reason = "PutObject"
		case "CompleteMultipartUpload":
			reason = "CompleteMultipartUpload"
		}
		return "Object Created", reason, true
	case strings.HasPrefix(eventName, "ObjectRemoved:"):
		reason = strings.TrimPrefix(eventName, "ObjectRemoved:")
		switch reason {
		case "Delete":
			reason = "DeleteObject"
		}
		return "Object Deleted", reason, true
	default:
		return "", "", false
	}
}

func buildS3EventBridgeDetailJSON(bucket, key, reason, versionID string, size int64, etag string) (string, error) {
	obj := map[string]any{
		"key": key,
		"size": size,
		"etag": etag,
	}
	if versionID != "" {
		obj["version-id"] = versionID
	}
	detail := map[string]any{
		"version": "0",
		"bucket":  map[string]any{"name": bucket},
		"object":  obj,
		"reason":  reason,
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func buildS3NotificationRecordsJSON(accountID, region, bucket, bucketARN, key, eventName, versionID string, size int64, etag, configID string) (string, error) {
	if configID == "" {
		configID = "default"
	}
	obj := map[string]any{
		"key":       s3NotificationEncodeKey(key),
		"size":      size,
		"eTag":      etag,
		"sequencer": fmt.Sprintf("%016X", time.Now().UTC().UnixNano()),
	}
	if versionID != "" {
		obj["versionId"] = versionID
	}
	record := map[string]any{
		"eventVersion": "2.1",
		"eventSource":  "aws:s3",
		"awsRegion":    region,
		"eventTime":    time.Now().UTC().Format(time.RFC3339),
		"eventName":    eventName,
		"userIdentity": map[string]any{
			"principalId": accountID,
		},
		"requestParameters": map[string]any{
			"sourceIPAddress": "127.0.0.1",
		},
		"responseElements": map[string]any{
			"x-amz-request-id": uuid.NewString(),
			"x-amz-id-2":       uuid.NewString(),
		},
		"s3": map[string]any{
			"s3SchemaVersion": "1.0",
			"configurationId": configID,
			"bucket": map[string]any{
				"name": bucket,
				"ownerIdentity": map[string]any{
					"principalId": accountID,
				},
				"arn": bucketARN,
			},
			"object": obj,
		},
	}
	envelope := map[string]any{"Records": []any{record}}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func s3NotificationEncodeKey(key string) string {
	// AWS notification keys are URL-encoded (spaces as %20, not '+').
	return strings.ReplaceAll(url.QueryEscape(key), "+", "%20")
}
