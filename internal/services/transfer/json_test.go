package transfer_test

import (
	"encoding/json"
	"testing"

	transfersvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/transfer"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestTransferJSON(t *testing.T) {
	sv := store.TransferServer{
		ServerID: "s-1", ServerARN: "arn:transfer:server", State: "ONLINE",
		IdentityProviderType: "SERVICE_MANAGED", EndpointType: "PUBLIC",
	}
	raw, err := transfersvc.CreateServerJSON(sv)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out["ServerId"] != "s-1" {
		t.Fatalf("create server=%v", out)
	}

	raw, _ = transfersvc.DescribeServerJSON(sv)
	_ = json.Unmarshal(raw, &out)
	server, _ := out["Server"].(map[string]any)
	if server["EndpointType"] != "PUBLIC" {
		t.Fatalf("describe=%v", out)
	}
	sv.EndpointType = ""
	raw, _ = transfersvc.DescribeServerJSON(sv)
	_ = json.Unmarshal(raw, &out)

	raw, _ = transfersvc.ListServersJSON([]store.TransferServer{sv})
	_ = json.Unmarshal(raw, &out)

	u := store.TransferUser{ServerID: "s-1", UserName: "alice", HomeDirectory: "/alice", RoleARN: "arn:role"}
	raw, _ = transfersvc.CreateUserJSON(u)
	_ = json.Unmarshal(raw, &out)

	keys := []store.TransferSshPublicKey{{
		SshPublicKeyID: "key-1", SshPublicKeyBody: "ssh-rsa AAAA", ImportedAt: 1_700_000_000_000,
	}}
	raw, _ = transfersvc.DescribeUserJSON("us-east-1", "000000000001", u, keys)
	_ = json.Unmarshal(raw, &out)
	user, _ := out["User"].(map[string]any)
	if user["Role"] != "arn:role" {
		t.Fatalf("user=%v", user)
	}
	u.RoleARN = "  "
	raw, _ = transfersvc.DescribeUserJSON("us-east-1", "000000000001", u, nil)

	raw, _ = transfersvc.ListUsersJSON("us-east-1", "000000000001", "s-1", []store.TransferUser{u}, map[string]int{"alice": 1})
	_ = json.Unmarshal(raw, &out)

	raw, _ = transfersvc.ImportSshPublicKeyJSON("s-1", "alice", "key-2")
	_ = json.Unmarshal(raw, &out)
}
