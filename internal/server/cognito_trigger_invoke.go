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
	s.store.SetCognitoTriggerInvoker(func(functionARN, eventJSON string) error {
		return s.syncInvokeLambdaARN(context.Background(), functionARN, eventJSON)
	})
}

// syncInvokeLambdaARN resolves and Invokes a Lambda function ARN (RequestResponse).
// Used by Cognito triggers (fail closed on missing function or invoke error).
func (s *Server) syncInvokeLambdaARN(ctx context.Context, functionARN, eventJSON string) error {
	fnAccount, fnName, ok := store.ParseLambdaARNFromSFNResource(functionARN)
	if !ok || strings.TrimSpace(fnName) == "" {
		return fmt.Errorf("invalid Lambda ARN %q", functionARN)
	}
	_, qualifier := store.ParseFunctionQualifier(functionARN)
	fn, executedVersion, err := s.store.ResolveFunction(fnAccount, fnName, qualifier)
	if err != nil {
		if errors.Is(err, store.ErrNoSuchFunction) {
			return fmt.Errorf("trigger function not found")
		}
		return err
	}
	_, err = s.executeLambdaInvoke(ctx, fnAccount, fnName, fn, executedVersion, eventJSON)
	if err != nil {
		return fmt.Errorf("trigger invoke failed: %w", err)
	}
	return nil
}
