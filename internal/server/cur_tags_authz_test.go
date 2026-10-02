package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCURTagActionsRequireAuthorize(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	userName := "cur-tag-deny"
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	// Explicit deny on tag actions (root bypasses empty identity; use Deny).
	denyDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":["cur:TagResource","cur:UntagResource","cur:ListTagsForResource"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy("arn:aws:iam::"+testAccountID+":user/"+userName, "deny-cur-tags", denyDoc); err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{"TagResource", "UntagResource", "ListTagsForResource"} {
		raw, _ := json.Marshal(map[string]any{"ReportName": "lab-report"})
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		req.Header.Set("X-Amz-Target", "AWSOrigamiServiceGatewayService."+target)
		signHeader(t, req, raw, akid, secret, testRegion, "cur", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d want 403 body=%q", target, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "AccessDeniedException") {
			t.Fatalf("%s body=%q", target, rec.Body.String())
		}
	}

	// Positive: root TagResource still succeeds.
	raw, _ := json.Marshal(map[string]any{"ReportName": "lab-report"})
	ok := mustJSONTarget(t, handler, "AWSOrigamiServiceGatewayService.TagResource", "cur", map[string]any{
		"ReportName": "lab-report",
	}, now)
	if ok.Code != http.StatusOK {
		t.Fatalf("root TagResource status=%d body=%q", ok.Code, ok.Body.String())
	}
	_ = raw
}
