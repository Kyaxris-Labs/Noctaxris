package server

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// mqttRepublish holds the live MQTT bridge publish callback (optional).
type mqttRepublish struct {
	mu sync.Mutex
	fn func(topic string, payload []byte) error
}

var sharedMQTTRepublish mqttRepublish

func setMQTTRepublish(fn func(topic string, payload []byte) error) {
	sharedMQTTRepublish.mu.Lock()
	defer sharedMQTTRepublish.mu.Unlock()
	sharedMQTTRepublish.fn = fn
}

func mqttRepublishTopic(topic string, payload []byte) {
	sharedMQTTRepublish.mu.Lock()
	fn := sharedMQTTRepublish.fn
	sharedMQTTRepublish.mu.Unlock()
	if fn == nil {
		return
	}
	if err := fn(topic, payload); err != nil {
		log.Printf("iot rules republish mqtt: %v", err)
	}
}

// PublishTopic evaluates enabled topic rules for account/region against topic
// and dispatches matching actions in-process. Used by tests and MQTT non-shadow publishes.
// evaluateRules=false skips rule matching (used by republish to avoid loops).
func (s *Server) PublishTopic(accountID, region, topic string, payload []byte, evaluateRules bool) {
	if s == nil || s.store == nil {
		return
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return
	}
	if payload == nil {
		payload = []byte{}
	}
	if !evaluateRules {
		mqttRepublishTopic(topic, payload)
		return
	}
	rules, err := s.store.ListIoTTopicRules(accountID, region)
	if err != nil {
		log.Printf("iot rules list: %v", err)
		return
	}
	for _, rule := range rules {
		if rule.RuleDisabled {
			continue
		}
		filter := store.ExtractIoTTopicFilter(rule.SQL)
		if !store.IoTTopicMatches(filter, topic) {
			continue
		}
		s.executeIoTTopicRule(rule, topic, payload)
	}
}

// DispatchMQTTPublish runs region-wide enabled rules for a non-shadow MQTT publish.
func (s *Server) DispatchMQTTPublish(topic string, payload []byte) {
	if s == nil || s.store == nil {
		return
	}
	topic = strings.TrimSpace(topic)
	if topic == "" || strings.HasPrefix(topic, "$aws/") {
		return
	}
	rules, err := s.store.ListEnabledIoTTopicRulesByRegion(store.DefaultIoTRegion)
	if err != nil {
		log.Printf("iot rules mqtt list: %v", err)
		return
	}
	if payload == nil {
		payload = []byte{}
	}
	for _, rule := range rules {
		filter := store.ExtractIoTTopicFilter(rule.SQL)
		if !store.IoTTopicMatches(filter, topic) {
			continue
		}
		s.executeIoTTopicRule(rule, topic, payload)
	}
}

func (s *Server) executeIoTTopicRule(rule store.IoTTopicRule, topic string, payload []byte) {
	var actions []map[string]any
	if err := json.Unmarshal([]byte(rule.ActionsJSON), &actions); err != nil {
		log.Printf("iot rules %s: invalid actions JSON: %v", rule.RuleName, err)
		return
	}
	for _, action := range actions {
		s.dispatchIoTRuleAction(rule, topic, payload, action)
	}
}

func (s *Server) dispatchIoTRuleAction(rule store.IoTTopicRule, topic string, payload []byte, action map[string]any) {
	if republish, ok := action["republish"].(map[string]any); ok {
		target, _ := republish["topic"].(string)
		target = strings.TrimSpace(target)
		if target == "" {
			log.Printf("iot rules %s: republish skip (missing topic)", rule.RuleName)
			return
		}
		s.PublishTopic(rule.AccountID, rule.Region, target, payload, false)
		return
	}
	if sqs, ok := action["sqs"].(map[string]any); ok {
		s.dispatchIoTRuleSQS(rule, payload, sqs)
		return
	}
	if sns, ok := action["sns"].(map[string]any); ok {
		s.dispatchIoTRuleSNS(rule, payload, sns)
		return
	}
	if s3a, ok := action["s3"].(map[string]any); ok {
		s.dispatchIoTRuleS3(rule, payload, s3a)
		return
	}
	if ddb, ok := action["dynamoDB"].(map[string]any); ok {
		s.dispatchIoTRuleDynamo(rule, topic, payload, ddb)
		return
	}
	if ddbv2, ok := action["dynamoDBv2"].(map[string]any); ok {
		putItem, _ := ddbv2["putItem"].(map[string]any)
		s.dispatchIoTRuleDynamo(rule, topic, payload, putItem)
		return
	}
	if kin, ok := action["kinesis"].(map[string]any); ok {
		s.dispatchIoTRuleKinesis(rule, payload, kin)
		return
	}
	if lam, ok := action["lambda"].(map[string]any); ok {
		s.dispatchIoTRuleLambda(rule, payload, lam)
		return
	}
	log.Printf("iot rules %s: unsupported or empty action skipped", rule.RuleName)
}

