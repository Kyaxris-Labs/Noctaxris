package audit

// Event is a CloudTrail-shaped audit record.
// See https://docs.aws.amazon.com/awscloudtrail/latest/userguide/cloudtrail-event-reference-record-contents.html
type Event struct {
	EventVersion       string         `json:"eventVersion"`
	UserIdentity       any            `json:"userIdentity,omitempty"`
	EventTime          string         `json:"eventTime"`
	EventSource        string         `json:"eventSource"`
	EventName          string         `json:"eventName"`
	AWSRegion          string         `json:"awsRegion,omitempty"`
	SourceIPAddress    string         `json:"sourceIPAddress,omitempty"`
	UserAgent          string         `json:"userAgent,omitempty"`
	RequestParameters  map[string]any `json:"requestParameters,omitempty"`
	ResponseElements   map[string]any `json:"responseElements,omitempty"`
	ErrorCode          string         `json:"errorCode,omitempty"`
	ErrorMessage       string         `json:"errorMessage,omitempty"`
	RequestID          string         `json:"requestID"`
	EventID            string         `json:"eventID"`
	EventType          string         `json:"eventType"`
	RecipientAccountID string         `json:"recipientAccountId"`
	ReadOnly           bool           `json:"readOnly,omitempty"`
}
