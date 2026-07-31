package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	kmssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/kms"
	sqssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/sqs"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func (s *Server) handleSQS(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	accountID := verified.AccountID
	region := verified.Region
	if region == "" {
		region = store.DefaultSQSRegion
	}

	resource := "*"
	queuePolicy := ""
	resourceAccountID := ""
	var (
		queue store.Queue
		err   error
	)

	switch action {
	case catalog.ActionSQSListQueues, "ListQueues":
		resource = "*"
	case catalog.ActionSQSCreateQueue, "CreateQueue":
		name, _ := params["QueueName"].(string)
		if strings.TrimSpace(name) == "" {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
				"QueueName is required.", readOnly, eventID, verified)
			return
		}
		attrs := stringMapParam(params["Attributes"])
		resource = store.QueueARN(region, accountID, name)
		queuePolicy = attrs["Policy"]
		resourceAccountID = accountID
	default:
		q, resolveErr := s.resolveSQSQueue(accountID, params)
		if resolveErr != nil {
			if errors.Is(resolveErr, store.ErrNoSuchQueue) {
				s.writeSQSError(w, r, requestID, http.StatusBadRequest, "AWS.SimpleQueueService.NonExistentQueue",
					"The specified queue does not exist.", readOnly, eventID, verified)
				return
			}
			if resolveErr.Error() == "missing queue" {
				s.writeSQSError(w, r, requestID, http.StatusBadRequest, "MissingParameter",
					"QueueUrl or QueueName is required.", readOnly, eventID, verified)
				return
			}
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
				"Unable to resolve queue.", readOnly, eventID, verified)
			return
		}
		queue = q
		resource = q.QueueARN
		resourceAccountID = q.AccountID
		if q.Attributes != nil {
			queuePolicy = q.Attributes["Policy"]
		}
	}

	if !s.authorizeSQS(verified, normalizeAction(action), resource, queuePolicy, resourceAccountID) {
		s.writeSQSError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform "+normalizeAction(action)+".", readOnly, eventID, verified)
		return
	}

	var payload []byte
	switch action {
	case catalog.ActionSQSCreateQueue, "CreateQueue":
		payload, err = s.sqsCreateQueue(r, verified, region, params)
		if errors.Is(err, store.ErrQueueAlreadyExists) {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "QueueNameExists",
				"A queue already exists with the same name and different attributes.", readOnly, eventID, verified)
			return
		}
		if errors.Is(err, store.ErrInvalidFIFOQueueName) {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
				"Can only include '.fifo' at the end of a FIFO queue name.", readOnly, eventID, verified)
			return
		}
		if err != nil {
			s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to create queue.", readOnly, eventID, verified)
			return
		}
	case catalog.ActionSQSGetQueueUrl, "GetQueueUrl":
		payload, err = sqssvc.GetQueueURLJSON(queue.QueueURL)
	case catalog.ActionSQSGetQueueAttributes, "GetQueueAttributes":
		payload, err = s.sqsGetQueueAttributes(queue, params)
	case catalog.ActionSQSSetQueueAttributes, "SetQueueAttributes":
		attrs := stringMapParam(params["Attributes"])
		if err := s.store.SetQueueAttributes(queue.AccountID, queue.QueueName, attrs); err != nil {
			if errors.Is(err, store.ErrInvalidFIFOQueueName) {
				s.writeSQSError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
					"Can only include '.fifo' at the end of a FIFO queue name.", readOnly, eventID, verified)
				return
			}
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "InvalidAttributeValue",
				"Unable to set queue attributes.", readOnly, eventID, verified)
			return
		}
		payload, err = sqssvc.EmptyOKJSON()
	case catalog.ActionSQSDeleteQueue, "DeleteQueue":
		if err := s.store.DeleteQueue(queue.AccountID, queue.QueueName); err != nil {
			if errors.Is(err, store.ErrNoSuchQueue) {
				s.writeSQSError(w, r, requestID, http.StatusBadRequest, "AWS.SimpleQueueService.NonExistentQueue",
					"The specified queue does not exist.", readOnly, eventID, verified)
				return
			}
			s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to delete queue.", readOnly, eventID, verified)
			return
		}
		payload, err = sqssvc.EmptyOKJSON()
	case catalog.ActionSQSListQueues, "ListQueues":
		prefix, _ := params["QueueNamePrefix"].(string)
		queues, listErr := s.store.ListQueues(accountID, prefix)
		if listErr != nil {
			s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list queues.", readOnly, eventID, verified)
			return
		}
		payload, err = sqssvc.ListQueuesJSON(queues)
	case catalog.ActionSQSPurgeQueue, "PurgeQueue":
		if err := s.store.PurgeQueue(queue.AccountID, queue.QueueName); err != nil {
			s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to purge queue.", readOnly, eventID, verified)
			return
		}
		payload, err = sqssvc.EmptyOKJSON()
	case catalog.ActionSQSSendMessage, "SendMessage":
		payload, err = s.sqsSendMessage(w, r, requestID, eventID, verified, readOnly, queue, params)
		if err != nil || payload == nil {
			return
		}
	case catalog.ActionSQSReceiveMessage, "ReceiveMessage":
		payload, err = s.sqsReceiveMessage(w, r, requestID, eventID, verified, readOnly, queue, params)
		if err != nil || payload == nil {
			return
		}
	case catalog.ActionSQSDeleteMessage, "DeleteMessage":
		handle, _ := params["ReceiptHandle"].(string)
		if handle == "" {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "MissingParameter",
				"ReceiptHandle is required.", readOnly, eventID, verified)
			return
		}
		if err := s.store.DeleteMessage(queue.AccountID, queue.QueueName, handle); err != nil {
			if errors.Is(err, store.ErrNoSuchMessage) {
				s.writeSQSError(w, r, requestID, http.StatusBadRequest, "ReceiptHandleIsInvalid",
					"The receipt handle is invalid.", readOnly, eventID, verified)
				return
			}
			s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to delete message.", readOnly, eventID, verified)
			return
		}
		payload, err = sqssvc.EmptyOKJSON()
	case catalog.ActionSQSSendMessageBatch, "SendMessageBatch":
		payload, err = s.sqsSendMessageBatch(w, r, requestID, eventID, verified, readOnly, queue, params)
		if err != nil || payload == nil {
			return
		}
	case catalog.ActionSQSDeleteMessageBatch, "DeleteMessageBatch":
		payload, err = s.sqsDeleteMessageBatch(w, r, requestID, eventID, verified, readOnly, queue, params)
		if err != nil || payload == nil {
			return
		}
	case catalog.ActionSQSChangeMessageVisibility, "ChangeMessageVisibility":
		handle, _ := params["ReceiptHandle"].(string)
		if handle == "" {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "MissingParameter",
				"ReceiptHandle is required.", readOnly, eventID, verified)
			return
		}
		timeout := intParam(params["VisibilityTimeout"], 0)
		if err := s.store.ChangeMessageVisibility(queue.AccountID, queue.QueueName, handle, timeout); err != nil {
			if errors.Is(err, store.ErrNoSuchMessage) {
				s.writeSQSError(w, r, requestID, http.StatusBadRequest, "ReceiptHandleIsInvalid",
					"The receipt handle is invalid.", readOnly, eventID, verified)
				return
			}
			s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to change message visibility.", readOnly, eventID, verified)
			return
		}
		payload, err = sqssvc.EmptyOKJSON()
	case catalog.ActionSQSListQueueTags, "ListQueueTags":
		tags, listErr := s.store.ListResourceTags(accountID, queue.QueueARN)
		if listErr != nil {
			s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list queue tags.", readOnly, eventID, verified)
			return
		}
		payload, err = sqssvc.ListQueueTagsJSON(resourceTagsToMap(tags))
	case catalog.ActionSQSTagQueue, "TagQueue":
		tags := parseStringMapTags(params["Tags"])
		if len(tags) == 0 {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "MissingParameter",
				"Tags is required.", readOnly, eventID, verified)
			return
		}
		if _, tagErr := s.store.TagResources(accountID, []string{queue.QueueARN}, tags); tagErr != nil {
			s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to tag queue.", readOnly, eventID, verified)
			return
		}
		payload, err = sqssvc.EmptyOKJSON()
	case catalog.ActionSQSUntagQueue, "UntagQueue":
		keys := stringSliceParam(params["TagKeys"])
		if len(keys) == 0 {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "MissingParameter",
				"TagKeys is required.", readOnly, eventID, verified)
			return
		}
		if _, untagErr := s.store.UntagResources(accountID, []string{queue.QueueARN}, keys); untagErr != nil {
			s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to untag queue.", readOnly, eventID, verified)
			return
		}
		payload, err = sqssvc.EmptyOKJSON()
	default:
		s.writeSQSError(w, r, requestID, http.StatusNotImplemented, "NotImplemented",
			"This SQS action is not implemented.", readOnly, eventID, verified)
		return
	}

	if err != nil {
		s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSQSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, "sqs.amazonaws.com", eventNameForRequest(r), readOnly)
}

