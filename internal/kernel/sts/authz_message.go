package sts

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// EncodeAuthorizationMessage builds a base64 JSON authorization message.
func EncodeAuthorizationMessage(code, message string) string {
	payload, _ := json.Marshal(map[string]string{
		"code":    code,
		"message": message,
	})
	return base64.StdEncoding.EncodeToString(payload)
}

// DecodeAuthorizationMessage decodes a previously encoded authorization message.
func DecodeAuthorizationMessage(encoded string) (decoded string, err error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", fmt.Errorf("%s", CodeInvalidAuthorizationMessageException)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("%s", CodeInvalidAuthorizationMessageException)
	}
	return string(raw), nil
}
