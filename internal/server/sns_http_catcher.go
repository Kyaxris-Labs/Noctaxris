package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// handleSNSHTTPCatcher is the deny-by-default lab HTTP subscription endpoint.
// It records POSTed SNS messages and can confirm subscriptions via Token query.
func (s *Server) handleSNSHTTPCatcher(w http.ResponseWriter, r *http.Request) {
	if action := r.URL.Query().Get("Action"); action == "ConfirmSubscription" {
		token := strings.TrimSpace(r.URL.Query().Get("Token"))
		topicARN := strings.TrimSpace(r.URL.Query().Get("TopicArn"))
		sub, err := s.store.ConfirmSubscription(topicARN, token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"SubscriptionArn": sub.SubscriptionARN,
			"Status":          "confirmed",
		})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	msgType := r.Header.Get("x-amz-sns-message-type")
	if msgType == "" {
		msgType = "Notification"
	}
	var envelope map[string]any
	topicARN := ""
	subARN := ""
	if err := json.Unmarshal(body, &envelope); err == nil {
		if v, ok := envelope["TopicArn"].(string); ok {
			topicARN = v
		}
		if v, ok := envelope["SubscriptionArn"].(string); ok {
			subARN = v
		}
		if v, ok := envelope["Type"].(string); ok && v != "" {
			msgType = v
		}
	}
	if err := s.store.RecordSNSHTTPCatcher(subARN, topicARN, msgType, string(body)); err != nil {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