func (s *Server) authorizeSQS(verified *authn.Verified, action, resource, queuePolicy, resourceAccountID string) bool {
	return s.authorizeDataplaneOR(verified, action, resource, resourceAccountID, func(caller authz.RequestContext, identityDocs []string, resourceAccountID string) authz.Decision {
		return authz.EvaluateSQS(authz.SQSRequest{
			Caller:            caller,
			IdentityDocs:      identityDocs,
			QueuePolicyDoc:    queuePolicy,
			ResourceAccountID: resourceAccountID,
		})
	})
}

func (s *Server) resolveSQSQueue(accountID string, params map[string]any) (store.Queue, error) {
	if url, _ := params["QueueUrl"].(string); strings.TrimSpace(url) != "" {
		return s.store.GetQueueByURL(strings.TrimSpace(url))
	}
	if name, _ := params["QueueName"].(string); strings.TrimSpace(name) != "" {
		return s.store.GetQueue(accountID, strings.TrimSpace(name))
	}
	return store.Queue{}, errors.New("missing queue")
}

func sqsEndpointHost(r *http.Request) string {
	host := r.Host
	if host == "" {
		host = "127.0.0.1:4566"
	}
	if r.TLS != nil {
		return "https://" + host
	}
	return "http://" + host
}

func (s *Server) sqsCreateQueue(r *http.Request, verified *authn.Verified, region string, params map[string]any) ([]byte, error) {
	name, _ := params["QueueName"].(string)
	attrs := stringMapParam(params["Attributes"])
	q, err := s.store.CreateQueue(verified.AccountID, region, sqsEndpointHost(r), name, attrs)
	if err != nil {
		return nil, err
	}
	return sqssvc.CreateQueueJSON(q.QueueURL)
}

