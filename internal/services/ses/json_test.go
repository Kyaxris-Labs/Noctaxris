package ses_test

import (
	"strings"
	"testing"

	sessvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ses"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSESJSONAndXML(t *testing.T) {
	req := "req-ses"
	id := store.SESIdentity{Identity: "lab@example.com", Type: "EmailAddress", Verified: true}
	if _, err := sessvc.CreateEmailIdentityJSON(id); err != nil {
		t.Fatal(err)
	}
	if _, err := sessvc.ListEmailIdentitiesJSON([]store.SESIdentity{id}); err != nil {
		t.Fatal(err)
	}
	if _, err := sessvc.GetEmailIdentityJSON(id); err != nil {
		t.Fatal(err)
	}
	if _, err := sessvc.SendEmailV2JSON("msg-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := sessvc.GetAccountJSON(5); err != nil {
		t.Fatal(err)
	}
	if len(sessvc.EmptyJSON()) == 0 {
		t.Fatal("empty json")
	}

	if _, err := sessvc.VerifyEmailIdentityXML(req); err != nil {
		t.Fatal(err)
	}
	if _, err := sessvc.SendEmailXML("msg-2", req); err != nil {
		t.Fatal(err)
	}
	if _, err := sessvc.SendRawEmailXML("msg-3", req); err != nil {
		t.Fatal(err)
	}
	list, err := sessvc.ListIdentitiesXML([]store.SESIdentity{id}, req)
	if err != nil || !strings.Contains(string(list), "lab@example.com") {
		t.Fatalf("list identities: %s %v", list, err)
	}
	stats := store.SESSendStatistics{Bounces: 1, Complaints: 0, DeliveryAttempts: 10, Rejects: 0}
	if _, err := sessvc.GetSendStatisticsXML(stats, req); err != nil {
		t.Fatal(err)
	}
	if _, err := sessvc.SetIdentityNotificationTopicXML(req); err != nil {
		t.Fatal(err)
	}
	errXML, err := sessvc.ErrorXML("MessageRejected", "bad", req)
	if err != nil || !strings.Contains(string(errXML), "MessageRejected") {
		t.Fatalf("error xml: %s %v", errXML, err)
	}
}
