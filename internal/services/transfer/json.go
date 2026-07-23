package transfer

import (
	"encoding/json"

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
