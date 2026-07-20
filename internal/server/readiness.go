package server

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	if err := s.store.Ping(); err != nil {
		http.Error(w, "store not ready", http.StatusServiceUnavailable)
		return
	}
	if host := strings.TrimSpace(s.cfg.DockerHost); host != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cli, err := s.computeClient()
		if err != nil {
			http.Error(w, "engine client not ready", http.StatusServiceUnavailable)
			return
		}
		if err := cli.Ping(ctx); err != nil {
			http.Error(w, "engine not ready", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}