func (s *Server) sqsGetQueueAttributes(queue store.Queue, params map[string]any) ([]byte, error) {
	attrs := map[string]string{}
	for k, v := range queue.Attributes {
		attrs[k] = v
	}
	if _, ok := attrs["QueueArn"]; !ok {
		attrs["QueueArn"] = queue.QueueARN
	}
	names := stringSliceParam(params["AttributeNames"])
	if len(names) == 0 {
		// AWS returns empty results when AttributeNames is omitted.
		return sqssvc.GetQueueAttributesJSON(map[string]string{})
	}
	return sqssvc.GetQueueAttributesJSON(sqssvc.FilterAttributes(attrs, names))
}

func (s *Server) sqsSendMessage(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	queue store.Queue,
	params map[string]any,
) ([]byte, error) {
	body, _ := params["MessageBody"].(string)
	if body == "" {
		s.writeSQSError(w, r, requestID, http.StatusBadRequest, "MissingParameter",
			"MessageBody is required.", readOnly, eventID, verified)
		return nil, nil
	}
	msgAttrs := anyMapParam(params["MessageAttributes"])
	attrsJSON := "{}"
	md5Attrs := ""
	if len(msgAttrs) > 0 {
		raw, err := json.Marshal(msgAttrs)
		if err != nil {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
				"Invalid MessageAttributes.", readOnly, eventID, verified)
			return nil, nil
		}
		attrsJSON = string(raw)
		md5Attrs, err = sqssvc.MD5OfMessageAttributesHex(msgAttrs)
		if err != nil {
			s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to digest message attributes.", readOnly, eventID, verified)
			return nil, nil
		}
	}

	storedBody, sealed, sealedDEK, encErr := s.encryptSQSBody(verified, queue.Attributes, []byte(body))
	if encErr != nil {
		s.writeSQSEncryptError(w, r, requestID, eventID, verified, readOnly, encErr)
		return nil, nil
	}
	sendOpts := sqsSendOptsFromParams(queue.Attributes, params)
	msg, err := s.store.SendMessage(queue.AccountID, queue.QueueName, storedBody, sealed, sealedDEK, attrsJSON, sendOpts)
	if err != nil {
		if errors.Is(err, store.ErrNoSuchQueue) {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "AWS.SimpleQueueService.NonExistentQueue",
				"The specified queue does not exist.", readOnly, eventID, verified)
			return nil, nil
		}
		if errors.Is(err, store.ErrMissingMessageGroupID) {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "MissingParameter",
				"The request must contain the parameter MessageGroupId.", readOnly, eventID, verified)
			return nil, nil
		}
		if errors.Is(err, store.ErrMissingMessageDeduplication) {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
				"The queue should either have ContentBasedDeduplication enabled or MessageDeduplicationId provided explicitly.", readOnly, eventID, verified)
			return nil, nil
		}
		s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to send message.", readOnly, eventID, verified)
		return nil, nil
	}
	return sqssvc.SendMessageJSON(msg.MessageID, sqssvc.MD5Hex(body), md5Attrs, msg.SequenceNumber)
}

