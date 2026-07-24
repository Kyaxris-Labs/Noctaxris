package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// wireCognitoTriggerInvoker registers sync Lambda Invoke for Cognito User Pool triggers.
func (s *Server) wireCognitoTriggerInvoker() {
	s.store.SetCognitoTriggerInvoker(func(functionARN, eventJSON string) ([]byte, error) {
		return s.syncInvokeLambdaARN(context.Background(), functionARN, eventJSON)
	})
}

// syncInvokeLambdaARN resolves and Invokes a Lambda function ARN (RequestResponse).
// Used by Cognito triggers (fail closed on missing function or invoke error).
func (s *Server) syncInvokeLambdaARN(ctx context.Context, functionARN, eventJSON string) ([]byte, error) {
	fnAccount, fnName, ok := store.ParseLambdaARNFromSFNResource(functionARN)
	if !ok || strings.TrimSpace(fnName) == "" {
		return nil, fmt.Errorf("invalid Lambda ARN %q", functionARN)
	}
	_, qualifier := store.ParseFunctionQualifier(functionARN)
	fn, executedVersion, err := s.store.ResolveFunction(fnAccount, fnName, qualifier)
	if err != nil {
		if errors.Is(err, store.ErrNoSuchFunction) {
			return nil, fmt.Errorf("trigger function not found")
		}
		return nil, err
	}
	payload, err := s.executeLambdaInvoke(ctx, fnAccount, fnName, fn, executedVersion, eventJSON)
	if err != nil {
		return nil, fmt.Errorf("trigger invoke failed: %w", err)
	}
	return payload, nil
}
