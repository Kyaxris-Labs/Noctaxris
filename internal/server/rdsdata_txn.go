package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// Lab idle timeout for held Data API transactions (AWS is 3 minutes; lab uses 5).
const rdsDataTxnIdleTimeout = 5 * time.Minute

type rdsDataTxnSession struct {
	conn      *pgx.Conn
	accountID string
	resource  string
	secret    string
	database  string
	expires   time.Time
}

var (
	rdsDataTxnMu sync.Mutex
	rdsDataTxns  = map[string]*rdsDataTxnSession{}
)

func rdsDataTxnKey(accountID, transactionID string) string {
	return accountID + "\x00" + transactionID
}

func (sess *rdsDataTxnSession) close() {
	if sess == nil || sess.conn == nil {
		return
	}
	_ = sess.conn.Close(context.Background())
	sess.conn = nil
}

func registerRDSDataTxnSession(transactionID string, sess *rdsDataTxnSession) {
	rdsDataTxnMu.Lock()
	defer rdsDataTxnMu.Unlock()
	rdsDataTxns[rdsDataTxnKey(sess.accountID, transactionID)] = sess
}

func removeRDSDataTxnSession(accountID, transactionID string) *rdsDataTxnSession {
	rdsDataTxnMu.Lock()
	defer rdsDataTxnMu.Unlock()
	key := rdsDataTxnKey(accountID, transactionID)
	sess := rdsDataTxns[key]
	delete(rdsDataTxns, key)
	return sess
}

// lookupRDSDataTxnSession returns an active held session, refreshing idle expiry.
// Expired sessions are rolled back best-effort and removed.
func (s *Server) lookupRDSDataTxnSession(accountID, transactionID string) (*rdsDataTxnSession, error) {
	transactionID = strings.TrimSpace(transactionID)
	if transactionID == "" {
		return nil, fmt.Errorf("%w: transactionId is required", store.ErrRDSDataBadRequest)
	}
	now := time.Now().UTC()
	rdsDataTxnMu.Lock()
	key := rdsDataTxnKey(accountID, transactionID)
	sess := rdsDataTxns[key]
	if sess == nil {
		rdsDataTxnMu.Unlock()
		return nil, store.ErrRDSDataTxnNotFound
	}
	if !sess.expires.After(now) {
		delete(rdsDataTxns, key)
		rdsDataTxnMu.Unlock()
		s.expireRDSDataTxnSession(sess, accountID, transactionID)
		return nil, store.ErrRDSDataTxnNotFound
	}
	sess.expires = now.Add(rdsDataTxnIdleTimeout)
	rdsDataTxnMu.Unlock()
	return sess, nil
}

func (s *Server) expireRDSDataTxnSession(sess *rdsDataTxnSession, accountID, transactionID string) {
	if sess != nil && sess.conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = sess.conn.Exec(ctx, "ROLLBACK")
		cancel()
		sess.close()
	}
	if s != nil && s.store != nil {
		_ = s.store.FinishRDSDataTransaction(accountID, transactionID, "rolled_back")
	}
}

