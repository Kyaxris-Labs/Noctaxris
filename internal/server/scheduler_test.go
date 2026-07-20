package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustSchedulerJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "Scheduler."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "scheduler", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestSchedulerCRUDAndDueDeliveryToSQS(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createQ := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "sched-handler-q",
	}, now)
	if createQ.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", createQ.Code, createQ.Body.String())
	}
	queueARN := "arn:aws:sqs:us-east-1:" + testAccountID + ":sched-handler-q"
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"scheduler.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + queueARN + `"}]}`
	var createOut map[string]any
	_ = json.Unmarshal(createQ.Body.Bytes(), &createOut)
	queueURL, _ := createOut["QueueUrl"].(string)
	setRec := mustSQSJSON(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl":   queueURL,
		"Attributes": map[string]string{"Policy": policy},
	}, now)
	if setRec.Code != http.StatusOK {
		t.Fatalf("SetQueueAttributes status=%d body=%q", setRec.Code, setRec.Body.String())
	}

	createRec := mustSchedulerJSON(t, handler, "CreateSchedule", map[string]any{
		"Name":               "lab-rate",
		"ScheduleExpression": "rate(1 minutes)",
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
		"Target": map[string]any{
			"Arn":   queueARN,
			"Input": `{"ping":true}`,
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSchedule status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	getRec := mustSchedulerJSON(t, handler, "GetSchedule", map[string]any{"Name": "lab-rate"}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetSchedule status=%d body=%q", getRec.Code, getRec.Body.String())
	}

	listRec := mustSchedulerJSON(t, handler, "ListSchedules", map[string]any{}, now)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "lab-rate") {
		t.Fatalf("ListSchedules status=%d body=%q", listRec.Code, listRec.Body.String())
	}

	n, err := st.ProcessDueSchedules(now.Add(2 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("delivered=%d want 1", n)
	}
	msgs, err := st.ReceiveMessages(testAccountID, "sched-handler-q", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || string(msgs[0].Body) != `{"ping":true}` {
		t.Fatalf("msgs=%+v", msgs)
	}

	delRec := mustSchedulerJSON(t, handler, "DeleteSchedule", map[string]any{"Name": "lab-rate"}, now)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DeleteSchedule status=%d body=%q", delRec.Code, delRec.Body.String())
	}
}
