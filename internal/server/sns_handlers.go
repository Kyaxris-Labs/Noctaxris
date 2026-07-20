package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	snssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/sns"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const snsEventSource = "sns.amazonaws.com"

var snsAttrEntryKey = regexp.MustCompile(`^Attributes\.entry\.(\d+)\.key$`)

func (s *Server) handleSNS(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	action = snsAction(action)
	accountID := verified.AccountID
	region := verified.Region
	if region == "" {
		region = store.DefaultSNSRegion
	}

	resource := "*"
	topicPolicy := ""
	var (
		topic store.Topic
		err   error
	)

	switch action {
	case catalog.ActionSNSListTopics, "ListTopics":
		resource = "*"
	case catalog.ActionSNSListSubscriptions, "ListSubscriptions":
		resource = "*"
	case catalog.ActionSNSCreateTopic, "CreateTopic":
		name := strings.TrimSpace(params.Get("Name"))
		if name == "" {
			s.writeSNSError(w, r, requestID, http.StatusBadRequest, "InvalidParameter",
				"Topic Name must be specified.", readOnly, eventID, verified)
			return
		}
		resource = store.TopicARN(region, accountID, name)
		// Policy attribute at create (SQS CreateQueue parity) feeds dual-eval authz.
		if p := snsAttributesFromForm(params)["Policy"]; strings.TrimSpace(p) != "" {
			topicPolicy = p
		}
	default:
		if action == catalog.ActionSNSUnsubscribe || action == "Unsubscribe" ||
			action == catalog.ActionSNSGetSubscriptionAttributes || action == "GetSubscriptionAttributes" {
			subARN := strings.TrimSpace(params.Get("SubscriptionArn"))
			if subARN == "" {
				s.writeSNSError(w, r, requestID, http.StatusBadRequest, "InvalidParameter",
					"SubscriptionArn is required.", readOnly, eventID, verified)
				return
			}
			sub, subErr := s.store.GetSubscription(subARN)
			if subErr != nil {
				if errors.Is(subErr, store.ErrNoSuchSubscription) {
					s.writeSNSError(w, r, requestID, http.StatusNotFound, "NotFound",
						"Subscription does not exist.", readOnly, eventID, verified)
					return
				}
				s.writeSNSError(w, r, requestID, http.StatusInternalServerError, "InternalError",
					"Unable to resolve subscription.", readOnly, eventID, verified)
				return
			}
			topic, err = s.store.GetTopicByARN(sub.TopicARN)
			if err != nil {
				if errors.Is(err, store.ErrNoSuchTopic) {
					s.writeSNSError(w, r, requestID, http.StatusNotFound, "NotFound",
						"Topic does not exist.", readOnly, eventID, verified)
					return
				}
				s.writeSNSError(w, r, requestID, http.StatusInternalServerError, "InternalError",
					"Unable to resolve topic.", readOnly, eventID, verified)
				return
			}
			if topic.AccountID != accountID {
				s.writeSNSError(w, r, requestID, http.StatusNotFound, "NotFound",
					"Subscription does not exist.", readOnly, eventID, verified)
				return
			}
			resource = topic.TopicARN
			topicPolicy = topic.Policy
			break
		}

		topic, err = s.resolveSNSTopic(accountID, params)
		if err != nil {
			if errors.Is(err, store.ErrNoSuchTopic) {
				s.writeSNSError(w, r, requestID, http.StatusNotFound, "NotFound",
					"Topic does not exist.", readOnly, eventID, verified)
				return
			}
			if err.Error() == "missing topic" {
				s.writeSNSError(w, r, requestID, http.StatusBadRequest, "InvalidParameter",
					"TopicArn or Name is required.", readOnly, eventID, verified)
				return
			}
			s.writeSNSError(w, r, requestID, http.StatusBadRequest, "InvalidParameter",
				"Unable to resolve topic.", readOnly, eventID, verified)
			return
		}
		resource = topic.TopicARN
		topicPolicy = topic.Policy
	}

	if !s.authorizeSNS(verified, action, resource, topicPolicy) {
		s.writeSNSError(w, r, requestID, http.StatusForbidden, "AuthorizationError",
			"User: "+verified.Principal.ARN()+" is not authorized to perform: "+action+" on resource: "+resource,
			readOnly, eventID, verified)
		return
	}

	var payload []byte
	switch action {
	case catalog.ActionSNSCreateTopic, "CreateTopic":
		payload, err = s.snsCreateTopic(region, accountID, params, requestID)
	case catalog.ActionSNSDeleteTopic, "DeleteTopic":
		payload, err = s.snsDeleteTopic(topic.AccountID, topic, requestID)
	case catalog.ActionSNSListTopics, "ListTopics":
		payload, err = s.snsListTopics(accountID, requestID)
	case catalog.ActionSNSGetTopicAttributes, "GetTopicAttributes":
		payload, err = s.snsGetTopicAttributes(topic, requestID)
	case catalog.ActionSNSSetTopicAttributes, "SetTopicAttributes":
		payload, err = s.snsSetTopicAttributes(topic.AccountID, topic, params, requestID)
	case catalog.ActionSNSPublish, "Publish":
		payload, err = s.snsPublish(w, r, requestID, eventID, verified, readOnly, topic.AccountID, topic, params)
		if err != nil || payload == nil {
			return
		}
	case catalog.ActionSNSSubscribe, "Subscribe":
		payload, err = s.snsSubscribe(accountID, topic, params, requestID)
	case catalog.ActionSNSUnsubscribe, "Unsubscribe":
		payload, err = s.snsUnsubscribe(params, requestID)
	case catalog.ActionSNSListSubscriptions, "ListSubscriptions":
		payload, err = s.snsListSubscriptions(accountID, requestID)
	case catalog.ActionSNSListSubscriptionsByTopic, "ListSubscriptionsByTopic":
		payload, err = s.snsListSubscriptionsByTopic(topic.AccountID, topic, requestID)
	case catalog.ActionSNSGetSubscriptionAttributes, "GetSubscriptionAttributes":
		payload, err = s.snsGetSubscriptionAttributes(params, requestID)
	case catalog.ActionSNSAddPermission, "AddPermission":
		payload, err = s.snsAddPermission(topic.AccountID, topic, params, requestID)
	case catalog.ActionSNSRemovePermission, "RemovePermission":
		payload, err = s.snsRemovePermission(topic.AccountID, topic, params, requestID)
	default:
		s.writeSNSError(w, r, requestID, http.StatusNotImplemented, "NotImplemented",
			"This SNS action is not implemented.", readOnly, eventID, verified)
		return
	}

	if err != nil {
		if errors.Is(err, store.ErrTopicAlreadyExists) {
			s.writeSNSError(w, r, requestID, http.StatusBadRequest, "TopicAlreadyExists",
				"Topic already exists with different attributes.", readOnly, eventID, verified)
			return
		}
		if errors.Is(err, store.ErrSNSPolicyStatementExists) {
			s.writeSNSError(w, r, requestID, http.StatusBadRequest, "InvalidParameter",
				"Statement id already exists.", readOnly, eventID, verified)
			return
		}
		s.writeSNSError(w, r, requestID, http.StatusInternalServerError, "InternalError",
			"Unable to complete SNS request.", readOnly, eventID, verified)
		return
	}

	s.writeXMLOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, snsEventSource, snsEventName(action), readOnly)
}

func snsAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateTopic":
		return catalog.ActionSNSCreateTopic
	case "DeleteTopic":
		return catalog.ActionSNSDeleteTopic
	case "ListTopics":
		return catalog.ActionSNSListTopics
	case "GetTopicAttributes":
		return catalog.ActionSNSGetTopicAttributes
	case "SetTopicAttributes":
		return catalog.ActionSNSSetTopicAttributes
	case "Publish":
		return catalog.ActionSNSPublish
	case "Subscribe":
		return catalog.ActionSNSSubscribe
	case "Unsubscribe":
		return catalog.ActionSNSUnsubscribe
	case "ListSubscriptions":
		return catalog.ActionSNSListSubscriptions
	case "ListSubscriptionsByTopic":
		return catalog.ActionSNSListSubscriptionsByTopic
	case "GetSubscriptionAttributes":
		return catalog.ActionSNSGetSubscriptionAttributes
	case "AddPermission":
		return catalog.ActionSNSAddPermission
	case "RemovePermission":
		return catalog.ActionSNSRemovePermission
	default:
		return action
	}
}

func snsEventName(action string) string {
	action = snsAction(action)
	if i := strings.LastIndex(action, ":"); i >= 0 && i+1 < len(action) {
		return action[i+1:]
	}
	return action
}