func (s *Server) sqsReceiveMessage(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	queue store.Queue,
	params map[string]any,
) ([]byte, error) {
	max := intParam(params["MaxNumberOfMessages"], 1)
	if max < 1 {
		max = 1
	}
	if max > 10 {
		max = 10
	}
	waitSec := intParam(params["WaitTimeSeconds"], 0)
	if waitSec < 0 {
		waitSec = 0
	}
	if waitSec > 20 {
		waitSec = 20
	}
	deadline := time.Now().Add(time.Duration(waitSec) * time.Second)
	var msgs []store.Message
	for {
		var err error
		msgs, err = s.store.ReceiveMessages(queue.AccountID, queue.QueueName, max)
		if err != nil {
			if errors.Is(err, store.ErrNoSuchQueue) {
				s.writeSQSError(w, r, requestID, http.StatusBadRequest, "AWS.SimpleQueueService.NonExistentQueue",
					"The specified queue does not exist.", readOnly, eventID, verified)
				return nil, nil
			}
			s.writeSQSError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to receive messages.", readOnly, eventID, verified)
			return nil, nil
		}
		if len(msgs) > 0 || waitSec == 0 || !time.Now().Before(deadline) {
			break
		}
		sleep := 100 * time.Millisecond
		if remaining := time.Until(deadline); remaining < sleep {
			sleep = remaining
		}
		if sleep <= 0 {
			break
		}
		select {
		case <-r.Context().Done():
			return sqssvc.ReceiveMessageJSON([]sqssvc.ReceivedMessage{})
		case <-time.After(sleep):
		}
	}

	out := make([]sqssvc.ReceivedMessage, 0, len(msgs))
	for _, m := range msgs {
		plain, decErr := s.decryptSQSBody(verified, queue.Attributes, m)
		if decErr != nil {
			s.writeSQSEncryptError(w, r, requestID, eventID, verified, readOnly, decErr)
			return nil, nil
		}
		bodyStr := string(plain)
		entry := sqssvc.ReceivedMessage{
			MessageID:              m.MessageID,
			ReceiptHandle:          m.ReceiptHandle,
			Body:                   bodyStr,
			MD5OfBody:              sqssvc.MD5Hex(bodyStr),
			MessageGroupID:         m.MessageGroupID,
			MessageDeduplicationID: m.MessageDeduplicationID,
			SequenceNumber:         m.SequenceNumber,
			Attributes: map[string]string{
				"ApproximateReceiveCount": strconv.Itoa(m.ReceiveCount),
				"SentTimestamp":           m.CreatedAt,
			},
		}
		if strings.TrimSpace(m.AttributesJSON) != "" && m.AttributesJSON != "{}" {
			var msgAttrs map[string]any
			if json.Unmarshal([]byte(m.AttributesJSON), &msgAttrs) == nil && len(msgAttrs) > 0 {
				entry.MessageAttributes = msgAttrs
				if digest, digErr := sqssvc.MD5OfMessageAttributesHex(msgAttrs); digErr == nil {
					entry.MD5OfMessageAttrs = digest
				}
			}
		}
		out = append(out, entry)
	}
	return sqssvc.ReceiveMessageJSON(out)
}

