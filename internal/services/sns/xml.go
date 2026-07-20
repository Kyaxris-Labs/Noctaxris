package sns

import (
	"encoding/xml"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const snsXMLNS = "http://sns.amazonaws.com/doc/2010-03-31/"

type responseMetadata struct {
	RequestID string `xml:"RequestId"`
}

func marshalResponse(v any) ([]byte, error) {
	return xml.Marshal(v)
}

type createTopicResponse struct {
	XMLName           xml.Name `xml:"CreateTopicResponse"`
	XMLNS             string   `xml:"xmlns,attr"`
	CreateTopicResult struct {
		TopicArn string `xml:"TopicArn"`
	} `xml:"CreateTopicResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// CreateTopicXML builds a CreateTopic response.
func CreateTopicXML(topicARN, requestID string) ([]byte, error) {
	resp := createTopicResponse{XMLNS: snsXMLNS}
	resp.CreateTopicResult.TopicArn = topicARN
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type publishResponse struct {
	XMLName        xml.Name `xml:"PublishResponse"`
	XMLNS          string   `xml:"xmlns,attr"`
	PublishResult  struct {
		MessageID string `xml:"MessageId"`
	} `xml:"PublishResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// PublishXML builds a Publish response.
func PublishXML(messageID, requestID string) ([]byte, error) {
	resp := publishResponse{XMLNS: snsXMLNS}
	resp.PublishResult.MessageID = messageID
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type listTopicsResponse struct {
	XMLName          xml.Name `xml:"ListTopicsResponse"`
	XMLNS            string   `xml:"xmlns,attr"`
	ListTopicsResult struct {
		Topics struct {
			Members []topicMemberXML `xml:"member"`
		} `xml:"Topics"`
	} `xml:"ListTopicsResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

type topicMemberXML struct {
	TopicArn string `xml:"TopicArn"`
}

// ListTopicsXML builds a ListTopics response.
func ListTopicsXML(topics []store.Topic, requestID string) ([]byte, error) {
	resp := listTopicsResponse{XMLNS: snsXMLNS}
	for _, t := range topics {
		resp.ListTopicsResult.Topics.Members = append(resp.ListTopicsResult.Topics.Members, topicMemberXML{
			TopicArn: t.TopicARN,
		})
	}
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type getTopicAttributesResponse struct {
	XMLName                    xml.Name `xml:"GetTopicAttributesResponse"`
	XMLNS                      string   `xml:"xmlns,attr"`
	GetTopicAttributesResult   struct {
		Attributes attributesXML `xml:"Attributes"`
	} `xml:"GetTopicAttributesResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

type attributesXML struct {
	Entries []attributeEntryXML `xml:"entry"`
}

type attributeEntryXML struct {
	Key   string `xml:"key"`
	Value string `xml:"value"`
}

// GetTopicAttributesXML builds a GetTopicAttributes response.
func GetTopicAttributesXML(attrs map[string]string, requestID string) ([]byte, error) {
	resp := getTopicAttributesResponse{XMLNS: snsXMLNS}
	for k, v := range attrs {
		resp.GetTopicAttributesResult.Attributes.Entries = append(
			resp.GetTopicAttributesResult.Attributes.Entries,
			attributeEntryXML{Key: k, Value: v},
		)
	}
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type subscribeResponse struct {
	XMLName           xml.Name `xml:"SubscribeResponse"`
	XMLNS             string   `xml:"xmlns,attr"`
	SubscribeResult   struct {
		SubscriptionArn string `xml:"SubscriptionArn"`
	} `xml:"SubscribeResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// SubscribeXML builds a Subscribe response.
func SubscribeXML(subscriptionARN, requestID string) ([]byte, error) {
	resp := subscribeResponse{XMLNS: snsXMLNS}
	resp.SubscribeResult.SubscriptionArn = subscriptionARN
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type confirmSubscriptionResponse struct {
	XMLName                     xml.Name `xml:"ConfirmSubscriptionResponse"`
	XMLNS                       string   `xml:"xmlns,attr"`
	ConfirmSubscriptionResult   struct {
		SubscriptionArn string `xml:"SubscriptionArn"`
	} `xml:"ConfirmSubscriptionResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ConfirmSubscriptionXML builds a ConfirmSubscription response.
func ConfirmSubscriptionXML(subscriptionARN, requestID string) ([]byte, error) {
	resp := confirmSubscriptionResponse{XMLNS: snsXMLNS}
	resp.ConfirmSubscriptionResult.SubscriptionArn = subscriptionARN
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type listSubscriptionsResponse struct {
	XMLName                 xml.Name `xml:"ListSubscriptionsResponse"`
	XMLNS                   string   `xml:"xmlns,attr"`
	ListSubscriptionsResult struct {
		Subscriptions struct {
			Members []subscriptionMemberXML `xml:"member"`
		} `xml:"Subscriptions"`
	} `xml:"ListSubscriptionsResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

type subscriptionMemberXML struct {
	SubscriptionArn string `xml:"SubscriptionArn"`
	Owner           string `xml:"Owner"`
	Protocol        string `xml:"Protocol"`
	Endpoint        string `xml:"Endpoint"`
	TopicArn        string `xml:"TopicArn"`
}

func subscriptionMembers(subs []store.Subscription) []subscriptionMemberXML {
	out := make([]subscriptionMemberXML, 0, len(subs))
	for _, s := range subs {
		out = append(out, subscriptionMemberXML{
			SubscriptionArn: s.SubscriptionARN,
			Owner:           s.Owner,
			Protocol:        s.Protocol,
			Endpoint:        s.Endpoint,
			TopicArn:        s.TopicARN,
		})
	}
	return out
}

// ListSubscriptionsXML builds a ListSubscriptions response.
func ListSubscriptionsXML(subs []store.Subscription, requestID string) ([]byte, error) {
	resp := listSubscriptionsResponse{XMLNS: snsXMLNS}
	resp.ListSubscriptionsResult.Subscriptions.Members = subscriptionMembers(subs)
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type listSubscriptionsByTopicResponse struct {
	XMLName                          xml.Name `xml:"ListSubscriptionsByTopicResponse"`
	XMLNS                            string   `xml:"xmlns,attr"`
	ListSubscriptionsByTopicResult   struct {
		Subscriptions struct {
			Members []subscriptionMemberXML `xml:"member"`
		} `xml:"Subscriptions"`
	} `xml:"ListSubscriptionsByTopicResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListSubscriptionsByTopicXML builds a ListSubscriptionsByTopic response.
func ListSubscriptionsByTopicXML(subs []store.Subscription, requestID string) ([]byte, error) {
	resp := listSubscriptionsByTopicResponse{XMLNS: snsXMLNS}
	resp.ListSubscriptionsByTopicResult.Subscriptions.Members = subscriptionMembers(subs)
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type getSubscriptionAttributesResponse struct {
	XMLName                           xml.Name `xml:"GetSubscriptionAttributesResponse"`
	XMLNS                             string   `xml:"xmlns,attr"`
	GetSubscriptionAttributesResult   struct {
		Attributes attributesXML `xml:"Attributes"`
	} `xml:"GetSubscriptionAttributesResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetSubscriptionAttributesXML builds a GetSubscriptionAttributes response.
func GetSubscriptionAttributesXML(attrs map[string]string, requestID string) ([]byte, error) {
	resp := getSubscriptionAttributesResponse{XMLNS: snsXMLNS}
	for k, v := range attrs {
		resp.GetSubscriptionAttributesResult.Attributes.Entries = append(
			resp.GetSubscriptionAttributesResult.Attributes.Entries,
			attributeEntryXML{Key: k, Value: v},
		)
	}
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type emptyResponse struct {
	XMLName          xml.Name `xml:"Response"`
	XMLNS            string   `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

type deleteTopicResponse struct {
	XMLName          xml.Name `xml:"DeleteTopicResponse"`
	XMLNS            string   `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// DeleteTopicXML builds a DeleteTopic response.
func DeleteTopicXML(requestID string) ([]byte, error) {
	resp := deleteTopicResponse{XMLNS: snsXMLNS}
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type setTopicAttributesResponse struct {
	XMLName          xml.Name `xml:"SetTopicAttributesResponse"`
	XMLNS            string   `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// SetTopicAttributesXML builds a SetTopicAttributes response.
func SetTopicAttributesXML(requestID string) ([]byte, error) {
	resp := setTopicAttributesResponse{XMLNS: snsXMLNS}
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type unsubscribeResponse struct {
	XMLName          xml.Name `xml:"UnsubscribeResponse"`
	XMLNS            string   `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// UnsubscribeXML builds an Unsubscribe response.
func UnsubscribeXML(requestID string) ([]byte, error) {
	resp := unsubscribeResponse{XMLNS: snsXMLNS}
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type addPermissionResponse struct {
	XMLName          xml.Name `xml:"AddPermissionResponse"`
	XMLNS            string   `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// AddPermissionXML builds an AddPermission response.
func AddPermissionXML(requestID string) ([]byte, error) {
	resp := addPermissionResponse{XMLNS: snsXMLNS}
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}

type removePermissionResponse struct {
	XMLName          xml.Name `xml:"RemovePermissionResponse"`
	XMLNS            string   `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// RemovePermissionXML builds a RemovePermission response.
func RemovePermissionXML(requestID string) ([]byte, error) {
	resp := removePermissionResponse{XMLNS: snsXMLNS}
	resp.ResponseMetadata.RequestID = requestID
	return marshalResponse(resp)
}