func (s *Server) authorizeSNS(verified *authn.Verified, action, resource, topicPolicy string) bool {
	resourceAccountID := resourceAccountIDFromARN(resource)
	if resourceAccountID == "" {
		resourceAccountID = verified.AccountID
	}
	return s.authorizeDataplaneOR(verified, action, resource, resourceAccountID, func(caller authz.RequestContext, identityDocs []string, resourceAccountID string) authz.Decision {
		return authz.EvaluateSNS(authz.SNSRequest{
			Caller:            caller,
			IdentityDocs:      identityDocs,
			TopicPolicyDoc:    topicPolicy,
			ResourceAccountID: resourceAccountID,
		})
	})
}

func (s *Server) resolveSNSTopic(accountID string, params url.Values) (store.Topic, error) {
	if arn := strings.TrimSpace(params.Get("TopicArn")); arn != "" {
		return s.store.GetTopicByARN(arn)
	}
	if name := strings.TrimSpace(params.Get("Name")); name != "" {
		return s.store.GetTopic(accountID, name)
	}
	return store.Topic{}, errors.New("missing topic")
}

func (s *Server) snsCreateTopic(region, accountID string, params url.Values, requestID string) ([]byte, error) {
	name := strings.TrimSpace(params.Get("Name"))
	attrs := snsAttributesFromForm(params)
	topic, err := s.store.CreateTopic(accountID, region, name, attrs)
	if err != nil {
		return nil, err
	}
	return snssvc.CreateTopicXML(topic.TopicARN, requestID)
}

func (s *Server) snsDeleteTopic(accountID string, topic store.Topic, requestID string) ([]byte, error) {
	if err := s.store.DeleteTopic(accountID, topic.TopicName); err != nil {
		return nil, err
	}
	return snssvc.DeleteTopicXML(requestID)
}

func (s *Server) snsListTopics(accountID, requestID string) ([]byte, error) {
	topics, err := s.store.ListTopics(accountID)
	if err != nil {
		return nil, err
	}
	return snssvc.ListTopicsXML(topics, requestID)
}

func (s *Server) snsGetTopicAttributes(topic store.Topic, requestID string) ([]byte, error) {
	attrs, err := s.store.GetTopicAttributes(topic.AccountID, topic.TopicName)
	if err != nil {
		return nil, err
	}
	return snssvc.GetTopicAttributesXML(attrs, requestID)
}

func (s *Server) snsSetTopicAttributes(accountID string, topic store.Topic, params url.Values, requestID string) ([]byte, error) {
	attrs := snsAttributesFromForm(params)
	if len(attrs) == 0 {
		return nil, fmt.Errorf("no attributes")
	}
	if err := s.store.SetTopicAttributes(accountID, topic.TopicName, attrs); err != nil {
		return nil, err
	}
	return snssvc.SetTopicAttributesXML(requestID)
}

func (s *Server) snsPublish(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	accountID string,
	topic store.Topic,
	params url.Values,
) ([]byte, error) {
	message := params.Get("Message")
	if message == "" {
		s.writeSNSError(w, r, requestID, http.StatusBadRequest, "InvalidParameter",
			"Message is required.", readOnly, eventID, verified)
		return nil, nil
	}
	subject := params.Get("Subject")
	result, err := s.store.Publish(accountID, topic.TopicName, message, subject, nil)
	if err != nil {
		if errors.Is(err, store.ErrNoSuchTopic) {
			s.writeSNSError(w, r, requestID, http.StatusNotFound, "NotFound",
				"Topic does not exist.", readOnly, eventID, verified)
			return nil, nil
		}
		return nil, err
	}
	return snssvc.PublishXML(result.MessageID, requestID)
}

func (s *Server) snsSubscribe(accountID string, topic store.Topic, params url.Values, requestID string) ([]byte, error) {
	protocol := strings.TrimSpace(params.Get("Protocol"))
	endpoint := strings.TrimSpace(params.Get("Endpoint"))
	sub, err := s.store.Subscribe(accountID, topic.TopicARN, protocol, endpoint)
	if err != nil {
		return nil, err
	}
	return snssvc.SubscribeXML(sub.SubscriptionARN, requestID)
}