func (s *Server) sqsSendMessageBatch(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	queue store.Queue,
	params map[string]any,
) ([]byte, error) {
	entries, ok := params["Entries"].([]any)
	if !ok || len(entries) == 0 {
		s.writeSQSError(w, r, requestID, http.StatusBadRequest, "EmptyBatchRequest",
			"There should be at least one SendMessageBatch entry.", readOnly, eventID, verified)
		return nil, nil
	}
	if len(entries) > 10 {
		s.writeSQSError(w, r, requestID, http.StatusBadRequest, "TooManyEntriesInBatchRequest",
			"Maximum number of entries per request is 10.", readOnly, eventID, verified)
		return nil, nil
	}

	var successful []sqssvc.SendMessageBatchSuccess
	var failed []sqssvc.BatchFailure
	seen := map[string]struct{}{}

	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := entry["Id"].(string)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "BatchEntryIdsNotDistinct",
				"Batch entry IDs must be distinct.", readOnly, eventID, verified)
			return nil, nil
		}
		seen[id] = struct{}{}

		body, _ := entry["MessageBody"].(string)
		if body == "" {
			failed = append(failed, sqssvc.BatchFailure{
				ID: id, Code: "InvalidParameterValue", Message: "MessageBody is required.", SenderFault: true,
			})
			continue
		}
		msgAttrs := anyMapParam(entry["MessageAttributes"])
		attrsJSON := "{}"
		md5Attrs := ""
		if len(msgAttrs) > 0 {
			rawAttrs, err := json.Marshal(msgAttrs)
			if err != nil {
				failed = append(failed, sqssvc.BatchFailure{
					ID: id, Code: "InvalidParameterValue", Message: "Invalid MessageAttributes.", SenderFault: true,
				})
				continue
			}
			attrsJSON = string(rawAttrs)
			md5Attrs, _ = sqssvc.MD5OfMessageAttributesHex(msgAttrs)
		}
		storedBody, sealed, sealedDEK, encErr := s.encryptSQSBody(verified, queue.Attributes, []byte(body))
		if encErr != nil {
			failed = append(failed, sqssvc.BatchFailure{
				ID: id, Code: "KmsAccessDenied", Message: encErr.Error(), SenderFault: true,
			})
			continue
		}
		msg, err := s.store.SendMessage(queue.AccountID, queue.QueueName, storedBody, sealed, sealedDEK, attrsJSON, sqsSendOptsFromEntry(queue.Attributes, entry))
		if err != nil {
			failed = append(failed, sqssvc.BatchFailure{
				ID: id, Code: "InternalError", Message: "Unable to send message.", SenderFault: false,
			})
			continue
		}
		successful = append(successful, sqssvc.SendMessageBatchSuccess{
			ID:                id,
			MessageID:         msg.MessageID,
			MD5OfMessageBody:  sqssvc.MD5Hex(body),
			MD5OfMessageAttrs: md5Attrs,
		})
	}
	return sqssvc.SendMessageBatchJSON(successful, failed)
}

