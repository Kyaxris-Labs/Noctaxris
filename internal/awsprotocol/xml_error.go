package awsprotocol

import "fmt"

// QueryErrorStyle selects the XML envelope for AWS Query protocol errors.
type QueryErrorStyle int

const (
	// QueryErrorIAMSender is ErrorResponse with Error.Type Sender (e.g. Auto Scaling, Elastic Beanstalk).
	QueryErrorIAMSender QueryErrorStyle = iota
	// QueryErrorIAM is ErrorResponse without Error.Type (e.g. RDS).
	QueryErrorIAM
	// QueryErrorEC2 is the EC2 Response/Errors wrapper.
	QueryErrorEC2
)

// MarshalQueryError builds a Query protocol error XML document.
func MarshalQueryError(style QueryErrorStyle, xmlns, code, message, requestID string) ([]byte, error) {
	ec := EscapeXML(code)
	em := EscapeXML(message)
	er := EscapeXML(requestID)
	switch style {
	case QueryErrorIAMSender:
		return []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
			`<ErrorResponse xmlns="` + EscapeXML(xmlns) + `"><Error><Type>Sender</Type><Code>` + ec +
			`</Code><Message>` + em + `</Message></Error><RequestId>` + er + `</RequestId></ErrorResponse>`), nil
	case QueryErrorIAM:
		return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ErrorResponse xmlns="%s"><Error><Code>%s</Code><Message>%s</Message></Error><RequestId>%s</RequestId></ErrorResponse>`,
			EscapeXML(xmlns), ec, em, er)), nil
	case QueryErrorEC2:
		return []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
			`<Response><Errors><Error><Code>` + ec + `</Code><Message>` + em +
			`</Message></Error></Errors><RequestID>` + er + `</RequestID></Response>`), nil
	default:
		return nil, fmt.Errorf("awsprotocol: unknown QueryErrorStyle %d", style)
	}
}
