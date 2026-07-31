package awsprotocol

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// FormParams merges URL query parameters with form fields from the request body when the
// body is application/x-www-form-urlencoded (or Content-Type is empty, matching legacy parsers).
func FormParams(r *http.Request, body []byte) (url.Values, error) {
	vals := url.Values{}
	for k, v := range r.URL.Query() {
		vals[k] = v
	}
	if len(body) == 0 || !formURLEncodedBody(r) {
		return vals, nil
	}
	parsed, err := url.ParseQuery(string(body))
	if err != nil {
		return vals, err
	}
	for k, v := range parsed {
		vals[k] = v
	}
	return vals, nil
}

func formURLEncodedBody(r *http.Request) bool {
	ct := strings.TrimSpace(r.Header.Get("Content-Type"))
	if ct == "" {
		return true
	}
	ct = strings.ToLower(ct)
	return strings.Contains(ct, "application/x-www-form-urlencoded")
}

// MemberList returns indexed list values for AWS Query list parameters, supporting both
// prefix.member.N and prefix.N key shapes.
func MemberList(values url.Values, prefix string) []string {
	var out []string
	for i := 1; ; i++ {
		v := strings.TrimSpace(values.Get(prefix + ".member." + strconv.Itoa(i)))
		if v == "" {
			v = strings.TrimSpace(values.Get(prefix + "." + strconv.Itoa(i)))
		}
		if v == "" {
			break
		}
		out = append(out, v)
	}
	return out
}
