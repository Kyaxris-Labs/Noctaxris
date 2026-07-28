package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// athenaEngineMode mirrors compute.ResolveAthenaEngineMode for handler wiring.
func athenaEngineMode() compute.AthenaEngineMode {
	return compute.ResolveAthenaEngineMode()
}

// tryStartAthenaDuck attempts DuckDB execution when the engine mode prefers it.
// used=false means the caller should fall back to the in-process engine.
func (s *Server) tryStartAthenaDuck(accountID string, in store.AthenaStartInput) (exec store.AthenaQueryExecution, used bool, err error) {
	mode := athenaEngineMode()
	if mode == compute.AthenaEngineInProcess {
		return store.AthenaQueryExecution{}, false, nil
	}

	runner, readyErr := s.athenaDuckRunner(accountID)
	if readyErr != nil {
		if mode == compute.AthenaEngineDuckDB {
			// Fail closed when DuckDB was explicitly selected.
			exec, err = s.store.StartAthenaDuckQueryExecution(accountID, in, func(_, _ string) ([]store.AthenaColumnInfo, [][]string, error) {
				return nil, nil, readyErr
			})
			return exec, true, err
		}
		return store.AthenaQueryExecution{}, false, nil
	}

	exec, err = s.store.StartAthenaDuckQueryExecution(accountID, in, runner)
	return exec, true, err
}

func (s *Server) athenaDuckRunner(accountID string) (store.AthenaDuckRunner, error) {
	if url := compute.ConfiguredDuckURL(); url != "" {
		if !compute.ProbeDuckHealth(context.Background(), http.DefaultClient, url) {
			return nil, fmt.Errorf("DuckDB URL %s is not healthy", url)
		}
		return func(querySQL, setupSQL string) ([]store.AthenaColumnInfo, [][]string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			body := compute.NewDuckQueryRequest(querySQL, setupSQL, accountID)
			res, err := compute.QueryDuckHTTP(ctx, http.DefaultClient, url, body)
			if err != nil {
				return nil, nil, err
			}
			return duckResultToAthena(res), res.Rows, nil
		}, nil
	}

	if strings.TrimSpace(s.cfg.DockerHost) == "" {
		return nil, fmt.Errorf("DuckDB nested engine unavailable (NOCTAXRIS_DOCKER_HOST empty and NOCTAXRIS_DUCKDB_URL unset)")
	}
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		if err == nil {
			err = fmt.Errorf("compute client unavailable")
		}
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	inst, err := cli.EnsureDuckDB(ctx)
	if err != nil {
		return nil, err
	}
	if err := cli.WaitDuckHealthy(ctx, inst.ContainerID, 45*time.Second); err != nil {
		return nil, err
	}
	cid := inst.ContainerID
	return func(querySQL, setupSQL string) ([]store.AthenaColumnInfo, [][]string, error) {
		qctx, qcancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer qcancel()
		body := compute.NewDuckQueryRequest(querySQL, setupSQL, accountID)
		res, err := cli.ExecDuckQuery(qctx, cid, body)
		if err != nil {
			return nil, nil, err
		}
		return duckResultToAthena(res), res.Rows, nil
	}, nil
}

func duckResultToAthena(res compute.DuckQueryResult) []store.AthenaColumnInfo {
	cols := make([]store.AthenaColumnInfo, len(res.Columns))
	for i, name := range res.Columns {
		cols[i] = store.AthenaColumnInfo{Name: name, Type: "varchar"}
	}
	return cols
}
