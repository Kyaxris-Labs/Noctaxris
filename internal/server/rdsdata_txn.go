package server

import (
	"context"
	"database/sql"
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
	engine    string
	conn      *pgx.Conn
	mysqlConn *sql.Conn
	mysqlTx   *sql.Tx
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
	if sess == nil {
		return
	}
	if sess.conn != nil {
		_ = sess.conn.Close(context.Background())
		sess.conn = nil
	}
	if sess.mysqlTx != nil {
		_ = sess.mysqlTx.Rollback()
		sess.mysqlTx = nil
	}
	if sess.mysqlConn != nil {
		_ = sess.mysqlConn.Close()
		sess.mysqlConn = nil
	}
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
	if sess != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if sess.conn != nil {
			_, _ = sess.conn.Exec(ctx, "ROLLBACK")
		}
		if sess.mysqlTx != nil {
			_ = sess.mysqlTx.Rollback()
		}
		cancel()
		sess.close()
	}
	if s != nil && s.store != nil {
		_ = s.store.FinishRDSDataTransaction(accountID, transactionID, "rolled_back")
	}
}

// BeginRDSDataSQLTransaction dials the nested engine, runs BEGIN, and registers a held session.
// Fails with store.ErrRDSDataUnavailable when the instance is not available or wire dial fails.
func (s *Server) BeginRDSDataSQLTransaction(
	ctx context.Context,
	accountID string,
	inst store.RDSDBInstance,
	secretARN, database string,
) (string, error) {
	if strings.TrimSpace(inst.ContainerID) == "" || inst.DBInstanceStatus != "available" {
		return "", store.ErrRDSDataUnavailable
	}
	engine := store.NormalizeRDSEngine(inst.Engine)
	switch engine {
	case "postgres":
		return s.beginRDSDataPostgresTxn(ctx, accountID, inst, secretARN, database)
	case "mysql", "mariadb":
		return s.beginRDSDataMySQLTxn(ctx, accountID, inst, secretARN, database)
	default:
		return "", store.ErrRDSDataUnavailable
	}
}

func (s *Server) beginRDSDataPostgresTxn(
	ctx context.Context,
	accountID string,
	inst store.RDSDBInstance,
	secretARN, database string,
) (string, error) {
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
		engine:    "postgres",
		conn:      conn,
		accountID: accountID,
		resource:  inst.DBInstanceARN,
		secret:    secretARN,
		database:  db,
		expires:   time.Now().UTC().Add(rdsDataTxnIdleTimeout),
	})
	return txnID, nil
}

func (s *Server) beginRDSDataMySQLTxn(
	ctx context.Context,
	accountID string,
	inst store.RDSDBInstance,
	secretARN, database string,
) (string, error) {
	if !rdsDataPgxEnabled() {
		return "", store.ErrRDSDataUnavailable
	}
	user, password, err := s.rdsDataMasterCreds(accountID, inst, secretARN)
	if err != nil {
		return "", err
	}
	db := rdsDataDatabaseName(inst, database, "mysql")
	dsn, err := buildNestedMySQLDSN(inst, user, password, db)
	if err != nil {
		return "", err
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	sqlConn, err := rdsDataMySQLConnect(runCtx, dsn)
	if err != nil {
		return "", fmt.Errorf("%w: nested mysql dial: %v", store.ErrRDSDataUnavailable, err)
	}
	tx, err := sqlConn.BeginTx(runCtx, nil)
	if err != nil {
		_ = sqlConn.Close()
		return "", err
	}
	txnID, err := s.store.BeginRDSDataTransaction(accountID, inst.DBInstanceARN, secretARN, db)
	if err != nil {
		_ = tx.Rollback()
		_ = sqlConn.Close()
		return "", err
	}
	registerRDSDataTxnSession(txnID, &rdsDataTxnSession{
		engine:    store.NormalizeRDSEngine(inst.Engine),
		mysqlConn: sqlConn,
		mysqlTx:   tx,
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
	if removed.mysqlTx != nil {
		err = removed.mysqlTx.Commit()
		removed.close()
	} else {
		_, err = removed.conn.Exec(runCtx, "COMMIT")
		removed.close()
	}
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
	if removed.mysqlTx != nil {
		err = removed.mysqlTx.Rollback()
		removed.close()
	} else {
		_, err = removed.conn.Exec(runCtx, "ROLLBACK")
		removed.close()
	}
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
	if sess.mysqlTx != nil {
		return executeRDSDataMySQLWithTx(runCtx, sess.mysqlTx, req)
	}
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
			var res store.RDSDataExecuteResult
			var err error
			if sess.mysqlTx != nil {
				res, err = executeRDSDataMySQLWithTx(runCtx, sess.mysqlTx, one)
			} else {
				res, err = executeRDSDataPgxWithConn(runCtx, sess.conn, one)
			}
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
