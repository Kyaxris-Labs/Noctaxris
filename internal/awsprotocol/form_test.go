package awsprotocol

import (
	"net/http"
	"net/url"
	"testing"
)

func TestMemberList_memberIndexed(t *testing.T) {
	v := url.Values{}
	v.Set("InstanceId.member.1", "i-aaa")
	v.Set("InstanceId.member.2", "i-bbb")
	got := MemberList(v, "InstanceId")
	want := []string{"i-aaa", "i-bbb"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestMemberList_dotIndexed(t *testing.T) {
	v := url.Values{}
	v.Set("VpcId.1", "vpc-1")
	v.Set("VpcId.2", "vpc-2")
	got := MemberList(v, "VpcId")
	want := []string{"vpc-1", "vpc-2"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestMemberList_prefersMemberShape(t *testing.T) {
	v := url.Values{}
	v.Set("GroupId.member.1", "sg-a")
	v.Set("GroupId.1", "sg-b")
	got := MemberList(v, "GroupId")
	if len(got) != 1 || got[0] != "sg-a" {
		t.Fatalf("got %v want [sg-a]", got)
	}
}

func TestMemberList_trimsWhitespace(t *testing.T) {
	v := url.Values{}
	v.Set("ImageId.member.1", "  ami-1  ")
	got := MemberList(v, "ImageId")
	if len(got) != 1 || got[0] != "ami-1" {
		t.Fatalf("got %v", got)
	}
}

func TestFormParams_mergesQueryAndBody(t *testing.T) {
	body := []byte("Action=Describe&Foo=bar")
	r, err := http.NewRequest(http.MethodPost, "http://localhost/?Action=Run&Version=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	vals, err := FormParams(r, body)
	if err != nil {
		t.Fatal(err)
	}
	if vals.Get("Action") != "Describe" {
		t.Fatalf("body overrides query: Action=%q", vals.Get("Action"))
	}
	if vals.Get("Version") != "1" {
		t.Fatalf("query preserved: Version=%q", vals.Get("Version"))
	}
	if vals.Get("Foo") != "bar" {
		t.Fatalf("Foo=%q", vals.Get("Foo"))
	}
}

func TestFormParams_skipsNonFormBody(t *testing.T) {
	r, err := http.NewRequest(http.MethodPost, "http://localhost/?Action=Run", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	body := []byte(`{"Action":"JsonAction"}`)
	vals, err := FormParams(r, body)
	if err != nil {
		t.Fatal(err)
	}
	if vals.Get("Action") != "Run" {
		t.Fatalf("json body must not merge: Action=%q", vals.Get("Action"))
	}
}

func TestFormParams_emptyContentTypeParsesBody(t *testing.T) {
	r, err := http.NewRequest(http.MethodPost, "http://localhost/", nil)
	if err != nil {
		t.Fatal(err)
	}
	vals, err := FormParams(r, []byte("Action=Legacy"))
	if err != nil {
		t.Fatal(err)
	}
	if vals.Get("Action") != "Legacy" {
		t.Fatalf("Action=%q", vals.Get("Action"))
	}
}
