package store_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAPIGatewayWebSocketCreateRoutesAndPostToConnection(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const acct = "000000000001"
	api, err := st.CreateAPIGatewayAPI(acct, "us-east-1", "ws-lab", store.APIGatewayProtocolWebSocket)
	if err != nil {
		t.Fatal(err)
	}
	if api.ProtocolType != store.APIGatewayProtocolWebSocket {
		t.Fatalf("protocol=%q", api.ProtocolType)
	}
	if !strings.HasPrefix(api.APIEndpoint, "http://127.0.0.1:4566/ws-api/") {
		t.Fatalf("endpoint=%q", api.APIEndpoint)
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + acct + ":function:ws"
	in, err := st.CreateAPIGatewayIntegration(acct, api.APIID, "AWS_PROXY", lambdaARN, "1.0", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, rk := range []string{"$connect", "$disconnect", "$default"} {
		if _, err := st.CreateAPIGatewayRoute(acct, api.APIID, rk, "integrations/"+in.IntegrationID, "NONE", ""); err != nil {
			t.Fatalf("route %s: %v", rk, err)
		}
	}
	if _, err := st.CreateAPIGatewayRoute(acct, api.APIID, "GET /nope", "integrations/"+in.IntegrationID, "NONE", ""); err == nil {
		t.Fatal("expected HTTP-style route reject on WebSocket API")
	}
	_, err = st.CreateAPIGatewayStage(acct, api.APIID, "$default", true)
	if err != nil {
		t.Fatal(err)
	}
	matched, err := st.MatchAPIGatewayWebSocketRoute(acct, api.APIID, "$connect")
	if err != nil || matched.RouteKey != "$connect" {
		t.Fatalf("match=%+v err=%v", matched, err)
	}

	conn := st.CreateAPIGatewayWebSocketConnection(acct, api.APIID, "$default")
	if err := st.PostToAPIGatewayWebSocketConnection(acct, api.APIID, "$default", conn.ConnectionID, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetAPIGatewayWebSocketConnection(acct, api.APIID, "$default", conn.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 1 || got.Messages[0] != "hello" {
		t.Fatalf("messages=%v", got.Messages)
	}
	if err := st.DeleteAPIGatewayWebSocketConnection(acct, api.APIID, "$default", conn.ConnectionID); err != nil {
		t.Fatal(err)
	}
	if err := st.PostToAPIGatewayWebSocketConnection(acct, api.APIID, "$default", conn.ConnectionID, []byte("x")); err == nil {
		t.Fatal("expected Gone after delete")
	}
}