func (s *Server) dispatchIoTRuleSQS(rule store.IoTTopicRule, payload []byte, sqs map[string]any) {
	queueURL, _ := sqs["queueUrl"].(string)
	queueURL = strings.TrimSpace(queueURL)
	if queueURL == "" {
		log.Printf("iot rules %s: sqs skip (missing queueUrl)", rule.RuleName)
		return
	}
	q, err := s.store.GetQueueByURL(queueURL)
	if err != nil {
		log.Printf("iot rules %s: sqs skip (queue missing): %v", rule.RuleName, err)
		return
	}
	body := payload
	if useB64, _ := sqs["useBase64"].(bool); useB64 {
		body = []byte(base64.StdEncoding.EncodeToString(payload))
	}
	if _, err := s.store.SendMessage(q.AccountID, q.QueueName, body, false, nil, "", nil); err != nil {
		log.Printf("iot rules %s: sqs send skip: %v", rule.RuleName, err)
	}
}

func (s *Server) dispatchIoTRuleSNS(rule store.IoTTopicRule, payload []byte, sns map[string]any) {
	targetARN, _ := sns["targetArn"].(string)
	if targetARN == "" {
		targetARN, _ = sns["topicArn"].(string)
	}
	targetARN = strings.TrimSpace(targetARN)
	if targetARN == "" {
		log.Printf("iot rules %s: sns skip (missing topicArn)", rule.RuleName)
		return
	}
	topic, err := s.store.GetTopicByARN(targetARN)
	if err != nil {
		log.Printf("iot rules %s: sns skip (topic missing): %v", rule.RuleName, err)
		return
	}
	if _, err := s.store.Publish(topic.AccountID, topic.TopicName, string(payload), "", nil); err != nil {
		log.Printf("iot rules %s: sns publish skip: %v", rule.RuleName, err)
	}
}

func (s *Server) dispatchIoTRuleS3(rule store.IoTTopicRule, payload []byte, s3a map[string]any) {
	bucket, _ := s3a["bucketName"].(string)
	if bucket == "" {
		bucket, _ = s3a["bucket"].(string)
	}
	key, _ := s3a["key"].(string)
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		log.Printf("iot rules %s: s3 skip (bucket/key required)", rule.RuleName)
		return
	}
	if _, err := s.store.GetBucket(rule.AccountID, bucket); err != nil {
		log.Printf("iot rules %s: s3 skip (bucket missing): %v", rule.RuleName, err)
		return
	}
	if _, err := s.store.PutObject(rule.AccountID, bucket, key, store.PutObjectMeta{
		Data:        payload,
		PlainSize:   int64(len(payload)),
		ContentType: "application/octet-stream",
	}); err != nil {
		log.Printf("iot rules %s: s3 put skip: %v", rule.RuleName, err)
	}
}

func (s *Server) dispatchIoTRuleDynamo(rule store.IoTTopicRule, topic string, payload []byte, ddb map[string]any) {
	if ddb == nil {
		log.Printf("iot rules %s: dynamoDB skip (empty action)", rule.RuleName)
		return
	}
	tableName, _ := ddb["tableName"].(string)
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		log.Printf("iot rules %s: dynamoDB skip (missing tableName)", rule.RuleName)
		return
	}
	if _, err := s.store.DescribeTable(rule.AccountID, tableName); err != nil {
		log.Printf("iot rules %s: dynamoDB skip (table missing): %v", rule.RuleName, err)
		return
	}
	itemJSON, pk, err := iotPayloadToDynamoItem(payload, topic)
	if err != nil {
		log.Printf("iot rules %s: dynamoDB skip: %v", rule.RuleName, err)
		return
	}
	if err := s.store.PutItemBytes(rule.AccountID, tableName, pk, "", "", "", itemJSON, false, nil); err != nil {
		log.Printf("iot rules %s: dynamoDB put skip: %v", rule.RuleName, err)
	}
}

