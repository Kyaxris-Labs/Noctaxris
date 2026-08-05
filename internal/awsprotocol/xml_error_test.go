package awsprotocol

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestMarshalQueryError_IAMSender(t *testing.T) {
	raw, err := MarshalQueryError(QueryErrorIAMSender, "http://autoscaling.amazonaws.com/doc/2011-01-01/", "ValidationError", "bad <input>", "req-1")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<ErrorResponse xmlns="http://autoscaling.amazonaws.com/doc/2011-01-01/">`,
		`<Type>Sender</Type>`,
		`<Code>ValidationError</Code>`,
		`<Message>bad &lt;input&gt;</Message>`,
		`<RequestId>req-1</RequestId>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestMarshalQueryError_IAM(t *testing.T) {
	raw, err := MarshalQueryError(QueryErrorIAM, "http://rds.amazonaws.com/doc/2014-10-31/", "DBInstanceNotFound", "missing", "req-2")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, "<Type>") {
		t.Fatalf("IAM style must omit Type: %s", body)
	}
	for _, want := range []string{
		`<ErrorResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">`,
		`<Code>DBInstanceNotFound</Code>`,
		`<Message>missing</Message>`,
		`<RequestId>req-2</RequestId>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestMarshalQueryError_EC2(t *testing.T) {
	raw, err := MarshalQueryError(QueryErrorEC2, "", "InvalidInstanceID.NotFound", "not found", "req-3")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		`<Response><Errors><Error>`,
		`<Code>InvalidInstanceID.NotFound</Code>`,
		`<Message>not found</Message>`,
		`<RequestID>req-3</RequestID>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestMarshalQueryError_unknownStyle(t *testing.T) {
	_, err := MarshalQueryError(QueryErrorStyle(99), "ns", "Code", "msg", "id")
	if err == nil {
		t.Fatal("expected error for unknown style")
	}
	if !strings.Contains(err.Error(), "unknown QueryErrorStyle") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarshalQueryError_escapesXMLNS(t *testing.T) {
	raw, err := MarshalQueryError(QueryErrorIAMSender, `http://x.com/"evil`, "C", "m", "r")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, `xmlns="http://x.com/"evil"`) {
		t.Fatalf("xmlns must be escaped: %s", body)
	}
	if !strings.Contains(body, `&#34;evil`) && !strings.Contains(body, `&quot;evil`) {
		t.Fatalf("expected escaped quote in xmlns: %s", body)
	}
}

func TestMemberList_empty(t *testing.T) {
	if got := MemberList(nil, "X"); len(got) != 0 {
		t.Fatalf("nil values: got %v", got)
	}
	if got := MemberList(url.Values{}, "X"); len(got) != 0 {
		t.Fatalf("empty values: got %v", got)
	}
}

func TestFormParams_emptyBodyKeepsQuery(t *testing.T) {
	r, err := http.NewRequest(http.MethodGet, "http://localhost/?A=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	vals, err := FormParams(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if vals.Get("A") != "1" {
		t.Fatalf("A=%q", vals.Get("A"))
	}
}