func (s *Server) sqsDeleteMessageBatch(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	queue store.Queue,
	params map[string]any,
) ([]byte, error) {
	entries, ok := params["Entries"].([]any)
	if !ok || len(entries) == 0 {
		s.writeSQSError(w, r, requestID, http.StatusBadRequest, "EmptyBatchRequest",
			"There should be at least one DeleteMessageBatch entry.", readOnly, eventID, verified)
		return nil, nil
	}
	if len(entries) > 10 {
		s.writeSQSError(w, r, requestID, http.StatusBadRequest, "TooManyEntriesInBatchRequest",
			"Maximum number of entries per request is 10.", readOnly, eventID, verified)
		return nil, nil
	}

	var successful []string
	var failed []sqssvc.BatchFailure
	seen := map[string]struct{}{}

	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := entry["Id"].(string)
		handle, _ := entry["ReceiptHandle"].(string)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			s.writeSQSError(w, r, requestID, http.StatusBadRequest, "BatchEntryIdsNotDistinct",
				"Batch entry IDs must be distinct.", readOnly, eventID, verified)
			return nil, nil
		}
		seen[id] = struct{}{}
		if handle == "" {
			failed = append(failed, sqssvc.BatchFailure{
				ID: id, Code: "InvalidParameterValue", Message: "ReceiptHandle is required.", SenderFault: true,
			})
			continue
		}
		if err := s.store.DeleteMessage(queue.AccountID, queue.QueueName, handle); err != nil {
			code := "InternalError"
			msg := "Unable to delete message."
			sender := false
			if errors.Is(err, store.ErrNoSuchMessage) {
				code = "ReceiptHandleIsInvalid"
				msg = "The receipt handle is invalid."
				sender = true
			}
			failed = append(failed, sqssvc.BatchFailure{ID: id, Code: code, Message: msg, SenderFault: sender})
			continue
		}
		successful = append(successful, id)
	}
	return sqssvc.DeleteMessageBatchJSON(successful, failed)
}

var errSQSAccessDenied = errors.New("sqs access denied")

func (s *Server) encryptSQSBody(verified *authn.Verified, attrs map[string]string, plain []byte) ([]byte, bool, []byte, error) {
	switch sqssvc.ModeFromAttributes(attrs) {
	case sqssvc.SSENone:
		return plain, false, nil, nil
	case sqssvc.SSESQS:
		dek, err := sqssvc.NewRandomDEK()
		if err != nil {
			return nil, false, nil, err
		}
		sealedDEK, err := s.store.SealWithMaster(dek)
		if err != nil {
			return nil, false, nil, err
		}
		ct, err := sqssvc.EncryptAES256GCM(dek, plain)
		if err != nil {
			return nil, false, nil, err
		}
		return ct, true, sealedDEK, nil
	case sqssvc.SSEKMS:
		keyParam := sqssvc.KmsMasterKeyID(attrs)
		keyID, err := s.store.ResolveKeyID(verified.AccountID, keyParam)
		if err != nil {
			return nil, false, nil, err
		}
		kmsKey, err := s.store.GetKey(keyID)
		if err != nil || kmsKey.AccountID != verified.AccountID {
			return nil, false, nil, err
		}
		if !s.authorizeKMSOp(verified, catalog.ActionKMSGenerateDataKey, kmsKey, nil) {
			return nil, false, nil, errSQSAccessDenied
		}
		if kmsKey.KeyState != store.KeyStateEnabled {
			return nil, false, nil, errors.New("kms key disabled")
		}
		cmk, err := s.store.UnsealKeyMaterial(keyID)
		if err != nil {
			return nil, false, nil, err
		}
		dek := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, dek); err != nil {
			return nil, false, nil, err
		}
		sealedDEK, err := kmssvc.EncryptUnderCMK(cmk, keyID, dek, nil)
		if err != nil {
			return nil, false, nil, err
		}
		ct, err := sqssvc.EncryptAES256GCM(dek, plain)
		if err != nil {
			return nil, false, nil, err
		}
		return ct, true, sealedDEK, nil
	default:
		return plain, false, nil, nil
	}
}

func (s *Server) decryptSQSBody(verified *authn.Verified, attrs map[string]string, msg store.Message) ([]byte, error) {
	if !msg.Sealed {
		return msg.Body, nil
	}
	switch sqssvc.ModeFromAttributes(attrs) {
	case sqssvc.SSEKMS:
		keyParam := sqssvc.KmsMasterKeyID(attrs)
		keyID, err := s.store.ResolveKeyID(verified.AccountID, keyParam)
		if err != nil {
			return nil, err
		}
		kmsKey, err := s.store.GetKey(keyID)
		if err != nil {
			return nil, err
		}
		if !s.authorizeKMSOp(verified, catalog.ActionKMSDecrypt, kmsKey, nil) {
			return nil, errSQSAccessDenied
		}
		dek, err := s.store.DecryptBlobWithKey(keyID, msg.SealedDEK)
		if err != nil {
			return nil, err
		}
		return sqssvc.DecryptAES256GCM(dek, msg.Body)
	default:
		// SSE-SQS (or sealed without explicit attr): DEK sealed with master key.
		dek, err := s.store.UnsealWithMaster(msg.SealedDEK)
		if err != nil {
			return nil, err
		}
		return sqssvc.DecryptAES256GCM(dek, msg.Body)
	}
}

