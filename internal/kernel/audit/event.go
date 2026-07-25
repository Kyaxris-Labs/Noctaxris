package audit

// Resource is a CloudTrail-shaped resources[] member
// (accountId, type, ARN per AWS event record contents).
type Resource struct {
	AccountID string `json:"accountId,omitempty"`
	Type      string `json:"type,omitempty"`
	ARN       string `json:"ARN,omitempty"`
}

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
	Resources          []Resource     `json:"resources,omitempty"`
	ErrorCode          string         `json:"errorCode,omitempty"`
	ErrorMessage       string         `json:"errorMessage,omitempty"`
	RequestID          string         `json:"requestID"`
	EventID            string         `json:"eventID"`
	EventType          string         `json:"eventType"`
	RecipientAccountID string         `json:"recipientAccountId"`
	ReadOnly           bool           `json:"readOnly,omitempty"`
	EventCategory      string         `json:"eventCategory,omitempty"`
	ManagementEvent    *bool          `json:"managementEvent,omitempty"`
	// Insight-shaped lab fields (AwsCloudTrailInsight). No ML engine; seeded via inject only.
	SharedEventID  string `json:"sharedEventID,omitempty"`
	InsightDetails any    `json:"insightDetails,omitempty"`
}
