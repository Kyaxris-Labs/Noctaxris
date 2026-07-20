package ses

import (
	"encoding/xml"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

type envelope struct {
	XMLName xml.Name
	Result  any        `xml:",any"`
	Metadata responseMetadata `xml:"ResponseMetadata"`
}

type responseMetadata struct {
	RequestID string `xml:"RequestId"`
}

// VerifyEmailIdentityXML builds a VerifyEmailIdentity response.
func VerifyEmailIdentityXML(requestID string) ([]byte, error) {
	type result struct {
		XMLName xml.Name `xml:"VerifyEmailIdentityResult"`
	}
	return marshal("VerifyEmailIdentityResponse", result{}, requestID)
}

// SendEmailXML builds a SendEmail response.
func SendEmailXML(messageID, requestID string) ([]byte, error) {
	type result struct {
		XMLName   xml.Name `xml:"SendEmailResult"`
		MessageID string   `xml:"MessageId"`
	}
	return marshal("SendEmailResponse", result{MessageID: messageID}, requestID)
}

// SendRawEmailXML builds a SendRawEmail response.
func SendRawEmailXML(messageID, requestID string) ([]byte, error) {
	type result struct {
		XMLName   xml.Name `xml:"SendRawEmailResult"`
		MessageID string   `xml:"MessageId"`
	}
	return marshal("SendRawEmailResponse", result{MessageID: messageID}, requestID)
}

// ListIdentitiesXML builds a ListIdentities response.
func ListIdentitiesXML(identities []store.SESIdentity, requestID string) ([]byte, error) {
	type member struct {
		Value string `xml:"member"`
	}
	type result struct {
		XMLName    xml.Name `xml:"ListIdentitiesResult"`
		Identities struct {
			Member []string `xml:"member"`
		} `xml:"Identities"`
	}
	var r result
	for _, id := range identities {
		r.Identities.Member = append(r.Identities.Member, id.Identity)
	}
	_ = member{}
	return marshal("ListIdentitiesResponse", r, requestID)
}

// GetSendStatisticsXML builds a GetSendStatistics response.
func GetSendStatisticsXML(stats store.SESSendStatistics, requestID string) ([]byte, error) {
	type datapoint struct {
		Timestamp        string `xml:"Timestamp"`
		DeliveryAttempts int64  `xml:"DeliveryAttempts"`
		Bounces          int64  `xml:"Bounces"`
		Complaints       int64  `xml:"Complaints"`
		Rejects          int64  `xml:"Rejects"`
	}
	type result struct {
		XMLName            xml.Name `xml:"GetSendStatisticsResult"`
		SendDataPoints struct {
			Member []datapoint `xml:"member"`
		} `xml:"SendDataPoints"`
	}
	var r result
	r.SendDataPoints.Member = append(r.SendDataPoints.Member, datapoint{
		Timestamp:        fmt.Sprintf("%d", stats.Timestamp),
		DeliveryAttempts: stats.DeliveryAttempts,
		Bounces:          stats.Bounces,
		Complaints:       stats.Complaints,
		Rejects:          stats.Rejects,
	})
	return marshal("GetSendStatisticsResponse", r, requestID)
}

// SetIdentityNotificationTopicXML builds a SetIdentityNotificationTopic response.
func SetIdentityNotificationTopicXML(requestID string) ([]byte, error) {
	type result struct {
		XMLName xml.Name `xml:"SetIdentityNotificationTopicResult"`
	}
	return marshal("SetIdentityNotificationTopicResponse", result{}, requestID)
}

// ErrorXML builds an SES error response.
func ErrorXML(code, message, requestID string) ([]byte, error) {
	type errBody struct {
		Type    string `xml:"Type"`
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	type errorResponse struct {
		XMLName   xml.Name `xml:"ErrorResponse"`
		Error     errBody  `xml:"Error"`
		RequestID string   `xml:"RequestId"`
	}
	return xml.Marshal(errorResponse{
		Error:     errBody{Type: "Sender", Code: code, Message: message},
		RequestID: requestID,
	})
}

func marshal(root string, result any, requestID string) ([]byte, error) {
	type wrap struct {
		XMLName  xml.Name `xml:""`
		Result   any      `xml:",omitempty"`
		Metadata responseMetadata `xml:"ResponseMetadata"`
	}
	w := wrap{
		XMLName:  xml.Name{Local: root},
		Result:   result,
		Metadata: responseMetadata{RequestID: requestID},
	}
	out, err := xml.Marshal(w)
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}