func (s *Server) dispatchIoTRuleKinesis(rule store.IoTTopicRule, payload []byte, kin map[string]any) {
	streamName, _ := kin["streamName"].(string)
	streamName = strings.TrimSpace(streamName)
	if streamName == "" {
		log.Printf("iot rules %s: kinesis skip (missing streamName)", rule.RuleName)
		return
	}
	partitionKey, _ := kin["partitionKey"].(string)
	if strings.TrimSpace(partitionKey) == "" {
		partitionKey = "noctaxris-iot"
	}
	if _, _, err := s.store.PutKinesisRecord(rule.AccountID, streamName, partitionKey, payload); err != nil {
		log.Printf("iot rules %s: kinesis skip: %v", rule.RuleName, err)
	}
}

func (s *Server) dispatchIoTRuleLambda(rule store.IoTTopicRule, payload []byte, lam map[string]any) {
	fnRef, _ := lam["functionArn"].(string)
	if fnRef == "" {
		fnRef, _ = lam["functionName"].(string)
	}
	fnRef = strings.TrimSpace(fnRef)
	if fnRef == "" {
		log.Printf("iot rules %s: lambda skip (missing functionName/Arn)", rule.RuleName)
		return
	}
	accountID, fnName, ok := store.ParseLambdaARNFromSFNResource(fnRef)
	if !ok || fnName == "" {
		log.Printf("iot rules %s: lambda skip (bad function ref)", rule.RuleName)
		return
	}
	if accountID == "" {
		accountID = rule.AccountID
	}
	name, qualifier := store.ParseFunctionQualifier(fnName)
	if _, err := s.store.GetFunction(accountID, name); err != nil {
		log.Printf("iot rules %s: lambda skip (function missing): %v", rule.RuleName, err)
		return
	}
	eventJSON := string(payload)
	if !json.Valid(payload) {
		b, _ := json.Marshal(map[string]any{"payload": eventJSON})
		eventJSON = string(b)
	}
	if _, err := s.store.EnqueueAsyncInvoke(accountID, name, qualifier, eventJSON); err != nil {
		log.Printf("iot rules %s: lambda enqueue skip: %v", rule.RuleName, err)
	}
}

func iotPayloadToDynamoItem(payload []byte, topic string) (itemJSON []byte, pk string, err error) {
	var doc map[string]any
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &doc); err != nil {
			return nil, "", err
		}
	}
	if doc == nil {
		doc = map[string]any{}
	}
	attrs := map[string]any{}
	for k, v := range doc {
		attrs[k] = iotToDynamoAttr(v)
	}
	pkVal := topic
	if id, ok := doc["id"].(string); ok && strings.TrimSpace(id) != "" {
		pkVal = id
	} else if topic == "" {
		pkVal = "iot"
	}
	if _, exists := attrs["id"]; !exists {
		attrs["id"] = map[string]any{"S": pkVal}
	}
	raw, err := json.Marshal(attrs)
	if err != nil {
		return nil, "", err
	}
	pkRaw, err := json.Marshal(map[string]any{"S": pkVal})
	if err != nil {
		return nil, "", err
	}
	return raw, string(pkRaw), nil
}

func iotToDynamoAttr(v any) map[string]any {
	switch t := v.(type) {
	case nil:
		return map[string]any{"NULL": true}
	case bool:
		return map[string]any{"BOOL": t}
	case float64:
		return map[string]any{"N": strconv.FormatFloat(t, 'f', -1, 64)}
	case json.Number:
		return map[string]any{"N": t.String()}
	case string:
		return map[string]any{"S": t}
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return map[string]any{"S": ""}
		}
		return map[string]any{"S": string(b)}
	}
}