func (s *Server) snsUnsubscribe(params url.Values, requestID string) ([]byte, error) {
	subARN := strings.TrimSpace(params.Get("SubscriptionArn"))
	if err := s.store.Unsubscribe(subARN); err != nil {
		return nil, err
	}
	return snssvc.UnsubscribeXML(requestID)
}

func (s *Server) snsListSubscriptions(accountID, requestID string) ([]byte, error) {
	subs, err := s.store.ListSubscriptions(accountID)
	if err != nil {
		return nil, err
	}
	return snssvc.ListSubscriptionsXML(subs, requestID)
}

func (s *Server) snsListSubscriptionsByTopic(accountID string, topic store.Topic, requestID string) ([]byte, error) {
	subs, err := s.store.ListSubscriptionsByTopic(accountID, topic.TopicName)
	if err != nil {
		return nil, err
	}
	return snssvc.ListSubscriptionsByTopicXML(subs, requestID)
}

func (s *Server) snsGetSubscriptionAttributes(params url.Values, requestID string) ([]byte, error) {
	subARN := strings.TrimSpace(params.Get("SubscriptionArn"))
	attrs, err := s.store.GetSubscriptionAttributes(subARN)
	if err != nil {
		return nil, err
	}
	return snssvc.GetSubscriptionAttributesXML(attrs, requestID)
}

func (s *Server) snsAddPermission(accountID string, topic store.Topic, params url.Values, requestID string) ([]byte, error) {
	label := strings.TrimSpace(params.Get("Label"))
	actionName := strings.TrimSpace(params.Get("ActionName"))
	principal := snsPermissionPrincipal(params)
	if principal == "" || actionName == "" {
		return nil, fmt.Errorf("missing add permission fields")
	}
	if err := s.store.AddTopicPermission(accountID, topic.TopicName, label, principal, actionName); err != nil {
		return nil, err
	}
	return snssvc.AddPermissionXML(requestID)
}

func (s *Server) snsRemovePermission(accountID string, topic store.Topic, params url.Values, requestID string) ([]byte, error) {
	label := strings.TrimSpace(params.Get("Label"))
	if err := s.store.RemoveTopicPermission(accountID, topic.TopicName, label); err != nil {
		return nil, err
	}
	return snssvc.RemovePermissionXML(requestID)
}

func snsPermissionPrincipal(params url.Values) string {
	if arn := strings.TrimSpace(params.Get("Principal")); arn != "" {
		return arn
	}
	if acct := strings.TrimSpace(params.Get("AWSAccountId")); acct != "" {
		return "arn:aws:iam::" + acct + ":root"
	}
	return ""
}

func snsAttributesFromForm(vals url.Values) map[string]string {
	attrs := map[string]string{}
	if name := strings.TrimSpace(vals.Get("AttributeName")); name != "" {
		attrs[name] = vals.Get("AttributeValue")
	}
	entryKeys := map[int]string{}
	entryVals := map[int]string{}
	for k, vs := range vals {
		if len(vs) == 0 {
			continue
		}
		if m := snsAttrEntryKey.FindStringSubmatch(k); len(m) == 2 {
			idx, _ := strconv.Atoi(m[1])
			entryKeys[idx] = vs[0]
			continue
		}
		if strings.HasPrefix(k, "Attributes.entry.") && strings.HasSuffix(k, ".value") {
			mid := strings.TrimSuffix(strings.TrimPrefix(k, "Attributes.entry."), ".value")
			idx, _ := strconv.Atoi(mid)
			entryVals[idx] = vs[0]
		}
	}
	for idx, key := range entryKeys {
		if val, ok := entryVals[idx]; ok && key != "" {
			attrs[key] = val
		}
	}
	return attrs
}

func (s *Server) writeSNSError(
	w http.ResponseWriter,
	r *http.Request,
	requestID string,
	status int,
	code, message string,
	readOnly bool,
	eventID string,
	verified *authn.Verified,
) {
	accessKeyID := ""
	accountID := ""
	if verified != nil {
		accessKeyID = verified.AccessKeyID
		accountID = verified.AccountID
	}
	s.writeAWSError(w, requestID, status, code, message, readOnly, r, eventID, accessKeyID, accountID, verified != nil)
}
