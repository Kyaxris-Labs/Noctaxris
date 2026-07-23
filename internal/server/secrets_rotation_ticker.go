package server

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const secretsRotationTickerInterval = 5 * time.Second

// StartSecretsRotationTicker starts the in-process due-rotation worker once.
func (s *Server) StartSecretsRotationTicker() {
	s.secretsRotationTickerOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.secretsRotationTickerCancel = cancel
		go s.runSecretsRotationTicker(ctx)
	})
}

// StopSecretsRotationTicker stops the in-process secrets rotation ticker if running.
func (s *Server) StopSecretsRotationTicker() {
	if s.secretsRotationTickerCancel != nil {
		s.secretsRotationTickerCancel()
	}
}

func (s *Server) runSecretsRotationTicker(ctx context.Context) {
	ticker := time.NewTicker(secretsRotationTickerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := s.now().UTC()
			n, err := s.store.ProcessDueSecretRotations(now, func(accountID, name string) error {
				return s.rotateSecretForSchedule(ctx, accountID, name)
			})
			if err != nil {
				log.Printf("secrets rotation ticker: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("secrets rotation ticker: rotated %d secret(s)", n)
			}
		}
	}
}

// rotateSecretForSchedule runs lab random rotate or Lambda four-step without caller PassRole
// (scheduled path is service-initiated; configure-time PassRole already validated).
func (s *Server) rotateSecretForSchedule(ctx context.Context, accountID, name string) error {
	meta, err := s.store.DescribeSecret(accountID, name)
	if err != nil {
		return err
	}
	if strings.TrimSpace(meta.RotationLambdaARN) == "" {
		_, err := s.store.RotateSecret(accountID, name)
		return err
	}
	fnAccount, functionName, qualifier, _, err := s.store.ResolveSecretRotationLambda(accountID, name)
	if err != nil {
		return err
	}
	if functionName == "" {
		return store.ErrSecretRotationLambdaInvalid
	}
	_, err = s.store.RotateSecretFourStep(accountID, name, "", func(eventJSON string) error {
		job, err := s.store.EnqueueAsyncInvokeQuiet(fnAccount, functionName, qualifier, eventJSON)
		if err != nil {
			return err
		}
		if err := s.store.ProcessAsyncInvocation(job.InvocationID, 0, func() error {
			fn, resolvedVersion, resolveErr := s.store.ResolveFunction(fnAccount, functionName, qualifier)
			if resolveErr != nil {
				return resolveErr
			}
			_, execErr := s.executeLambdaInvoke(ctx, fnAccount, functionName, fn, resolvedVersion, job.EventJSON)
			return execErr
		}); err != nil {
			return err
		}
		done, err := s.store.GetAsyncInvocation(job.InvocationID)
		if err != nil {
			return err
		}
		if done.Status != "succeeded" {
			msg := strings.TrimSpace(done.LastError)
			if msg == "" {
				msg = "rotation lambda invoke failed"
			}
			return fmt.Errorf("%s", msg)
		}
		return nil
	})
	return err
}
