package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// athenaEngineMode mirrors compute.ResolveAthenaEngineMode for handler wiring.
func athenaEngineMode() compute.AthenaEngineMode {
	return compute.ResolveAthenaEngineMode()
}

// tryStartAthenaDuck attempts DuckDB execution when the engine mode prefers it.
// used=false means the caller should fall back to the in-process engine.
func (s *Server) tryStartAthenaDuck(verified *authn.Verified, accountID string, in store.AthenaStartInput) (exec store.AthenaQueryExecution, used bool, err error) {
	mode := athenaEngineMode()
	if mode == compute.AthenaEngineInProcess {
		return store.AthenaQueryExecution{}, false, nil
	}

	runner, readyErr := s.athenaDuckRunner(accountID)
	if readyErr != nil {
		if mode == compute.AthenaEngineDuckDB {
			if authErr := s.authorizeAthenaDuckS3Reads(verified, accountID, in); authErr != nil {
				return store.AthenaQueryExecution{}, true, authErr
			}
			// Fail closed when DuckDB was explicitly selected.
			exec, err = s.store.StartAthenaDuckQueryExecution(accountID, in, func(_, _ string) ([]store.AthenaColumnInfo, [][]string, error) {
				return nil, nil, readyErr
			})
			return exec, true, err
		}
		return store.AthenaQueryExecution{}, false, nil
	}

	if err := s.authorizeAthenaDuckS3Reads(verified, accountID, in); err != nil {
		return store.AthenaQueryExecution{}, true, err
	}

	exec, err = s.store.StartAthenaDuckQueryExecution(accountID, in, runner)
	return exec, true, err
}

// authorizeAthenaDuckS3Reads requires caller s3:GetObject on each object key under
// Glue table StorageLocation prefixes that Duck will scan.
func (s *Server) authorizeAthenaDuckS3Reads(verified *authn.Verified, accountID string, in store.AthenaStartInput) error {
	if verified == nil {
		return fmt.Errorf("%w: authenticated caller required for DuckDB S3 reads", store.ErrAthenaAccessDenied)
	}
	dbName, err := store.ResolveAthenaDuckDatabase(in)
	if err != nil {
		return err
	}
	tables, err := s.store.ListAthenaDuckTables(accountID, dbName)
	if err != nil {
		return err
	}
	for _, t := range tables {
		if strings.TrimSpace(t.Location) == "" {
			continue
		}
		bucket, prefix, locErr := store.ParseS3Location(t.Location)
		if locErr != nil {
			return locErr
		}
		ref, refErr := s.s3ResolveBucket(bucket)
		if refErr != nil {
			return fmt.Errorf("%w: %v", store.ErrAthenaBadRequest, refErr)
		}
		listed, listErr := s.store.ListObjectsV2(accountID, bucket, prefix, "")
		if listErr != nil {
			return fmt.Errorf("%w: ListObjects for s3://%s/%s: %v", store.ErrAthenaBadRequest, bucket, prefix, listErr)
		}
		if len(listed.Contents) == 0 {
			objectARN := store.ObjectARN(bucket, strings.TrimPrefix(prefix, "/"))
			if objectARN == store.ObjectARN(bucket, "") {
				objectARN = store.ObjectARN(bucket, "*")
			}
			if !s.authorizeS3(verified, catalog.ActionS3GetObject, objectARN, ref.policy, ref.accountID) {
				return fmt.Errorf("%w: not authorized to perform s3:GetObject on %s", store.ErrAthenaAccessDenied, objectARN)
			}
			continue
		}
		for _, obj := range listed.Contents {
			objectARN := store.ObjectARN(bucket, obj.Key)
			if !s.authorizeS3(verified, catalog.ActionS3GetObject, objectARN, ref.policy, ref.accountID) {
				return fmt.Errorf("%w: not authorized to perform s3:GetObject on %s", store.ErrAthenaAccessDenied, objectARN)
			}
		}
	}
	return nil
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