// BeginRDSDataSQLTransaction dials nested Postgres, runs BEGIN, and registers a held session.
// Fails with store.ErrRDSDataUnavailable when the instance is not available or pgx dial fails.
func (s *Server) BeginRDSDataSQLTransaction(
	ctx context.Context,
	accountID string,
	inst store.RDSDBInstance,
	secretARN, database string,
) (string, error) {
	if strings.TrimSpace(inst.ContainerID) == "" || inst.DBInstanceStatus != "available" {
		return "", store.ErrRDSDataUnavailable
	}
	user, password, err := s.rdsDataMasterCreds(accountID, inst, secretARN)
	if err != nil {
		return "", err
	}
	db := strings.TrimSpace(database)
	if db == "" {
		db = inst.DBName
	}
	if db == "" {
		db = "postgres"
	}
	dsn, err := buildNestedPostgresDSN(inst, user, password, db)
	if err != nil {
		return "", err
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, err := rdsDataPgxConnect(runCtx, dsn)
	if err != nil {
		return "", fmt.Errorf("%w: nested pgx dial: %v", store.ErrRDSDataUnavailable, err)
	}
	if _, err := conn.Exec(runCtx, "BEGIN"); err != nil {
		_ = conn.Close(context.Background())
		return "", err
	}
	txnID, err := s.store.BeginRDSDataTransaction(accountID, inst.DBInstanceARN, secretARN, db)
	if err != nil {
		_, _ = conn.Exec(context.Background(), "ROLLBACK")
		_ = conn.Close(context.Background())
		return "", err
	}
	registerRDSDataTxnSession(txnID, &rdsDataTxnSession{
		conn:      conn,
		accountID: accountID,
		resource:  inst.DBInstanceARN,
		secret:    secretARN,
		database:  db,
		expires:   time.Now().UTC().Add(rdsDataTxnIdleTimeout),
	})
	return txnID, nil
}

// CommitRDSDataSQLTransaction runs COMMIT on the held session and finishes metadata.
func (s *Server) CommitRDSDataSQLTransaction(ctx context.Context, accountID, transactionID, resourceARN, secretARN string) error {
	sess, err := s.lookupRDSDataTxnSession(accountID, transactionID)
	if err != nil {
		return err
	}
	if err := matchRDSDataTxnResource(sess, resourceARN, secretARN); err != nil {
		return err
	}
	// Remove before COMMIT so concurrent callers cannot reuse the session.
	removed := removeRDSDataTxnSession(accountID, transactionID)
	if removed == nil {
		return store.ErrRDSDataTxnNotFound
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = removed.conn.Exec(runCtx, "COMMIT")
	removed.close()
	if err != nil {
		_ = s.store.FinishRDSDataTransaction(accountID, transactionID, "failed")
		return err
	}
	return s.store.FinishRDSDataTransaction(accountID, transactionID, "committed")
}

// RollbackRDSDataSQLTransaction runs ROLLBACK on the held session and finishes metadata.
func (s *Server) RollbackRDSDataSQLTransaction(ctx context.Context, accountID, transactionID, resourceARN, secretARN string) error {
	sess, err := s.lookupRDSDataTxnSession(accountID, transactionID)
	if err != nil {
		return err
	}
	if err := matchRDSDataTxnResource(sess, resourceARN, secretARN); err != nil {
		return err
	}
	removed := removeRDSDataTxnSession(accountID, transactionID)
	if removed == nil {
		return store.ErrRDSDataTxnNotFound
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = removed.conn.Exec(runCtx, "ROLLBACK")
	removed.close()
	if err != nil {
		_ = s.store.FinishRDSDataTransaction(accountID, transactionID, "failed")
		return err
	}
	return s.store.FinishRDSDataTransaction(accountID, transactionID, "rolled_back")
}

// executeRDSDataOnTransaction runs ExecuteStatement SQL on a held pgx transaction session.
func (s *Server) executeRDSDataOnTransaction(
	ctx context.Context,
	accountID string,
	req store.RDSDataExecuteRequest,
) (store.RDSDataExecuteResult, error) {
	sess, err := s.lookupRDSDataTxnSession(accountID, req.TransactionID)
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	if err := matchRDSDataTxnResource(sess, req.ResourceARN, req.SecretARN); err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return executeRDSDataPgxWithConn(runCtx, sess.conn, req)
}

func matchRDSDataTxnResource(sess *rdsDataTxnSession, resourceARN, secretARN string) error {
	resourceARN = strings.TrimSpace(resourceARN)
	secretARN = strings.TrimSpace(secretARN)
	if resourceARN != "" && resourceARN != sess.resource {
		return fmt.Errorf("%w: resourceArn does not match transaction", store.ErrRDSDataBadRequest)
	}
	if secretARN != "" && secretARN != sess.secret {
		return fmt.Errorf("%w: secretArn does not match transaction", store.ErrRDSDataBadRequest)
	}
	return nil
}

// preferNestedRDSDataBatch runs SQL once per parameter set (auto-commit or held txn).
func (s *Server) preferNestedRDSDataBatch(
	ctx context.Context,
	accountID string,
	inst store.RDSDBInstance,
	req store.RDSDataBatchExecuteRequest,
) ([]store.RDSDataExecuteResult, error) {
	if len(req.ParameterSets) == 0 {
		return nil, fmt.Errorf("%w: parameterSets is required", store.ErrRDSDataBadRequest)
	}
	results := make([]store.RDSDataExecuteResult, 0, len(req.ParameterSets))
	if strings.TrimSpace(req.TransactionID) != "" {
		sess, err := s.lookupRDSDataTxnSession(accountID, req.TransactionID)
		if err != nil {
			return nil, err
		}
		if err := matchRDSDataTxnResource(sess, req.ResourceARN, req.SecretARN); err != nil {
			return nil, err
		}
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		for _, params := range req.ParameterSets {
			one := store.RDSDataExecuteRequest{
				ResourceARN:   req.ResourceARN,
				SecretARN:     req.SecretARN,
				Database:      req.Database,
				SQL:           req.SQL,
				TransactionID: req.TransactionID,
				Parameters:    params,
			}
			res, err := executeRDSDataPgxWithConn(runCtx, sess.conn, one)
			if err != nil {
				return nil, err
			}
			results = append(results, res)
		}
		return results, nil
	}
	for _, params := range req.ParameterSets {
		one := store.RDSDataExecuteRequest{
			ResourceARN: req.ResourceARN,
			SecretARN:   req.SecretARN,
			Database:    req.Database,
			SQL:         req.SQL,
			Parameters:  params,
		}
		res, err := s.preferNestedRDSDataExecute(ctx, accountID, inst, one)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}
