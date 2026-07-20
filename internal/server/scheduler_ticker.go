package server

import (
	"context"
	"log"
	"time"
)

const schedulerTickerInterval = 5 * time.Second

// StartSchedulerTicker starts the in-process due-schedule worker once.
// Safe to call multiple times. Stop via StopSchedulerTicker or process exit.
func (s *Server) StartSchedulerTicker() {
	s.schedulerTickerOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.schedulerTickerCancel = cancel
		go s.runSchedulerTicker(ctx)
	})
}

// StopSchedulerTicker stops the in-process scheduler ticker if running.
func (s *Server) StopSchedulerTicker() {
	if s.schedulerTickerCancel != nil {
		s.schedulerTickerCancel()
	}
}

func (s *Server) runSchedulerTicker(ctx context.Context) {
	ticker := time.NewTicker(schedulerTickerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := s.now().UTC()
			if _, err := s.store.ProcessDueSchedules(now); err != nil {
				log.Printf("scheduler ticker: %v", err)
			}
		}
	}
}
