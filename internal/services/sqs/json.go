package sqs

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateQueueJSON builds a CreateQueue response.
func CreateQueueJSON(queueURL string) ([]byte, error) {
	return json.Marshal(map[string]string{"QueueUrl": queueURL})
}

// GetQueueURLJSON builds a GetQueueUrl response.
func GetQueueURLJSON(queueURL string) ([]byte, error) {
	return json.Marshal(map[string]string{"QueueUrl": queueURL})
}

// GetQueueAttributesJSON builds a GetQueueAttributes response.
func GetQueueAttributesJSON(attrs map[string]string) ([]byte, error) {
	if attrs == nil {
		attrs = map[string]string{}
	}
	return json.Marshal(map[string]any{"Attributes": attrs})
}

// ListQueuesJSON builds a ListQueues response.
func ListQueuesJSON(queues []store.Queue) ([]byte, error) {
	urls := make([]string, 0, len(queues))
	for _, q := range queues {
		urls = append(urls, q.QueueURL)
	}
	return json.Marshal(map[string]any{"QueueUrls": urls})
}

// EmptyOKJSON is the empty success body used by several SQS actions.
func EmptyOKJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// SendMessageJSON builds a SendMessage response. md5Attrs may be empty.
func SendMessageJSON(messageID, md5Body, md5Attrs string) ([]byte, error) {
	out := map[string]string{
		"MessageId":        messageID,
		"MD5OfMessageBody": md5Body,
	}
	if md5Attrs != "" {
		out["MD5OfMessageAttributes"] = md5Attrs
	}
	return json.Marshal(out)
}

// ReceivedMessage is one message entry in a ReceiveMessage response.
type ReceivedMessage struct {
	MessageID         string
	ReceiptHandle     string
	Body              string
	MD5OfBody         string
	MD5OfMessageAttrs string
	Attributes        map[string]string
	MessageAttributes map[string]any
}

// ReceiveMessageJSON builds a ReceiveMessage response.
func ReceiveMessageJSON(messages []ReceivedMessage) ([]byte, error) {
	entries := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		entry := map[string]any{
			"MessageId":     m.MessageID,
			"ReceiptHandle": m.ReceiptHandle,
			"MD5OfBody":     m.MD5OfBody,
			"Body":          m.Body,
		}
		if len(m.Attributes) > 0 {
			entry["Attributes"] = m.Attributes
		}
		if len(m.MessageAttributes) > 0 {
			entry["MessageAttributes"] = m.MessageAttributes
			if m.MD5OfMessageAttrs != "" {
				entry["MD5OfMessageAttributes"] = m.MD5OfMessageAttrs
			}
		}
		entries = append(entries, entry)
	}
	return json.Marshal(map[string]any{"Messages": entries})
}

// SendMessageBatchSuccess is one successful SendMessageBatch entry.
type SendMessageBatchSuccess struct {
	ID                    string
	MessageID             string
	MD5OfMessageBody      string
	MD5OfMessageAttrs     string
}

// BatchFailure is one failed batch entry.
type BatchFailure struct {
	ID          string
	Code        string
	Message     string
	SenderFault bool
}

// SendMessageBatchJSON builds a SendMessageBatch response.
func SendMessageBatchJSON(successful []SendMessageBatchSuccess, failed []BatchFailure) ([]byte, error) {
	ok := make([]map[string]any, 0, len(successful))
	for _, s := range successful {
		entry := map[string]any{
			"Id":               s.ID,
			"MessageId":        s.MessageID,
			"MD5OfMessageBody": s.MD5OfMessageBody,
		}
		if s.MD5OfMessageAttrs != "" {
			entry["MD5OfMessageAttributes"] = s.MD5OfMessageAttrs
		}
		ok = append(ok, entry)
	}
	fail := make([]map[string]any, 0, len(failed))
	for _, f := range failed {
		fail = append(fail, map[string]any{
			"Id":          f.ID,
			"Code":        f.Code,
			"Message":     f.Message,
			"SenderFault": f.SenderFault,
		})
	}
	return json.Marshal(map[string]any{
		"Successful": ok,
		"Failed":     fail,
	})
}

// DeleteMessageBatchJSON builds a DeleteMessageBatch response.
func DeleteMessageBatchJSON(successfulIDs []string, failed []BatchFailure) ([]byte, error) {
	ok := make([]map[string]string, 0, len(successfulIDs))
	for _, id := range successfulIDs {
		ok = append(ok, map[string]string{"Id": id})
	}
	fail := make([]map[string]any, 0, len(failed))
	for _, f := range failed {
		fail = append(fail, map[string]any{
			"Id":          f.ID,
			"Code":        f.Code,
			"Message":     f.Message,
			"SenderFault": f.SenderFault,
		})
	}
	return json.Marshal(map[string]any{
		"Successful": ok,
		"Failed":     fail,
	})
}

// MD5Hex returns the hex MD5 of s (SQS message body digest).
func MD5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// MD5OfMessageAttributesHex digests the canonical JSON of message attributes.
// Lab approximation: MD5 of the JSON object bytes when attributes are present.
func MD5OfMessageAttributesHex(attrs map[string]any) (string, error) {
	if len(attrs) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(attrs)
	if err != nil {
		return "", fmt.Errorf("marshal message attributes: %w", err)
	}
	sum := md5.Sum(raw)
	return hex.EncodeToString(sum[:]), nil
}

// FilterAttributes returns attrs filtered by names. "All" returns a copy of attrs.
func FilterAttributes(attrs map[string]string, names []string) map[string]string {
	if attrs == nil {
		attrs = map[string]string{}
	}
	if len(names) == 0 {
		return map[string]string{}
	}
	for _, n := range names {
		if n == "All" {
			out := make(map[string]string, len(attrs))
			for k, v := range attrs {
				out[k] = v
			}
			return out
		}
	}
	out := map[string]string{}
	for _, n := range names {
		if v, ok := attrs[n]; ok {
			out[n] = v
		}
	}
	return out
}
