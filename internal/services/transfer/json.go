package transfer

import (
	"encoding/json"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateServerJSON builds a CreateServer success body.
func CreateServerJSON(sv store.TransferServer) ([]byte, error) {
	return json.Marshal(map[string]any{"ServerId": sv.ServerID})
}

// DescribeServerJSON builds a DescribeServer success body.
func DescribeServerJSON(sv store.TransferServer) ([]byte, error) {
	server := map[string]any{
		"Arn":                  sv.ServerARN,
		"ServerId":             sv.ServerID,
		"State":                sv.State,
		"IdentityProviderType": sv.IdentityProviderType,
		"Protocols":            []string{"SFTP"},
	}
	// Omit EndpointType until a real listener exists (no VPC theatre).
	if sv.EndpointType != "" {
		server["EndpointType"] = sv.EndpointType
	}
	return json.Marshal(map[string]any{"Server": server})
}

// ListServersJSON builds a ListServers success body.
func ListServersJSON(servers []store.TransferServer) ([]byte, error) {
	summaries := make([]map[string]any, 0, len(servers))
	for _, sv := range servers {
		summaries = append(summaries, map[string]any{
			"Arn":      sv.ServerARN,
			"ServerId": sv.ServerID,
			"State":    sv.State,
		})
	}
	return json.Marshal(map[string]any{"Servers": summaries})
}

// CreateUserJSON builds a CreateUser success body.
func CreateUserJSON(u store.TransferUser) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ServerId": u.ServerID,
		"UserName": u.UserName,
	})
}

// DescribeUserJSON builds a DescribeUser success body.
func DescribeUserJSON(region, accountID string, u store.TransferUser, keys []store.TransferSshPublicKey) ([]byte, error) {
	user := map[string]any{
		"Arn":               store.TransferUserARN(region, accountID, u.ServerID, u.UserName),
		"HomeDirectory":     u.HomeDirectory,
		"HomeDirectoryType": "PATH",
		"UserName":          u.UserName,
	}
	if strings.TrimSpace(u.RoleARN) != "" {
		user["Role"] = u.RoleARN
	}
	keyNodes := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		keyNodes = append(keyNodes, map[string]any{
			"DateImported":     float64(k.ImportedAt) / 1000.0,
			"SshPublicKeyBody": k.SshPublicKeyBody,
			"SshPublicKeyId":   k.SshPublicKeyID,
		})
	}
	user["SshPublicKeys"] = keyNodes
	return json.Marshal(map[string]any{
		"ServerId": u.ServerID,
		"User":     user,
	})
}

// ListUsersJSON builds a ListUsers success body.
func ListUsersJSON(region, accountID, serverID string, users []store.TransferUser, keyCounts map[string]int) ([]byte, error) {
	summaries := make([]map[string]any, 0, len(users))
	for _, u := range users {
		entry := map[string]any{
			"Arn":               store.TransferUserARN(region, accountID, u.ServerID, u.UserName),
			"HomeDirectory":     u.HomeDirectory,
			"HomeDirectoryType": "PATH",
			"UserName":          u.UserName,
			"SshPublicKeyCount": keyCounts[u.UserName],
		}
		if strings.TrimSpace(u.RoleARN) != "" {
			entry["Role"] = u.RoleARN
		}
		summaries = append(summaries, entry)
	}
	return json.Marshal(map[string]any{
		"ServerId": serverID,
		"Users":    summaries,
	})
}

// ImportSshPublicKeyJSON builds an ImportSshPublicKey success body.
func ImportSshPublicKeyJSON(serverID, userName, sshPublicKeyID string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ServerId":       serverID,
		"SshPublicKeyId": sshPublicKeyID,
		"UserName":       userName,
	})
}
