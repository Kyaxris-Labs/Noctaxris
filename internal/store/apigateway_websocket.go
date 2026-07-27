package store

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const APIGatewayProtocolWebSocket = "WEBSOCKET"

// APIGatewayWebSocketConnection is an in-memory lab WebSocket connection row.
type APIGatewayWebSocketConnection struct {
	ConnectionID string
	AccountID    string
	APIID        string
	Stage        string
	ConnectedAt  int64
	Messages     []string // PostToConnection lab inbox (oldest first)
}

// Ensure connection map exists (lazy).
func (s *Store) wsConnections() *sync.Map {
	s.wsConnOnce.Do(func() {
		s.wsConn = &sync.Map{}
	})
	return s.wsConn
}

// APIGatewayWebSocketAPIEndpoint builds the lab invoke base URL for a WebSocket API.
func APIGatewayWebSocketAPIEndpoint(apiID string) string {
	return "http://127.0.0.1:4566/ws-api/" + apiID
}

// IsAPIGatewayWebSocketRouteKey reports reserved WebSocket route keys.
func IsAPIGatewayWebSocketRouteKey(routeKey string) bool {
	switch strings.TrimSpace(routeKey) {
	case "$connect", "$disconnect", "$default":
		return true
	default:
		return false
	}
}

// MatchAPIGatewayWebSocketRoute finds an exact route key on a WebSocket API.
func (s *Store) MatchAPIGatewayWebSocketRoute(accountID, apiID, routeKey string) (APIGatewayRoute, error) {
	routeKey = strings.TrimSpace(routeKey)
	rows, err := s.db.Query(
		`SELECT route_id, api_id, route_key, target, authorization_type, authorizer_id
		 FROM apigwv2_routes WHERE account_id = ? AND api_id = ? AND route_key = ?`,
		accountID, apiID, routeKey,
	)
	if err != nil {
		return APIGatewayRoute{}, fmt.Errorf("match ws route: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return APIGatewayRoute{}, ErrAPIGatewayNotFound
	}
	var r APIGatewayRoute
	if err := rows.Scan(&r.RouteID, &r.APIID, &r.RouteKey, &r.Target, &r.AuthorizationType, &r.AuthorizerID); err != nil {
		return APIGatewayRoute{}, fmt.Errorf("match ws route scan: %w", err)
	}
	return r, rows.Err()
}

// CreateAPIGatewayWebSocketConnection registers a lab connection id.
func (s *Store) CreateAPIGatewayWebSocketConnection(accountID, apiID, stage string) APIGatewayWebSocketConnection {
	id := uuid.NewString()
	c := APIGatewayWebSocketConnection{
		ConnectionID: id,
		AccountID:    accountID,
		APIID:        apiID,
		Stage:        stage,
		ConnectedAt:  time.Now().UTC().UnixMilli(),
	}
	s.wsConnections().Store(wsConnKey(accountID, apiID, stage, id), &c)
	return c
}

// GetAPIGatewayWebSocketConnection returns a connection or ErrAPIGatewayNotFound.
func (s *Store) GetAPIGatewayWebSocketConnection(accountID, apiID, stage, connectionID string) (APIGatewayWebSocketConnection, error) {
	v, ok := s.wsConnections().Load(wsConnKey(accountID, apiID, stage, connectionID))
	if !ok {
		return APIGatewayWebSocketConnection{}, ErrAPIGatewayNotFound
	}
	c := v.(*APIGatewayWebSocketConnection)
	cp := *c
	cp.Messages = append([]string(nil), c.Messages...)
	return cp, nil
}

// DeleteAPIGatewayWebSocketConnection removes a connection (Gone after).
func (s *Store) DeleteAPIGatewayWebSocketConnection(accountID, apiID, stage, connectionID string) error {
	key := wsConnKey(accountID, apiID, stage, connectionID)
	if _, ok := s.wsConnections().Load(key); !ok {
		return ErrAPIGatewayNotFound
	}
	s.wsConnections().Delete(key)
	return nil
}

// PostToAPIGatewayWebSocketConnection appends a lab message for PostToConnection.
func (s *Store) PostToAPIGatewayWebSocketConnection(accountID, apiID, stage, connectionID string, data []byte) error {
	v, ok := s.wsConnections().Load(wsConnKey(accountID, apiID, stage, connectionID))
	if !ok {
		return ErrAPIGatewayNotFound
	}
	c := v.(*APIGatewayWebSocketConnection)
	c.Messages = append(c.Messages, string(data))
	return nil
}

func wsConnKey(accountID, apiID, stage, connectionID string) string {
	return accountID + "\x00" + apiID + "\x00" + stage + "\x00" + connectionID
}