func (s *Server) writeSQSEncryptError(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	err error,
) {
	if errors.Is(err, errSQSAccessDenied) {
		s.writeSQSError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"Access Denied", readOnly, eventID, verified)
		return
	}
	s.writeSQSError(w, r, requestID, http.StatusBadRequest, "KmsAccessDenied",
		"Unable to encrypt or decrypt message body.", readOnly, eventID, verified)
}

func (s *Server) writeSQSOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeSQSError(
	w http.ResponseWriter,
	r *http.Request,
	requestID string,
	status int,
	code, message string,
	readOnly bool,
	eventID string,
	verified *authn.Verified,
) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]string{
		"__type":  code,
		"message": message,
	})
	_, _ = w.Write(payload)

	accessKeyID := ""
	accountID := ""
	if verified != nil {
		accessKeyID = verified.AccessKeyID
		accountID = verified.AccountID
	}
	s.auditAPIError(r, requestID, eventID, code, message, readOnly, accessKeyID, accountID, verified != nil)
}

func sqsSendOptsFromParams(attrs map[string]string, params map[string]any) *store.SendMessageOpts {
	opts := &store.SendMessageOpts{}
	if n := intParam(params["DelaySeconds"], 0); n > 0 {
		opts.DelaySeconds = n
	}
	if attrTruthySQS(attrs, "FifoQueue") {
		if groupID, _ := params["MessageGroupId"].(string); strings.TrimSpace(groupID) != "" {
			opts.MessageGroupID = strings.TrimSpace(groupID)
		}
		if dedupID, _ := params["MessageDeduplicationId"].(string); strings.TrimSpace(dedupID) != "" {
			opts.MessageDeduplicationID = strings.TrimSpace(dedupID)
		}
	}
	if opts.DelaySeconds == 0 && opts.MessageGroupID == "" && opts.MessageDeduplicationID == "" {
		return nil
	}
	return opts
}

func sqsSendOptsFromEntry(attrs map[string]string, entry map[string]any) *store.SendMessageOpts {
	opts := &store.SendMessageOpts{}
	if n := intParam(entry["DelaySeconds"], 0); n > 0 {
		opts.DelaySeconds = n
	}
	if attrTruthySQS(attrs, "FifoQueue") {
		if groupID, _ := entry["MessageGroupId"].(string); strings.TrimSpace(groupID) != "" {
			opts.MessageGroupID = strings.TrimSpace(groupID)
		}
		if dedupID, _ := entry["MessageDeduplicationId"].(string); strings.TrimSpace(dedupID) != "" {
			opts.MessageDeduplicationID = strings.TrimSpace(dedupID)
		}
	}
	if opts.DelaySeconds == 0 && opts.MessageGroupID == "" && opts.MessageDeduplicationID == "" {
		return nil
	}
	return opts
}

func attrTruthySQS(attrs map[string]string, name string) bool {
	v, ok := attrs[name]
	return ok && strings.EqualFold(strings.TrimSpace(v), "true")
}

func stringMapParam(v any) map[string]string {
	out := map[string]string{}
	switch t := v.(type) {
	case map[string]string:
		for k, val := range t {
			out[k] = val
		}
	case map[string]any:
		for k, val := range t {
			switch s := val.(type) {
			case string:
				out[k] = s
			case float64:
				out[k] = strconv.FormatFloat(s, 'f', -1, 64)
			case bool:
				out[k] = strconv.FormatBool(s)
			case json.Number:
				out[k] = s.String()
			}
		}
	}
	return out
}

func anyMapParam(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	default:
		return nil
	}
}
