package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTransferDescribeUserListUsersSshKeys(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustTransferJSON(t, handler, "CreateServer", map[string]any{"Protocols": []string{"SFTP"}}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateServer status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	id, _ := out["ServerId"].(string)

	user := mustTransferJSON(t, handler, "CreateUser", map[string]any{
		"ServerId": id,
		"UserName": "carol",
	}, now)
	if user.Code != http.StatusOK {
		t.Fatalf("CreateUser status=%d body=%q", user.Code, user.Body.String())
	}

	keyBody := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC test"
	imp := mustTransferJSON(t, handler, "ImportSshPublicKey", map[string]any{
		"ServerId":         id,
		"UserName":         "carol",
		"SshPublicKeyBody": keyBody,
	}, now)
	if imp.Code != http.StatusOK {
		t.Fatalf("ImportSshPublicKey status=%d body=%q", imp.Code, imp.Body.String())
	}
	var impOut map[string]any
	if err := json.Unmarshal(imp.Body.Bytes(), &impOut); err != nil {
		t.Fatal(err)
	}
	keyID, _ := impOut["SshPublicKeyId"].(string)
	if keyID == "" {
		t.Fatalf("import out=%v", impOut)
	}

	desc := mustTransferJSON(t, handler, "DescribeUser", map[string]any{
		"ServerId": id,
		"UserName": "carol",
	}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeUser status=%d body=%q", desc.Code, desc.Body.String())
	}
	descBody := desc.Body.String()
	if !strings.Contains(descBody, keyBody) || !strings.Contains(descBody, keyID) {
		t.Fatalf("DescribeUser missing key: %q", descBody)
	}
	if !strings.Contains(descBody, `"HomeDirectoryType":"PATH"`) && !strings.Contains(descBody, `"HomeDirectoryType": "PATH"`) {
		t.Fatalf("want HomeDirectoryType PATH: %q", descBody)
	}

	list := mustTransferJSON(t, handler, "ListUsers", map[string]any{"ServerId": id}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListUsers status=%d body=%q", list.Code, list.Body.String())
	}
	listBody := list.Body.String()
	if !strings.Contains(listBody, `"SshPublicKeyCount":1`) && !strings.Contains(listBody, `"SshPublicKeyCount": 1`) {
		t.Fatalf("want SshPublicKeyCount 1: %q", listBody)
	}

	delKey := mustTransferJSON(t, handler, "DeleteSshPublicKey", map[string]any{
		"ServerId":       id,
		"UserName":       "carol",
		"SshPublicKeyId": keyID,
	}, now)
	if delKey.Code != http.StatusOK {
		t.Fatalf("DeleteSshPublicKey status=%d body=%q", delKey.Code, delKey.Body.String())
	}

	desc2 := mustTransferJSON(t, handler, "DescribeUser", map[string]any{
		"ServerId": id,
		"UserName": "carol",
	}, now)
	if desc2.Code != http.StatusOK {
		t.Fatalf("DescribeUser2 status=%d body=%q", desc2.Code, desc2.Body.String())
	}
	if strings.Contains(desc2.Body.String(), keyID) {
		t.Fatalf("key should be gone: %q", desc2.Body.String())
	}
}
