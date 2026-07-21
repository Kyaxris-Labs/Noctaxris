package server

import (
	"context"
	"log"
	"time"
)

const pipesTickerInterval = time.Second

// StartPipesTicker starts the in-process PollPipeOnce worker once.
// Safe to call multiple times. Mirrors Scheduler / ESM poll patterns.
func (s *Server) StartPipesTicker() {
	s.pipesTickerOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.pipesTickerCancel = cancel
		go s.runPipesTicker(ctx)
	})
}

// StopPipesTicker stops the in-process pipes ticker if running.
func (s *Server) StopPipesTicker() {
	if s.pipesTickerCancel != nil {
		s.pipesTickerCancel()
	}
}

func (s *Server) runPipesTicker(ctx context.Context) {
	ticker := time.NewTicker(pipesTickerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollAllRunningPipes()
		}
	}
}

func (s *Server) pollAllRunningPipes() {
	pipes, err := s.store.ListRunningPipes()
	if err != nil || len(pipes) == 0 {
		return
	}
	for _, entry := range pipes {
		err := s.store.PollPipeOnce(entry.AccountID, entry.Pipe.Name, func(accountID, functionName, payloadJSON string) (string, error) {
			fn, executedVersion, err := s.store.ResolveFunction(accountID, functionName, "$LATEST")
			if err != nil {
				return "", err
			}
			out, err := s.executeLambdaInvoke(context.Background(), accountID, functionName, fn, executedVersion, payloadJSON)
			if err != nil {
				return "", err
			}
			return string(out), nil
		})
		if err != nil {
			log.Printf("pipes ticker: pipe=%s err=%v", entry.Pipe.ARN, err)
		}
	}
}
