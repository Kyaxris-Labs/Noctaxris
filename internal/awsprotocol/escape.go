package awsprotocol

import (
	"encoding/xml"
	"strings"
)

// EscapeXML escapes s for inclusion in XML text nodes (encoding/xml EscapeText rules).
func EscapeXML(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
