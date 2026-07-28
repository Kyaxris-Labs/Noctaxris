package server

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/Kyaxris-Labs/Noctaxris/internal/version"
	"github.com/google/uuid"
)

const (
	eventVersion    = "1.11"
	healthPath      = "/_noctaxris/health"
	readyPath       = "/_noctaxris/ready"
	versionPath     = "/_noctaxris/version"
	requestIDHeader = "x-amz-request-id"
	maxBodyBytes    = 1 << 20  // 1 MiB
	maxS3BodyBytes  = 16 << 20 // 16 MiB lab PutObject
	sigv4Skew       = 15 * time.Minute
	shutdownTimeout = 10 * time.Second
)

type Server struct {
	cfg   config.Config
	store *store.Store
	audit *audit.Writer
	now   func() time.Time

	// Lazy nested-engine client for ECS / Registry (DinD, cfg.DockerHost).
	computeOnce sync.Once
	compute     *compute.Client
	computeErr  error

	// Lazy Lambda invoker (nested DinD via cfg.DockerHost).
	invokerOnce sync.Once
	invoker     compute.FunctionInvoker
	invokerErr  error

	// Optional unit-test hook replacing nested DinD invoke.
	lambdaInvokeHook func(ctx context.Context, accountID, name string, fn store.LambdaFunction, executedVersion, eventJSON string) ([]byte, error)

	// Optional unit-test hook replacing nested DinD docker exec for SSM Run Command.
	ssmExecHook func(ctx context.Context, containerID string, commands []string, timeoutSeconds int) (compute.ExecResult, error)

	// In-process EventBridge Scheduler ticker (ADR-0007).
	schedulerTickerOnce   sync.Once
	schedulerTickerCancel context.CancelFunc

	// In-process EventBridge Pipes poller.
	pipesTickerOnce   sync.Once
	pipesTickerCancel context.CancelFunc

	// In-process Secrets Manager RotationRules ticker.
	secretsRotationTickerOnce   sync.Once
	secretsRotationTickerCancel context.CancelFunc

	clockMu       sync.RWMutex
	clockOverride *time.Time
}

type awsError struct {
	XMLName xml.Name `xml:"Error"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
	Type    string   `xml:"Type"`
}

type awsErrorResponse struct {
	XMLName   xml.Name `xml:"ErrorResponse"`
	XMLNS     string   `xml:"xmlns,attr"`
	Error     awsError `xml:"Error"`
	RequestID string   `xml:"RequestId"`
}

func New(cfg config.Config, st *store.Store, aud *audit.Writer) *Server {
	s := &Server{
		cfg:   cfg,
		store: st,
		audit: aud,
	}
	s.now = func() time.Time { return s.effectiveNow() }
	// CS-016: any EnqueueAsyncInvoke (SNS/EB/Scheduler/Firehose/Invoke Event) starts the worker.
	st.SetOnAsyncEnqueue(func(job store.LambdaAsyncInvocation) {
		s.startAsyncInvoke(job, job.AccountID, job.FunctionName, "")
	})
	if aud != nil {
		dataRoot := cfg.DataRoot
		aud.SetAfterWrite(func() {
			if err := st.ShipCloudTrailContinuousDeliveries(dataRoot); err != nil {
				log.Printf("cloudtrail continuous delivery: %v", err)
			}
		})
	}
	st.SetCognitoInsecureCodes(cfg.CognitoInsecureCodes)
	s.wireCognitoTriggerInvoker()
	s.wireCURDuckRunner()
	return s
}

// SetLambdaInvokeHookForTest replaces nested DinD invoke for unit tests.
// Pass nil to clear. Not for production use.
func (s *Server) SetLambdaInvokeHookForTest(
	hook func(ctx context.Context, accountID, name string, fn store.LambdaFunction, executedVersion, eventJSON string) ([]byte, error),
) {
	s.lambdaInvokeHook = hook
}

// SetSSMExecHookForTest replaces nested DinD docker exec for SSM Run Command unit tests.
// Pass nil to clear. Not for production use.
func (s *Server) SetSSMExecHookForTest(
	hook func(ctx context.Context, containerID string, commands []string, timeoutSeconds int) (compute.ExecResult, error),
) {
	s.ssmExecHook = hook
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.serveHTTP)
}

func (s *Server) ListenAndServe() error {
	return s.ListenAndServeContext(context.Background())
}

// ListenAndServeContext serves until ctx is cancelled, then drains tickers and
// shuts down the HTTP server with a timeout.
func (s *Server) ListenAndServeContext(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.ListenAddr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		var err error
		if s.cfg.TLSCertFile != "" && s.cfg.TLSKeyFile != "" {
			err = srv.ListenAndServeTLS(s.cfg.TLSCertFile, s.cfg.TLSKeyFile)
		} else {
			err = srv.ListenAndServe()
		}
		errCh <- err
	}()
	s.startSharedMQTTIfEnabled()

	select {
	case <-ctx.Done():
		s.StopBackgroundWorkers()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// StopBackgroundWorkers stops in-process Scheduler, ESM, Pipes, Secrets rotation, and ECS/ASG reconciler tickers.
func (s *Server) StopBackgroundWorkers() {
	s.StopSchedulerTicker()
	s.StopESMPoller()
	s.StopPipesTicker()
	s.StopSecretsRotationTicker()
	s.StopECSServiceReconciler()
	s.StopASGReconciler()
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == healthPath {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == readyPath {
		s.handleReady(w, r)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == versionPath {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(version.Version + "\n"))
		return
	}

	if r.URL.Path == store.LabSNSHTTPCatcherPath || strings.HasPrefix(r.URL.Path, store.LabSNSHTTPCatcherPath+"/") {
		s.handleSNSHTTPCatcher(w, r)
		return
	}

	if isLabCodeBuildWebhookPath(r.URL.Path) {
		s.handleLabCodeBuildWebhook(w, r)
		return
	}

	if isRegistryV2Path(r.URL.Path) {
		s.handleRegistryV2(w, r)
		return
	}

	if isFunctionURLPath(r.URL.Path) {
		s.handleFunctionURLInvoke(w, r)
		return
	}

	if isCognitoJWKSPath(r.URL.Path) {
		s.handleCognitoJWKS(w, r)
		return
	}

	requestID := newRequestID()
	eventID := newRequestID()
	readOnly := r.Method == http.MethodGet || r.Method == http.MethodHead

	bodyLimit := bodyLimitForRequest(r)
	body, err := readBody(r, bodyLimit)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusBadRequest, "InvalidRequest",
			"Unable to read request body.", readOnly, r, eventID, "", "", false)
		return
	}

	if isAppSyncGraphQLPath(r.URL.Path) {
		s.handleAppSyncGraphQLRuntime(w, r, body, requestID, eventID, readOnly)
		return
	}

	if isHTTPAPIInvokePath(r.URL.Path) {
		s.handleHTTPAPIInvoke(w, r, body, requestID, eventID, readOnly)
		return
	}

	if isRestAPIInvokePath(r.URL.Path) {
		s.handleRestAPIInvoke(w, r, body, requestID, eventID, readOnly)
		return
	}

	if isWebSocketAPILabPath(r.URL.Path) {
		s.handleWebSocketAPILabInvoke(w, r, body, requestID, eventID, readOnly)
		return
	}

	if isAPIGatewayConnectionsPath(r.URL.Path) {
		s.handleAPIGatewayConnections(w, r, body, requestID, eventID, readOnly)
		return
	}

	if isELBv2LabListenerPath(r.URL.Path) {
		s.handleELBv2LabListener(w, r, body, requestID, eventID, readOnly)
		return
	}

	if isELBv2NLBLabListenerPath(r.URL.Path) {
		s.handleELBv2NLBLabListener(w, r, body, requestID, eventID, readOnly)
		return
	}

	action := resolveAction(r, body)
	var verified *authn.Verified
	if isUnauthenticatedSTSAction(action) {
		// AWS STS federation APIs authenticate via SAML/OIDC token, not SigV4.
		verified = &authn.Verified{Region: federationRegion(r)}
	} else if isUnauthenticatedCognitoAction(action) {
		// Cognito public IdP APIs (InitiateAuth, RevokeToken, MFA associate/verify/challenge); AWS CLI omits Authorization.
		verified = &authn.Verified{Region: federationRegion(r), Service: "cognito-idp"}
	} else {
		var err error
		verified, err = authn.Verify(r, body, s.authClock(), sigv4Skew, s.lookupKey)
		if err != nil {
			code := authn.Code(err)
			// Narrow anonymous S3 GetObject/HeadObject gate: only MissingAuthenticationToken,
			// only object GET/HEAD, and only when env + policy/ACL allow that object.
			if code == authn.CodeMissingAuthenticationToken &&
				s.tryAnonymousS3Object(w, r, body, requestID, eventID, readOnly) {
				return
			}
			if code == "" {
				code = authn.CodeInvalidClientTokenId
			}
			msg := defaultAuthnMessage(code)
			accessKeyID := ""
			if ak, ok := parseAccessKeyID(r.Header.Get("Authorization")); ok {
				accessKeyID = ak
			}
			s.writeAWSError(w, requestID, http.StatusForbidden, code, msg, readOnly, r, eventID, accessKeyID, "", false)
			return
		}
		verified.SourceIP = peerClientIP(r)
		s.recordAccessKeyLastUsed(verified)
	}

	// Lab facades on :4566 after SigV4 (account-scoped; no anonymous edge/query/file paths).
	if isCloudFrontEdgePath(r.URL.Path) {
		s.handleCloudFrontEdgeAfterAuth(w, r, body, requestID, eventID, verified, readOnly)
		return
	}
	if isOpenSearchLabQueryPath(r.URL.Path) {
		s.handleOpenSearchLabQuery(w, r, body, requestID, eventID, verified, readOnly)
		return
	}
	if IsTransferLabHomePath(r.URL.Path) {
		s.handleTransferLabHome(w, r, body, requestID, eventID, verified, readOnly)
		return
	}

	if action == "" && (strings.EqualFold(verified.Service, "lambda") || isLambdaRESTPath(r.URL.Path)) {
		restAction, body2 := resolveLambdaREST(r, body)
		if restAction != "" {
			s.handleLambda(w, r, body2, requestID, eventID, restAction, verified, readOnly)
			return
		}
	}

	if action == "" && isAPIGatewayRESTMgmtPath(r.URL.Path) &&
		strings.EqualFold(verified.Service, "apigateway") {
		restAction, body2 := resolveAPIGatewayREST(r, body)
		if restAction != "" {
			s.handleAPIGatewayREST(w, r, body2, requestID, eventID, restAction, verified, readOnly)
			return
		}
	}

	if action == "" && isAPIGatewayV2RESTPath(r.URL.Path) &&
		(strings.EqualFold(verified.Service, "apigateway") ||
			strings.EqualFold(verified.Service, "apigatewayv2")) {
		restAction, body2 := resolveAPIGatewayV2REST(r, body)
		if restAction != "" {
			s.handleAPIGatewayV2(w, r, body2, requestID, eventID, restAction, verified, readOnly)
			return
		}
	}

	if action == "" && (strings.EqualFold(verified.Service, "batch") || isBatchRESTPath(r.URL.Path)) {
		if restAction := resolveBatchREST(r); restAction != "" {
			s.handleBatch(w, r, body, requestID, eventID, restAction, verified, readOnly)
			return
		}
	}

	if action == "" && (strings.EqualFold(verified.Service, "backup") || isBackupRESTPath(r.URL.Path)) {
		if restAction, pathParams := resolveBackupREST(r); restAction != "" {
			merged := backupMergeParams(pathParams, jsonBodyMap(body))
			raw, _ := json.Marshal(merged)
			s.handleBackup(w, r, raw, requestID, eventID, restAction, verified, readOnly)
			return
		}
	}

	if action == "" && (strings.EqualFold(verified.Service, "eks") || isEKSRESTPath(r.URL.Path)) {
		if restAction := resolveEKSREST(r); restAction != "" {
			s.handleEKS(w, r, body, requestID, eventID, restAction, verified, readOnly)
			return
		}
	}

	if action == "" && (strings.EqualFold(verified.Service, "bedrock") ||
		strings.EqualFold(verified.Service, "bedrock-runtime") ||
		isBedrockRuntimePath(r.URL.Path)) {
		if restAction, modelID := resolveBedrockRuntimeREST(r); restAction != "" {
			s.handleBedrockRuntime(w, r, body, requestID, eventID, restAction, verified, readOnly, modelID)
			return
		}
	}

	if action == "" && (strings.EqualFold(verified.Service, "ses") ||
		strings.EqualFold(verified.Service, "email") ||
		isSESV2RESTPath(r.URL.Path)) {
		if restAction, identity := resolveSESV2REST(r); restAction != "" {
			s.handleSESV2(w, r, body, requestID, eventID, restAction, verified, readOnly, identity)
			return
		}
	}

	if action == "" && (strings.EqualFold(verified.Service, "kafka") || strings.EqualFold(verified.Service, "msk")) {
		s.handleMSK(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if action == "" && (verified.Service == "s3" || isS3PathStyleRequest(r, body, action)) {
		s.handleS3(w, r, body, requestID, eventID, verified, readOnly)
		return
	}

	if isOrgsDepthAction(action, verified.Service) {
		s.handleOrgsDepth(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if isControlTowerAction(action, verified.Service) {
		s.handleControlTower(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "sns" || strings.HasPrefix(action, "sns:") {
		s.handleSNS(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "ses" || verified.Service == "email" || strings.HasPrefix(action, "ses:") {
		s.handleSES(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "cloudformation" || strings.HasPrefix(action, "cloudformation:") {
		s.handleCloudFormation(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "config" || strings.HasPrefix(action, "config:") {
		s.handleConfig(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "dynamodbstreams") || strings.HasPrefix(action, "dynamodbstreams:") {
		s.handleDynamoDBStreams(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "pipes") || strings.HasPrefix(action, "pipes:") {
		s.StartPipesTicker()
		s.handlePipes(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "mq") || strings.HasPrefix(action, "mq:") {
		s.handleMQ(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "athena") || strings.HasPrefix(action, "athena:") {
		s.handleAthena(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "es") || strings.EqualFold(verified.Service, "opensearch") ||
		strings.HasPrefix(action, "es:") {
		s.handleOpenSearch(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "elasticache") || strings.HasPrefix(action, "elasticache:") {
		s.handleElastiCache(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "memorydb") || strings.HasPrefix(action, "memorydb:") {
		s.handleMemoryDB(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "neptune") ||
		strings.HasPrefix(action, "neptune:") ||
		(strings.EqualFold(verified.Service, "neptune") && isNeptuneControlPlaneAction(action)) {
		s.handleNeptune(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "kafka") || strings.EqualFold(verified.Service, "msk") ||
		strings.HasPrefix(action, "kafka:") {
		s.handleMSK(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "docdb") ||
		strings.HasPrefix(action, "docdb:") ||
		(strings.EqualFold(verified.Service, "rds") && isDocDBControlPlaneAction(action)) {
		s.handleDocDB(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "rds-data") || strings.HasPrefix(action, "rds-data:") {
		s.handleRDSData(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "rds") || strings.HasPrefix(action, "rds:") {
		s.handleRDS(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "transfer") || strings.HasPrefix(action, "transfer:") {
		s.handleTransfer(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "elasticbeanstalk") || strings.HasPrefix(action, "elasticbeanstalk:") {
		s.handleElasticBeanstalk(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "autoscaling") || strings.HasPrefix(action, "autoscaling:") {
		s.handleAutoScaling(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "lightsail") || strings.HasPrefix(action, "lightsail:") {
		s.handleLightsail(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "backup") || strings.HasPrefix(action, "backup:") {
		s.handleBackup(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "eks") || strings.HasPrefix(action, "eks:") {
		s.handleEKS(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "glue") || strings.HasPrefix(action, "glue:") {
		s.handleGlue(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "appconfig") || strings.EqualFold(verified.Service, "appconfigdata") ||
		strings.HasPrefix(action, "appconfig:") || strings.HasPrefix(action, "appconfigdata:") {
		s.handleAppConfig(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "events" || strings.HasPrefix(action, "events:") {
		s.handleEventBridge(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "scheduler" || strings.HasPrefix(action, "scheduler:") {
		s.StartSchedulerTicker()
		s.handleScheduler(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "acm" || strings.HasPrefix(action, "acm:") {
		s.handleACM(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "noctaxris") || strings.HasPrefix(action, "noctaxris-lab:") {
		s.handleLabForensics(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "guardduty") || strings.HasPrefix(action, "guardduty:") {
		s.handleGuardDuty(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "detective") || strings.HasPrefix(action, "detective:") {
		s.handleDetective(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "macie2") || strings.EqualFold(verified.Service, "macie") ||
		strings.HasPrefix(action, "macie2:") {
		s.handleMacie(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "ec2") || strings.HasPrefix(action, "ec2:") {
		s.handleEC2(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if strings.EqualFold(verified.Service, "securityhub") || strings.HasPrefix(action, "securityhub:") {
		s.handleSecurityHub(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "route53" || strings.HasPrefix(action, "route53:") {
		s.handleRoute53(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "servicediscovery" || strings.HasPrefix(action, "servicediscovery:") {
		s.handleServiceDiscovery(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "pricing" || strings.HasPrefix(action, "pricing:") {
		s.handlePricing(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "appsync" || strings.HasPrefix(action, "appsync:") {
		s.handleAppSync(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "apigateway" ||
		strings.EqualFold(verified.Service, "apigatewayv2") ||
		strings.HasPrefix(action, "apigatewayv2:") ||
		strings.HasPrefix(action, "apigateway:") {
		if isAPIGatewayRESTAction(action) ||
			(strings.HasPrefix(action, "apigateway:") && !strings.HasPrefix(action, "apigatewayv2:")) {
			s.handleAPIGatewayREST(w, r, body, requestID, eventID, action, verified, readOnly)
			return
		}
		s.handleAPIGatewayV2(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "cognito-idp" || strings.HasPrefix(action, "cognito-idp:") {
		s.handleCognito(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	// AWS SDK Go v2 signs Cloud Control as cloudcontrolapi with X-Amz-Target CloudApiService.*.
	if verified.Service == "cloudcontrol" || verified.Service == "cloudcontrolapi" ||
		strings.HasPrefix(action, "cloudcontrol:") {
		s.handleCloudControl(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "bcm-data-exports" || strings.HasPrefix(action, "bcm-data-exports:") {
		s.handleBCMExports(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "ce" || strings.HasPrefix(action, "ce:") {
		s.handleCostExplorer(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "budgets" || strings.HasPrefix(action, "budgets:") {
		s.handleBudgets(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "cur" || strings.HasPrefix(action, "cur:") {
		s.handleCUR(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "iot" || verified.Service == "iotdata" || verified.Service == "iot-data" ||
		verified.Service == "data.iot" || strings.HasPrefix(action, "iot:") || strings.HasPrefix(action, "iot-data:") {
		s.handleIoT(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "monitoring" ||
		verified.Service == "cloudwatch" ||
		strings.HasPrefix(action, "cloudwatch:") {
		s.handleCloudWatch(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "codedeploy" || strings.HasPrefix(action, "codedeploy:") {
		s.handleCodeDeploy(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "cloudfront" || strings.HasPrefix(action, "cloudfront:") {
		s.handleCloudFront(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "elasticloadbalancing" || strings.HasPrefix(action, "elasticloadbalancing:") {
		s.handleELBv2(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "s3vectors" || strings.HasPrefix(action, "s3vectors:") {
		s.handleS3Vectors(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "bedrock" ||
		verified.Service == "bedrock-runtime" ||
		strings.HasPrefix(action, "bedrock:") {
		s.handleBedrockRuntime(w, r, body, requestID, eventID, action, verified, readOnly, "")
		return
	}

	if verified.Service == "textract" || strings.HasPrefix(action, "textract:") {
		s.handleTextract(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "transcribe" || strings.HasPrefix(action, "transcribe:") {
		s.handleTranscribe(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "elasticmapreduce" || strings.HasPrefix(action, "elasticmapreduce:") {
		s.handleEMR(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "ecs" || strings.HasPrefix(action, "ecs:") {
		s.handleECS(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	switch action {
	case catalog.ActionSTSGetCallerIdentity, "GetCallerIdentity":
		s.handleGetCallerIdentity(w, r, requestID, eventID, verified)
	case catalog.ActionOrgsCreateAccount, "CreateAccount":
		s.handleCreateAccount(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionOrgsDescribeCreateAccountStatus, "DescribeCreateAccountStatus":
		s.handleDescribeCreateAccountStatus(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSAssumeRole, "AssumeRole":
		s.handleAssumeRole(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetSessionToken, "GetSessionToken":
		s.handleGetSessionToken(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetFederationToken, "GetFederationToken":
		s.handleGetFederationToken(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetAccessKeyInfo, "GetAccessKeyInfo":
		s.handleGetAccessKeyInfo(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSDecodeAuthorizationMessage, "DecodeAuthorizationMessage":
		s.handleDecodeAuthorizationMessage(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSAssumeRoot, "AssumeRoot":
		s.handleAssumeRoot(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSAssumeRoleWithSAML, "AssumeRoleWithSAML":
		s.handleAssumeRoleWithSAML(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSAssumeRoleWithWebIdentity, "AssumeRoleWithWebIdentity":
		s.handleAssumeRoleWithWebIdentity(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetDelegatedAccessToken, "GetDelegatedAccessToken":
		s.handleGetDelegatedAccessToken(w, r, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetWebIdentityToken, "GetWebIdentityToken":
		s.handleGetWebIdentityToken(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionIAMCreateUser, "CreateUser",
		catalog.ActionIAMGetUser, "GetUser",
		catalog.ActionIAMListUsers, "ListUsers",
		catalog.ActionIAMDeleteUser, "DeleteUser",
		catalog.ActionIAMCreateAccessKey, "CreateAccessKey",
		catalog.ActionIAMDeleteAccessKey, "DeleteAccessKey",
		catalog.ActionIAMListAccessKeys, "ListAccessKeys",
		catalog.ActionIAMUpdateAccessKey, "UpdateAccessKey",
		catalog.ActionIAMGetAccessKeyLastUsed, "GetAccessKeyLastUsed",
		catalog.ActionIAMGenerateCredentialReport, "GenerateCredentialReport",
		catalog.ActionIAMGetCredentialReport, "GetCredentialReport",
		catalog.ActionIAMCreatePolicy, "CreatePolicy",
		catalog.ActionIAMGetPolicy, "GetPolicy",
		catalog.ActionIAMListPolicies, "ListPolicies",
		catalog.ActionIAMDeletePolicy, "DeletePolicy",
		catalog.ActionIAMCreatePolicyVersion, "CreatePolicyVersion",
		catalog.ActionIAMGetPolicyVersion, "GetPolicyVersion",
		catalog.ActionIAMListPolicyVersions, "ListPolicyVersions",
		catalog.ActionIAMDeletePolicyVersion, "DeletePolicyVersion",
		catalog.ActionIAMSetDefaultPolicyVersion, "SetDefaultPolicyVersion",
		catalog.ActionIAMAttachUserPolicy, "AttachUserPolicy",
		catalog.ActionIAMDetachUserPolicy, "DetachUserPolicy",
		catalog.ActionIAMAttachRolePolicy, "AttachRolePolicy",
		catalog.ActionIAMDetachRolePolicy, "DetachRolePolicy",
		catalog.ActionIAMListAttachedUserPolicies, "ListAttachedUserPolicies",
		catalog.ActionIAMListAttachedRolePolicies, "ListAttachedRolePolicies",
		catalog.ActionIAMPutUserPolicy, "PutUserPolicy",
		catalog.ActionIAMGetUserPolicy, "GetUserPolicy",
		catalog.ActionIAMDeleteUserPolicy, "DeleteUserPolicy",
		catalog.ActionIAMListUserPolicies, "ListUserPolicies",
		catalog.ActionIAMPutRolePolicy, "PutRolePolicy",
		catalog.ActionIAMGetRolePolicy, "GetRolePolicy",
		catalog.ActionIAMDeleteRolePolicy, "DeleteRolePolicy",
		catalog.ActionIAMListRolePolicies, "ListRolePolicies",
		catalog.ActionIAMCreateRole, "CreateRole",
		catalog.ActionIAMGetRole, "GetRole",
		catalog.ActionIAMListRoles, "ListRoles",
		catalog.ActionIAMDeleteRole, "DeleteRole",
		catalog.ActionIAMUpdateAssumeRolePolicy, "UpdateAssumeRolePolicy",
		catalog.ActionIAMCreateGroup, "CreateGroup",
		catalog.ActionIAMDeleteGroup, "DeleteGroup",
		catalog.ActionIAMGetGroup, "GetGroup",
		catalog.ActionIAMListGroups, "ListGroups",
		catalog.ActionIAMAddUserToGroup, "AddUserToGroup",
		catalog.ActionIAMRemoveUserFromGroup, "RemoveUserFromGroup",
		catalog.ActionIAMAttachGroupPolicy, "AttachGroupPolicy",
		catalog.ActionIAMDetachGroupPolicy, "DetachGroupPolicy",
		catalog.ActionIAMListAttachedGroupPolicies, "ListAttachedGroupPolicies",
		catalog.ActionIAMPutGroupPolicy, "PutGroupPolicy",
		catalog.ActionIAMGetGroupPolicy, "GetGroupPolicy",
		catalog.ActionIAMDeleteGroupPolicy, "DeleteGroupPolicy",
		catalog.ActionIAMListGroupPolicies, "ListGroupPolicies",
		catalog.ActionIAMPutUserPermissionsBoundary, "PutUserPermissionsBoundary",
		catalog.ActionIAMGetUserPermissionsBoundary, "GetUserPermissionsBoundary",
		catalog.ActionIAMDeleteUserPermissionsBoundary, "DeleteUserPermissionsBoundary",
		catalog.ActionIAMPutRolePermissionsBoundary, "PutRolePermissionsBoundary",
		catalog.ActionIAMGetRolePermissionsBoundary, "GetRolePermissionsBoundary",
		catalog.ActionIAMDeleteRolePermissionsBoundary, "DeleteRolePermissionsBoundary",
		catalog.ActionIAMCreateInstanceProfile, "CreateInstanceProfile",
		catalog.ActionIAMDeleteInstanceProfile, "DeleteInstanceProfile",
		catalog.ActionIAMGetInstanceProfile, "GetInstanceProfile",
		catalog.ActionIAMAddRoleToInstanceProfile, "AddRoleToInstanceProfile",
		catalog.ActionIAMRemoveRoleFromInstanceProfile, "RemoveRoleFromInstanceProfile",
		catalog.ActionIAMListInstanceProfiles, "ListInstanceProfiles",
		catalog.ActionIAMListInstanceProfilesForRole, "ListInstanceProfilesForRole",
		catalog.ActionIAMCreateOpenIDConnectProvider, "CreateOpenIDConnectProvider",
		catalog.ActionIAMDeleteOpenIDConnectProvider, "DeleteOpenIDConnectProvider",
		catalog.ActionIAMListOpenIDConnectProviders, "ListOpenIDConnectProviders",
		catalog.ActionIAMGetOpenIDConnectProvider, "GetOpenIDConnectProvider",
		catalog.ActionIAMCreateSAMLProvider, "CreateSAMLProvider",
		catalog.ActionIAMDeleteSAMLProvider, "DeleteSAMLProvider",
		catalog.ActionIAMListSAMLProviders, "ListSAMLProviders",
		catalog.ActionIAMGetSAMLProvider, "GetSAMLProvider",
		catalog.ActionIAMCreateVirtualMFADevice, "CreateVirtualMFADevice",
		catalog.ActionIAMEnableMFADevice, "EnableMFADevice",
		catalog.ActionIAMListMFADevices, "ListMFADevices",
		catalog.ActionIAMDeactivateMFADevice, "DeactivateMFADevice":
		s.handleIAM(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionKMSCreateKey, "CreateKey",
		catalog.ActionKMSDescribeKey, "DescribeKey",
		catalog.ActionKMSListKeys, "ListKeys",
		catalog.ActionKMSEnableKey, "EnableKey",
		catalog.ActionKMSDisableKey, "DisableKey",
		catalog.ActionKMSGetKeyPolicy, "GetKeyPolicy",
		catalog.ActionKMSPutKeyPolicy, "PutKeyPolicy",
		catalog.ActionKMSEncrypt, "Encrypt",
		catalog.ActionKMSDecrypt, "Decrypt",
		catalog.ActionKMSGenerateDataKey, "GenerateDataKey",
		catalog.ActionKMSGenerateDataKeyWithoutPlaintext, "GenerateDataKeyWithoutPlaintext",
		catalog.ActionKMSCreateGrant, "CreateGrant",
		catalog.ActionKMSListGrants, "ListGrants",
		catalog.ActionKMSRetireGrant, "RetireGrant",
		catalog.ActionKMSRevokeGrant, "RevokeGrant",
		catalog.ActionKMSCreateAlias, "CreateAlias",
		catalog.ActionKMSListAliases, "ListAliases",
		catalog.ActionKMSDeleteAlias, "DeleteAlias",
		catalog.ActionKMSUpdateAlias, "UpdateAlias",
		catalog.ActionKMSScheduleKeyDeletion, "ScheduleKeyDeletion",
		catalog.ActionKMSCancelKeyDeletion, "CancelKeyDeletion",
		catalog.ActionKMSEnableKeyRotation, "EnableKeyRotation",
		catalog.ActionKMSDisableKeyRotation, "DisableKeyRotation",
		catalog.ActionKMSGetKeyRotationStatus, "GetKeyRotationStatus",
		catalog.ActionKMSListResourceTags, "ListResourceTags",
		catalog.ActionKMSTagResource, "TagResource",
		catalog.ActionKMSUntagResource, "UntagResource",
		"ReEncrypt":
		s.handleKMS(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionDynamoDBCreateTable, "CreateTable",
		catalog.ActionDynamoDBDescribeTable, "DescribeTable",
		catalog.ActionDynamoDBDeleteTable, "DeleteTable",
		catalog.ActionDynamoDBListTables, "ListTables",
		catalog.ActionDynamoDBUpdateTable, "UpdateTable",
		catalog.ActionDynamoDBPutItem, "PutItem",
		catalog.ActionDynamoDBGetItem, "GetItem",
		catalog.ActionDynamoDBDeleteItem, "DeleteItem",
		catalog.ActionDynamoDBUpdateItem, "UpdateItem",
		catalog.ActionDynamoDBQuery, "Query",
		catalog.ActionDynamoDBScan, "Scan",
		catalog.ActionDynamoDBBatchGetItem, "BatchGetItem",
		catalog.ActionDynamoDBBatchWriteItem, "BatchWriteItem",
		catalog.ActionDynamoDBTransactGetItems, "TransactGetItems",
		catalog.ActionDynamoDBTransactWriteItems, "TransactWriteItems",
		catalog.ActionDynamoDBPutResourcePolicy, "PutResourcePolicy",
		catalog.ActionDynamoDBGetResourcePolicy, "GetResourcePolicy",
		catalog.ActionDynamoDBDeleteResourcePolicy, "DeleteResourcePolicy",
		catalog.ActionDynamoDBUpdateTimeToLive, "UpdateTimeToLive",
		catalog.ActionDynamoDBDescribeTimeToLive, "DescribeTimeToLive",
		catalog.ActionDynamoDBDescribeContinuousBackups, "DescribeContinuousBackups",
		catalog.ActionDynamoDBListTagsOfResource, "ListTagsOfResource",
		catalog.ActionDynamoDBTagResource,
		catalog.ActionDynamoDBUntagResource,
		catalog.ActionDynamoDBExecuteStatement, "ExecuteStatement",
		catalog.ActionDynamoDBBatchExecuteStatement, "BatchExecuteStatement":
		s.handleDynamoDB(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionSQSCreateQueue, "CreateQueue",
		catalog.ActionSQSGetQueueUrl, "GetQueueUrl",
		catalog.ActionSQSGetQueueAttributes, "GetQueueAttributes",
		catalog.ActionSQSSetQueueAttributes, "SetQueueAttributes",
		catalog.ActionSQSDeleteQueue, "DeleteQueue",
		catalog.ActionSQSListQueues, "ListQueues",
		catalog.ActionSQSPurgeQueue, "PurgeQueue",
		catalog.ActionSQSSendMessage, "SendMessage",
		catalog.ActionSQSReceiveMessage, "ReceiveMessage",
		catalog.ActionSQSDeleteMessage, "DeleteMessage",
		catalog.ActionSQSSendMessageBatch, "SendMessageBatch",
		catalog.ActionSQSDeleteMessageBatch, "DeleteMessageBatch",
		catalog.ActionSQSChangeMessageVisibility, "ChangeMessageVisibility",
		catalog.ActionSQSListQueueTags, "ListQueueTags",
		catalog.ActionSQSTagQueue, "TagQueue",
		catalog.ActionSQSUntagQueue, "UntagQueue":
		s.handleSQS(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionSSMPutParameter, "PutParameter",
		catalog.ActionSSMGetParameter, "GetParameter",
		catalog.ActionSSMGetParameters, "GetParameters",
		catalog.ActionSSMGetParametersByPath, "GetParametersByPath",
		catalog.ActionSSMDeleteParameter, "DeleteParameter",
		catalog.ActionSSMDescribeParameters, "DescribeParameters",
		catalog.ActionSSMListTagsForResource,
		catalog.ActionSSMAddTagsToResource, "AddTagsToResource",
		catalog.ActionSSMRemoveTagsFromResource, "RemoveTagsFromResource",
		catalog.ActionSSMSendCommand, "SendCommand",
		catalog.ActionSSMGetCommandInvocation, "GetCommandInvocation",
		catalog.ActionSSMListCommandInvocations, "ListCommandInvocations":
		s.handleSSM(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionSecretsCreateSecret, "CreateSecret",
		catalog.ActionSecretsGetSecretValue, "GetSecretValue",
		catalog.ActionSecretsPutSecretValue, "PutSecretValue",
		catalog.ActionSecretsDeleteSecret, "DeleteSecret",
		catalog.ActionSecretsRestoreSecret, "RestoreSecret",
		catalog.ActionSecretsRotateSecret, "RotateSecret",
		catalog.ActionSecretsUpdateSecretVersionStage, "UpdateSecretVersionStage",
		catalog.ActionSecretsDescribeSecret, "DescribeSecret",
		catalog.ActionSecretsListSecrets, "ListSecrets",
		catalog.ActionSecretsPutResourcePolicy,
		catalog.ActionSecretsGetResourcePolicy,
		catalog.ActionSecretsDeleteResourcePolicy,
		catalog.ActionSecretsListTagsForResource,
		catalog.ActionSecretsTagResource,
		catalog.ActionSecretsUntagResource:
		s.handleSecretsManager(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionECRCreateRepository, "CreateRepository",
		catalog.ActionECRDescribeRepositories, "DescribeRepositories",
		catalog.ActionECRDeleteRepository, "DeleteRepository",
		catalog.ActionECRGetAuthorizationToken, "GetAuthorizationToken",
		catalog.ActionECRGetRepositoryPolicy, "GetRepositoryPolicy",
		catalog.ActionECRSetRepositoryPolicy, "SetRepositoryPolicy",
		catalog.ActionECRDeleteRepositoryPolicy, "DeleteRepositoryPolicy",
		catalog.ActionECRPutImage, "PutImage",
		catalog.ActionECRBatchGetImage, "BatchGetImage",
		catalog.ActionECRListImages, "ListImages",
		catalog.ActionECRBatchDeleteImage, "BatchDeleteImage",
		catalog.ActionECRListTagsForResource,
		catalog.ActionECRTagResource,
		catalog.ActionECRUntagResource:
		s.handleECR(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionEventsPutEvents, "PutEvents",
		catalog.ActionEventsCreateEventBus, "CreateEventBus",
		catalog.ActionEventsDeleteEventBus, "DeleteEventBus",
		catalog.ActionEventsDescribeEventBus, "DescribeEventBus",
		catalog.ActionEventsListEventBuses, "ListEventBuses",
		catalog.ActionEventsPutRule, "PutRule",
		catalog.ActionEventsDescribeRule, "DescribeRule",
		catalog.ActionEventsListRules, "ListRules",
		catalog.ActionEventsDeleteRule, "DeleteRule",
		catalog.ActionEventsEnableRule, "EnableRule",
		catalog.ActionEventsDisableRule, "DisableRule",
		catalog.ActionEventsPutTargets, "PutTargets",
		catalog.ActionEventsRemoveTargets, "RemoveTargets",
		catalog.ActionEventsListTargetsByRule, "ListTargetsByRule",
		catalog.ActionEventsPutPermission, "PutPermission",
		catalog.ActionEventsRemovePermission,
		catalog.ActionEventsListTagsForResource,
		catalog.ActionEventsTagResource,
		catalog.ActionEventsUntagResource:
		s.handleEventBridge(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionLambdaCreateFunction, "CreateFunction",
		catalog.ActionLambdaGetFunction, "GetFunction",
		catalog.ActionLambdaDeleteFunction, "DeleteFunction",
		catalog.ActionLambdaListFunctions, "ListFunctions",
		catalog.ActionLambdaUpdateFunctionCode, "UpdateFunctionCode",
		catalog.ActionLambdaUpdateFunctionConfiguration, "UpdateFunctionConfiguration",
		catalog.ActionLambdaInvoke, "Invoke",
		catalog.ActionLambdaPublishVersion, "PublishVersion",
		catalog.ActionLambdaListVersionsByFunction, "ListVersionsByFunction",
		catalog.ActionLambdaCreateAlias,
		catalog.ActionLambdaUpdateAlias,
		catalog.ActionLambdaDeleteAlias,
		catalog.ActionLambdaGetAlias, "GetAlias",
		catalog.ActionLambdaListAliases,
		catalog.ActionLambdaPublishLayerVersion, "PublishLayerVersion",
		catalog.ActionLambdaGetLayerVersion, "GetLayerVersion",
		catalog.ActionLambdaListLayerVersions, "ListLayerVersions",
		catalog.ActionLambdaDeleteLayerVersion, "DeleteLayerVersion",
		catalog.ActionLambdaAddPermission, "AddPermission",
		catalog.ActionLambdaRemovePermission, "RemovePermission",
		catalog.ActionLambdaGetPolicy,
		catalog.ActionLambdaCreateEventSourceMapping, "CreateEventSourceMapping",
		catalog.ActionLambdaGetEventSourceMapping, "GetEventSourceMapping",
		catalog.ActionLambdaListEventSourceMappings, "ListEventSourceMappings",
		catalog.ActionLambdaUpdateEventSourceMapping, "UpdateEventSourceMapping",
		catalog.ActionLambdaDeleteEventSourceMapping, "DeleteEventSourceMapping",
		catalog.ActionLambdaCreateFunctionUrlConfig, "CreateFunctionUrlConfig",
		catalog.ActionLambdaGetFunctionUrlConfig, "GetFunctionUrlConfig",
		catalog.ActionLambdaDeleteFunctionUrlConfig, "DeleteFunctionUrlConfig",
		catalog.ActionLambdaListFunctionUrlConfigs, "ListFunctionUrlConfigs",
		catalog.ActionLambdaListTags, "ListTags",
		catalog.ActionLambdaGetFunctionCodeSigningConfig, "GetFunctionCodeSigningConfig",
		catalog.ActionLambdaPutFunctionEventInvokeConfig, "PutFunctionEventInvokeConfig",
		catalog.ActionLambdaGetFunctionEventInvokeConfig, "GetFunctionEventInvokeConfig",
		catalog.ActionLambdaDeleteFunctionEventInvokeConfig, "DeleteFunctionEventInvokeConfig":
		s.handleLambda(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionECSRegisterTaskDefinition, "RegisterTaskDefinition",
		catalog.ActionECSDescribeTaskDefinition, "DescribeTaskDefinition",
		catalog.ActionECSListTaskDefinitions, "ListTaskDefinitions",
		catalog.ActionECSDeregisterTaskDefinition, "DeregisterTaskDefinition",
		catalog.ActionECSRunTask, "RunTask",
		catalog.ActionECSDescribeTasks, "DescribeTasks",
		catalog.ActionECSListTasks, "ListTasks",
		catalog.ActionECSStopTask, "StopTask",
		catalog.ActionECSDescribeClusters, "DescribeClusters",
		catalog.ActionECSListClusters, "ListClusters",
		catalog.ActionECSCreateService, "CreateService",
		catalog.ActionECSUpdateService, "UpdateService",
		catalog.ActionECSDeleteService, "DeleteService",
		catalog.ActionECSDescribeServices, "DescribeServices",
		catalog.ActionECSListServices, "ListServices",
		catalog.ActionECSListTagsForResource,
		catalog.ActionECSTagResource,
		catalog.ActionECSUntagResource:
		s.handleECS(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCloudTrailLookupEvents, "LookupEvents",
		catalog.ActionCloudTrailCreateTrail, "CreateTrail",
		catalog.ActionCloudTrailDescribeTrails, "DescribeTrails",
		catalog.ActionCloudTrailDeleteTrail, "DeleteTrail",
		catalog.ActionCloudTrailStartLogging, "StartLogging",
		catalog.ActionCloudTrailStopLogging, "StopLogging",
		catalog.ActionCloudTrailInjectEvents, "InjectEvents",
		catalog.ActionCloudTrailInjectInsightsEvents, "InjectInsightsEvents",
		catalog.ActionCloudTrailPutEventSelectors, "PutEventSelectors",
		catalog.ActionCloudTrailGetEventSelectors, "GetEventSelectors",
		catalog.ActionCloudTrailValidateLogs, "ValidateLogs":
		s.handleCloudTrail(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionLogsCreateLogGroup, "CreateLogGroup",
		catalog.ActionLogsCreateLogStream, "CreateLogStream",
		catalog.ActionLogsDeleteLogGroup, "DeleteLogGroup",
		catalog.ActionLogsDeleteLogStream, "DeleteLogStream",
		catalog.ActionLogsDescribeLogStreams, "DescribeLogStreams",
		catalog.ActionLogsPutLogEvents, "PutLogEvents",
		catalog.ActionLogsGetLogEvents, "GetLogEvents",
		catalog.ActionLogsFilterLogEvents, "FilterLogEvents",
		catalog.ActionLogsDescribeLogGroups, "DescribeLogGroups",
		catalog.ActionLogsPutSubscriptionFilter, "PutSubscriptionFilter",
		catalog.ActionLogsDeleteSubscriptionFilter, "DeleteSubscriptionFilter",
		catalog.ActionLogsDescribeSubscriptionFilters, "DescribeSubscriptionFilters",
		catalog.ActionLogsPutMetricFilter, "PutMetricFilter",
		catalog.ActionLogsDeleteMetricFilter, "DeleteMetricFilter",
		catalog.ActionLogsDescribeMetricFilters, "DescribeMetricFilters",
		catalog.ActionLogsPutResourcePolicy,
		catalog.ActionLogsGetResourcePolicy,
		catalog.ActionLogsDeleteResourcePolicy,
		catalog.ActionLogsDescribeResourcePolicies, "DescribeResourcePolicies",
		catalog.ActionLogsPutRetentionPolicy, "PutRetentionPolicy",
		catalog.ActionLogsDeleteRetentionPolicy, "DeleteRetentionPolicy":
		s.handleLogs(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionTaggingTagResources, "TagResources",
		catalog.ActionTaggingUntagResources, "UntagResources",
		catalog.ActionTaggingGetResources, "GetResources":
		s.handleTagging(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionKinesisCreateStream, "CreateStream",
		catalog.ActionKinesisDeleteStream, "DeleteStream",
		catalog.ActionKinesisDescribeStream, "DescribeStream",
		catalog.ActionKinesisListStreams, "ListStreams",
		catalog.ActionKinesisPutRecord, "PutRecord",
		catalog.ActionKinesisPutRecords, "PutRecords",
		catalog.ActionKinesisGetShardIterator, "GetShardIterator",
		catalog.ActionKinesisGetRecords, "GetRecords",
		catalog.ActionKinesisPutResourcePolicy,
		catalog.ActionKinesisGetResourcePolicy,
		catalog.ActionKinesisDeleteResourcePolicy,
		catalog.ActionKinesisRegisterStreamConsumer, "RegisterStreamConsumer",
		catalog.ActionKinesisDescribeStreamConsumer, "DescribeStreamConsumer",
		catalog.ActionKinesisListStreamConsumers, "ListStreamConsumers",
		catalog.ActionKinesisDeregisterStreamConsumer, "DeregisterStreamConsumer",
		catalog.ActionKinesisSubscribeToShard, "SubscribeToShard",
		catalog.ActionKinesisUpdateShardCount, "UpdateShardCount":
		s.handleKinesis(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionAppConfigCreateApplication,
		catalog.ActionAppConfigCreateEnvironment,
		catalog.ActionAppConfigCreateConfigurationProfile, "CreateConfigurationProfile",
		catalog.ActionAppConfigCreateHostedConfigurationVersion, "CreateHostedConfigurationVersion",
		catalog.ActionAppConfigGetConfiguration, "GetConfiguration",
		catalog.ActionAppConfigDataStartConfigurationSession, "StartConfigurationSession",
		catalog.ActionAppConfigDataGetLatestConfiguration, "GetLatestConfiguration":
		s.handleAppConfig(w, r, body, requestID, eventID, action, verified, readOnly)
	case "CreateApplication", "CreateEnvironment":
		// Ambiguous short Action names: prefer SigV4 credential scope.
		switch strings.ToLower(verified.Service) {
		case "elasticbeanstalk":
			s.handleElasticBeanstalk(w, r, body, requestID, eventID, action, verified, readOnly)
		case "codedeploy":
			s.handleCodeDeploy(w, r, body, requestID, eventID, action, verified, readOnly)
		default:
			s.handleAppConfig(w, r, body, requestID, eventID, action, verified, readOnly)
		}
	case catalog.ActionSFNCreateStateMachine, "CreateStateMachine",
		catalog.ActionSFNDeleteStateMachine, "DeleteStateMachine",
		catalog.ActionSFNDescribeStateMachine, "DescribeStateMachine",
		catalog.ActionSFNListStateMachines, "ListStateMachines",
		catalog.ActionSFNStartExecution, "StartExecution",
		catalog.ActionSFNDescribeExecution, "DescribeExecution",
		catalog.ActionSFNGetExecutionHistory, "GetExecutionHistory",
		catalog.ActionSFNPutResourcePolicy,
		catalog.ActionSFNGetResourcePolicy,
		catalog.ActionSFNDeleteResourcePolicy:
		s.handleSFN(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCodeBuildCreateProject, "CreateProject",
		catalog.ActionCodeBuildUpdateProject, "UpdateProject",
		catalog.ActionCodeBuildDeleteProject, "DeleteProject",
		catalog.ActionCodeBuildListProjects, "ListProjects",
		catalog.ActionCodeBuildBatchGetProjects, "BatchGetProjects",
		catalog.ActionCodeBuildStartBuild, "StartBuild",
		catalog.ActionCodeBuildStartBuildBatch, "StartBuildBatch",
		catalog.ActionCodeBuildStopBuild, "StopBuild",
		catalog.ActionCodeBuildBatchGetBuilds, "BatchGetBuilds",
		catalog.ActionCodeBuildListBuilds, "ListBuilds",
		catalog.ActionCodeBuildCreateWebhook, "CreateWebhook",
		catalog.ActionCodeBuildDeleteWebhook, "DeleteWebhook",
		catalog.ActionCodeBuildListWebhooks, "ListWebhooks":
		s.handleCodeBuild(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCodeCommitCreateRepository,
		catalog.ActionCodeCommitGetRepository, "GetRepository",
		catalog.ActionCodeCommitListRepositories, "ListRepositories",
		catalog.ActionCodeCommitDeleteRepository,
		catalog.ActionCodeCommitPutFile, "PutFile",
		catalog.ActionCodeCommitGetFile, "GetFile",
		catalog.ActionCodeCommitGetFolder, "GetFolder":
		s.handleCodeCommit(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionBatchCreateComputeEnvironment, "CreateComputeEnvironment",
		catalog.ActionBatchCreateJobQueue, "CreateJobQueue",
		catalog.ActionBatchRegisterJobDefinition, "RegisterJobDefinition",
		catalog.ActionBatchSubmitJob, "SubmitJob",
		catalog.ActionBatchDescribeComputeEnvironments, "DescribeComputeEnvironments",
		catalog.ActionBatchDescribeJobQueues, "DescribeJobQueues",
		catalog.ActionBatchDescribeJobDefinitions, "DescribeJobDefinitions",
		catalog.ActionBatchDescribeJobs, "DescribeJobs":
		s.handleBatch(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCFNCreateStack, "CreateStack",
		catalog.ActionCFNDescribeStacks, "DescribeStacks",
		catalog.ActionCFNDeleteStack, "DeleteStack",
		catalog.ActionCFNListStacks, "ListStacks",
		catalog.ActionCFNUpdateStack, "UpdateStack",
		catalog.ActionCFNCreateChangeSet, "CreateChangeSet",
		catalog.ActionCFNDescribeChangeSet, "DescribeChangeSet",
		catalog.ActionCFNExecuteChangeSet, "ExecuteChangeSet",
		catalog.ActionCFNDetectStackDrift, "DetectStackDrift",
		catalog.ActionCFNDescribeStackDriftDetectionStatus, "DescribeStackDriftDetectionStatus",
		catalog.ActionCFNDescribeStackResourceDrifts, "DescribeStackResourceDrifts":
		s.handleCloudFormation(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCodePipelineCreatePipeline, "CreatePipeline",
		catalog.ActionCodePipelineGetPipeline, "GetPipeline",
		catalog.ActionCodePipelineDeletePipeline, "DeletePipeline",
		catalog.ActionCodePipelineStartPipelineExecution, "StartPipelineExecution",
		catalog.ActionCodePipelineGetPipelineState, "GetPipelineState":
		s.handleCodePipeline(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionFirehoseCreateDeliveryStream, "CreateDeliveryStream",
		catalog.ActionFirehoseDeleteDeliveryStream, "DeleteDeliveryStream",
		catalog.ActionFirehoseDescribeDeliveryStream, "DescribeDeliveryStream",
		catalog.ActionFirehoseListDeliveryStreams, "ListDeliveryStreams",
		catalog.ActionFirehosePutRecord,
		catalog.ActionFirehosePutRecordBatch, "PutRecordBatch":
		s.handleFirehose(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionGlueCreateDatabase, "CreateDatabase",
		catalog.ActionGlueGetDatabase, "GetDatabase",
		catalog.ActionGlueGetDatabases, "GetDatabases",
		catalog.ActionGlueDeleteDatabase, "DeleteDatabase",
		catalog.ActionGlueCreateTable,
		catalog.ActionGlueGetTable, "GetTable",
		catalog.ActionGlueGetTables, "GetTables",
		catalog.ActionGlueDeleteTable:
		s.handleGlue(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionWAFCreateWebACL, "CreateWebACL",
		catalog.ActionWAFUpdateWebACL, "UpdateWebACL",
		catalog.ActionWAFGetWebACL, "GetWebACL",
		catalog.ActionWAFListWebACLs, "ListWebACLs",
		catalog.ActionWAFCreateRuleGroup, "CreateRuleGroup",
		catalog.ActionWAFAssociateWebACL, "AssociateWebACL",
		catalog.ActionWAFEvaluate, "Evaluate":
		s.handleWAFv2(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionConfigPutConfigurationRecorder, "PutConfigurationRecorder",
		catalog.ActionConfigPutDeliveryChannel, "PutDeliveryChannel",
		catalog.ActionConfigStartConfigurationRecorder, "StartConfigurationRecorder",
		catalog.ActionConfigDescribeComplianceByConfigRule, "DescribeComplianceByConfigRule",
		catalog.ActionConfigGetResourceConfigHistory, "GetResourceConfigHistory":
		s.handleConfig(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionDynamoDBStreamsListStreams,
		catalog.ActionDynamoDBStreamsDescribeStream,
		catalog.ActionDynamoDBStreamsGetShardIterator,
		catalog.ActionDynamoDBStreamsGetRecords:
		s.handleDynamoDBStreams(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionPipesCreatePipe, "CreatePipe",
		catalog.ActionPipesDescribePipe, "DescribePipe",
		catalog.ActionPipesDeletePipe, "DeletePipe",
		catalog.ActionPipesListPipes, "ListPipes":
		s.handlePipes(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionMQCreateBroker, "CreateBroker",
		catalog.ActionMQDescribeBroker, "DescribeBroker",
		catalog.ActionMQListBrokers, "ListBrokers",
		catalog.ActionMQDeleteBroker, "DeleteBroker":
		s.handleMQ(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionElastiCacheCreateCacheCluster, "CreateCacheCluster",
		catalog.ActionElastiCacheDescribeCacheClusters, "DescribeCacheClusters",
		catalog.ActionElastiCacheDeleteCacheCluster, "DeleteCacheCluster":
		s.handleElastiCache(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionMemoryDBCreateCluster,
		catalog.ActionMemoryDBDescribeClusters,
		catalog.ActionMemoryDBDeleteCluster,
		catalog.ActionMemoryDBDescribeUsers,
		catalog.ActionMemoryDBDescribeACLs:
		s.handleMemoryDB(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionDocDBCreateDBCluster,
		catalog.ActionDocDBDescribeDBClusters,
		catalog.ActionDocDBDeleteDBCluster:
		s.handleDocDB(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionMSKCreateCluster,
		catalog.ActionMSKDescribeCluster,
		catalog.ActionMSKListClusters,
		catalog.ActionMSKDeleteCluster,
		catalog.ActionMSKGetBootstrapBrokers:
		s.handleMSK(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionNeptuneCreateDBCluster,
		catalog.ActionNeptuneDescribeDBClusters,
		catalog.ActionNeptuneDeleteDBCluster:
		s.handleNeptune(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionRDSCreateDBInstance, "CreateDBInstance",
		catalog.ActionRDSDescribeDBInstances, "DescribeDBInstances",
		catalog.ActionRDSDeleteDBInstance, "DeleteDBInstance":
		s.handleRDS(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionRDSDataExecuteStatement,
		catalog.ActionRDSDataBatchExecuteStatement,
		catalog.ActionRDSDataBeginTransaction,
		catalog.ActionRDSDataCommitTransaction,
		catalog.ActionRDSDataRollbackTransaction:
		s.handleRDSData(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionAthenaStartQueryExecution,
		catalog.ActionAthenaGetQueryExecution,
		catalog.ActionAthenaGetQueryResults,
		catalog.ActionAthenaStopQueryExecution:
		s.handleAthena(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionOpenSearchCreateDomain,
		catalog.ActionOpenSearchDescribeDomain,
		catalog.ActionOpenSearchListDomainNames,
		catalog.ActionOpenSearchDeleteDomain:
		s.handleOpenSearch(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionTransferCreateServer, "CreateServer",
		catalog.ActionTransferDescribeServer, "DescribeServer",
		catalog.ActionTransferListServers, "ListServers",
		catalog.ActionTransferDeleteServer, "DeleteServer",
		catalog.ActionTransferCreateUser,
		catalog.ActionTransferDeleteUser:
		s.handleTransfer(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionACMRequestCertificate, "RequestCertificate",
		catalog.ActionACMDescribeCertificate, "DescribeCertificate",
		catalog.ActionACMListCertificates, "ListCertificates",
		catalog.ActionACMDeleteCertificate, "DeleteCertificate":
		s.handleACM(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionGuardDutyCreateDetector, "CreateDetector",
		catalog.ActionGuardDutyListDetectors, "ListDetectors",
		catalog.ActionGuardDutyListFindings,
		catalog.ActionGuardDutyGetFindings,
		catalog.ActionGuardDutyInjectFindings, "InjectFindings":
		// ListFindings/GetFindings/InjectFindings also used by Security Hub and Macie;
		// prefer SigV4 service routing above.
		if strings.EqualFold(verified.Service, "securityhub") || strings.HasPrefix(action, "securityhub:") {
			s.handleSecurityHub(w, r, body, requestID, eventID, action, verified, readOnly)
			break
		}
		if strings.EqualFold(verified.Service, "macie2") || strings.EqualFold(verified.Service, "macie") ||
			strings.HasPrefix(action, "macie2:") {
			s.handleMacie(w, r, body, requestID, eventID, action, verified, readOnly)
			break
		}
		s.handleGuardDuty(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionMacieEnableMacie, "EnableMacie",
		catalog.ActionMacieGetMacieSession, "GetMacieSession",
		catalog.ActionMacieCreateClassificationJob, "CreateClassificationJob",
		catalog.ActionMacieDescribeClassificationJob, "DescribeClassificationJob",
		catalog.ActionMacieListClassificationJobs, "ListClassificationJobs",
		catalog.ActionMacieListFindings,
		catalog.ActionMacieGetFindings,
		catalog.ActionMacieInjectFindings:
		s.handleMacie(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionEC2RunInstances, "RunInstances",
		catalog.ActionEC2DescribeInstances, "DescribeInstances",
		catalog.ActionEC2DescribeImages, "DescribeImages",
		catalog.ActionEC2TerminateInstances, "TerminateInstances",
		catalog.ActionEC2StopInstances, "StopInstances",
		catalog.ActionEC2StartInstances, "StartInstances",
		catalog.ActionEC2CreateFlowLogs, "CreateFlowLogs",
		catalog.ActionEC2InjectFlowLogs, "InjectFlowLogs":
		s.handleEC2(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionSecurityHubBatchImportFindings, "BatchImportFindings",
		catalog.ActionSecurityHubGetFindings:
		s.handleSecurityHub(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionRoute53CreateHostedZone, "CreateHostedZone",
		catalog.ActionRoute53DeleteHostedZone, "DeleteHostedZone",
		catalog.ActionRoute53ListHostedZones, "ListHostedZones",
		catalog.ActionRoute53ChangeResourceRecordSets, "ChangeResourceRecordSets",
		catalog.ActionRoute53ListResourceRecordSets, "ListResourceRecordSets",
		catalog.ActionRoute53InjectQueryLogs, "InjectQueryLogs":
		s.handleRoute53(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionSDCreatePrivateDnsNamespace, "CreatePrivateDnsNamespace",
		catalog.ActionSDCreateHttpNamespace, "CreateHttpNamespace",
		catalog.ActionSDCreateService,
		catalog.ActionSDRegisterInstance, "RegisterInstance",
		catalog.ActionSDDeregisterInstance, "DeregisterInstance",
		catalog.ActionSDDiscoverInstances, "DiscoverInstances":
		s.handleServiceDiscovery(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionPricingDescribeServices,
		catalog.ActionPricingGetAttributeValues, "GetAttributeValues",
		catalog.ActionPricingGetProducts, "GetProducts":
		s.handlePricing(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionAppSyncCreateGraphqlApi, "CreateGraphqlApi",
		catalog.ActionAppSyncDeleteGraphqlApi, "DeleteGraphqlApi",
		catalog.ActionAppSyncGetGraphqlApi, "GetGraphqlApi",
		catalog.ActionAppSyncListGraphqlApis, "ListGraphqlApis",
		catalog.ActionAppSyncStartSchemaCreation, "StartSchemaCreation",
		catalog.ActionAppSyncCreateApiKey, "CreateApiKey",
		catalog.ActionAppSyncCreateDataSource, "CreateDataSource",
		catalog.ActionAppSyncCreateResolver, "CreateResolver":
		s.handleAppSync(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCognitoCreateUserPool, "CreateUserPool",
		catalog.ActionCognitoDescribeUserPool, "DescribeUserPool",
		catalog.ActionCognitoUpdateUserPool, "UpdateUserPool",
		catalog.ActionCognitoListUserPools, "ListUserPools",
		catalog.ActionCognitoDeleteUserPool, "DeleteUserPool",
		catalog.ActionCognitoCreateUserPoolClient, "CreateUserPoolClient",
		catalog.ActionCognitoDescribeUserPoolClient, "DescribeUserPoolClient",
		catalog.ActionCognitoListUserPoolClients, "ListUserPoolClients",
		catalog.ActionCognitoDeleteUserPoolClient, "DeleteUserPoolClient",
		catalog.ActionCognitoAdminCreateUser, "AdminCreateUser",
		catalog.ActionCognitoSignUp, "SignUp",
		catalog.ActionCognitoConfirmSignUp, "ConfirmSignUp",
		catalog.ActionCognitoForgotPassword, "ForgotPassword",
		catalog.ActionCognitoConfirmForgotPassword, "ConfirmForgotPassword",
		catalog.ActionCognitoResendConfirmationCode, "ResendConfirmationCode",
		catalog.ActionCognitoUpdateUserAttributes, "UpdateUserAttributes",
		catalog.ActionCognitoGetUserAttributeVerificationCode, "GetUserAttributeVerificationCode",
		catalog.ActionCognitoVerifyUserAttribute, "VerifyUserAttribute",
		catalog.ActionCognitoInitiateAuth, "InitiateAuth",
		catalog.ActionCognitoAdminInitiateAuth, "AdminInitiateAuth",
		catalog.ActionCognitoRevokeToken, "RevokeToken",
		catalog.ActionCognitoAssociateSoftwareToken, "AssociateSoftwareToken",
		catalog.ActionCognitoVerifySoftwareToken, "VerifySoftwareToken",
		catalog.ActionCognitoRespondToAuthChallenge, "RespondToAuthChallenge":
		s.handleCognito(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCloudControlCreateResource,
		catalog.ActionCloudControlGetResource,
		catalog.ActionCloudControlListResources,
		catalog.ActionCloudControlDeleteResource,
		catalog.ActionCloudControlUpdateResource,
		catalog.ActionCloudControlGetResourceRequestStatus:
		s.handleCloudControl(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionBCMCreateExport,
		catalog.ActionBCMGetExport,
		catalog.ActionBCMListExports,
		catalog.ActionBCMDeleteExport:
		s.handleBCMExports(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCEGetCostAndUsage,
		catalog.ActionCEGetCostForecast:
		s.handleCostExplorer(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionBudgetsCreateBudget,
		catalog.ActionBudgetsDescribeBudget,
		catalog.ActionBudgetsDescribeBudgets,
		catalog.ActionBudgetsDeleteBudget:
		s.handleBudgets(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCURPutReportDefinition,
		catalog.ActionCURModifyReportDefinition,
		catalog.ActionCURDescribeReportDefinitions,
		catalog.ActionCURDeleteReportDefinition,
		catalog.ActionCURTagResource,
		catalog.ActionCURUntagResource,
		catalog.ActionCURListTagsForResource:
		s.handleCUR(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionIoTCreateThing,
		catalog.ActionIoTDescribeThing,
		catalog.ActionIoTListThings,
		catalog.ActionIoTUpdateThing,
		catalog.ActionIoTDeleteThing,
		catalog.ActionIoTCreateKeysAndCertificate,
		catalog.ActionIoTDescribeCertificate,
		catalog.ActionIoTListCertificates,
		catalog.ActionIoTUpdateCertificate,
		catalog.ActionIoTDeleteCertificate,
		catalog.ActionIoTCreatePolicy,
		catalog.ActionIoTGetPolicy,
		catalog.ActionIoTListPolicies,
		catalog.ActionIoTDeletePolicy,
		catalog.ActionIoTAttachPolicy,
		catalog.ActionIoTDetachPolicy,
		catalog.ActionIoTAttachThingPrincipal,
		catalog.ActionIoTListThingPrincipals,
		catalog.ActionIoTDataUpdateThingShadow,
		catalog.ActionIoTDataGetThingShadow,
		catalog.ActionIoTDataDeleteThingShadow:
		s.handleIoT(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCloudWatchPutMetricData,
		catalog.ActionCloudWatchListMetrics,
		catalog.ActionCloudWatchGetMetricStatistics,
		catalog.ActionCloudWatchGetMetricData,
		catalog.ActionCloudWatchPutMetricAlarm,
		catalog.ActionCloudWatchDescribeAlarms,
		catalog.ActionCloudWatchDeleteAlarms,
		catalog.ActionCloudWatchSetAlarmState,
		"PutMetricData", "ListMetrics", "GetMetricStatistics", "GetMetricData",
		"PutMetricAlarm", "DescribeAlarms", "DeleteAlarms", "SetAlarmState":
		s.handleCloudWatch(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionLightsailGetBlueprints,
		catalog.ActionLightsailGetBundles,
		catalog.ActionLightsailCreateInstances,
		catalog.ActionLightsailGetInstance,
		catalog.ActionLightsailGetInstances,
		catalog.ActionLightsailStartInstance,
		catalog.ActionLightsailStopInstance,
		catalog.ActionLightsailRebootInstance,
		catalog.ActionLightsailDeleteInstance:
		s.handleLightsail(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionASGCreateLaunchConfiguration,
		catalog.ActionASGDescribeLaunchConfigurations,
		catalog.ActionASGDeleteLaunchConfiguration,
		catalog.ActionASGCreateAutoScalingGroup,
		catalog.ActionASGDescribeAutoScalingGroups,
		catalog.ActionASGUpdateAutoScalingGroup,
		catalog.ActionASGDeleteAutoScalingGroup,
		catalog.ActionASGSetDesiredCapacity:
		s.handleAutoScaling(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionBeanstalkCreateApplication,
		catalog.ActionBeanstalkDescribeApplications,
		catalog.ActionBeanstalkDeleteApplication,
		catalog.ActionBeanstalkCreateApplicationVersion,
		catalog.ActionBeanstalkCreateEnvironment,
		catalog.ActionBeanstalkDescribeEnvironments,
		catalog.ActionBeanstalkTerminateEnvironment,
		catalog.ActionBeanstalkListAvailableSolutionStacks:
		s.handleElasticBeanstalk(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionBackupCreateBackupVault,
		catalog.ActionBackupDescribeBackupVault,
		catalog.ActionBackupListBackupVaults,
		catalog.ActionBackupDeleteBackupVault,
		catalog.ActionBackupCreateBackupPlan,
		catalog.ActionBackupGetBackupPlan,
		catalog.ActionBackupListBackupPlans,
		catalog.ActionBackupDeleteBackupPlan,
		catalog.ActionBackupStartBackupJob,
		catalog.ActionBackupDescribeBackupJob,
		catalog.ActionBackupDescribeRecoveryPoint,
		catalog.ActionBackupListRecoveryPointsByBackupVault:
		s.handleBackup(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCodeDeployCreateApplication,
		catalog.ActionCodeDeployCreateDeploymentGroup,
		catalog.ActionCodeDeployCreateDeployment,
		catalog.ActionCodeDeployGetDeployment,
		catalog.ActionCodeDeployListDeployments:
		s.handleCodeDeploy(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCloudFrontCreateDistribution,
		catalog.ActionCloudFrontGetDistribution,
		catalog.ActionCloudFrontListDistributions,
		catalog.ActionCloudFrontDeleteDistribution:
		s.handleCloudFront(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionELBv2CreateLoadBalancer,
		catalog.ActionELBv2DescribeLoadBalancers,
		catalog.ActionELBv2DeleteLoadBalancer,
		catalog.ActionELBv2CreateTargetGroup,
		catalog.ActionELBv2DescribeTargetGroups,
		catalog.ActionELBv2DeleteTargetGroup,
		catalog.ActionELBv2CreateListener,
		catalog.ActionELBv2DescribeListeners,
		catalog.ActionELBv2DeleteListener,
		catalog.ActionELBv2RegisterTargets,
		catalog.ActionELBv2DescribeTargetHealth,
		catalog.ActionELBv2CreateRule,
		catalog.ActionELBv2DescribeRules,
		catalog.ActionELBv2DeleteRule:
		s.handleELBv2(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionS3VectorsCreateVectorBucket,
		catalog.ActionS3VectorsListVectorBuckets,
		catalog.ActionS3VectorsDeleteVectorBucket,
		catalog.ActionS3VectorsCreateIndex,
		catalog.ActionS3VectorsListIndexes,
		catalog.ActionS3VectorsDeleteIndex,
		catalog.ActionS3VectorsPutVectors,
		catalog.ActionS3VectorsQueryVectors:
		s.handleS3Vectors(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionBedrockInvokeModel, "InvokeModel":
		s.handleBedrockRuntime(w, r, body, requestID, eventID, action, verified, readOnly, "")
	case catalog.ActionBedrockConverse, "Converse":
		s.handleBedrockRuntime(w, r, body, requestID, eventID, action, verified, readOnly, "")
	case catalog.ActionTextractDetectDocumentText, "DetectDocumentText",
		catalog.ActionTextractAnalyzeDocument, "AnalyzeDocument":
		s.handleTextract(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionTranscribeStartTranscriptionJob, "StartTranscriptionJob",
		catalog.ActionTranscribeGetTranscriptionJob, "GetTranscriptionJob",
		catalog.ActionTranscribeListTranscriptionJobs, "ListTranscriptionJobs":
		s.handleTranscribe(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionEMRRunJobFlow, "RunJobFlow",
		catalog.ActionEMRDescribeCluster, "DescribeCluster",
		catalog.ActionEMRListClusters,
		catalog.ActionEMRTerminateJobFlows, "TerminateJobFlows",
		catalog.ActionEMRAddJobFlowSteps, "AddJobFlowSteps",
		catalog.ActionEMRDescribeStep, "DescribeStep",
		catalog.ActionEMRListSteps, "ListSteps":
		s.handleEMR(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionLabFreezeClock, "FreezeClock",
		catalog.ActionLabUnfreezeClock, "UnfreezeClock",
		catalog.ActionLabSetClock, "SetClock",
		catalog.ActionLabBulkSeed, "BulkSeed":
		s.handleLabForensics(w, r, body, requestID, eventID, action, verified, readOnly)
	default:
		s.writeAWSError(w, requestID, http.StatusNotImplemented, "NotImplemented",
			"This API action is not implemented in Noctaxris.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
	}
}

func bodyLimitForRequest(r *http.Request) int64 {
	if isLikelyNonS3Protocol(r) {
		return maxBodyBytes
	}
	return maxS3BodyBytes
}

func isLikelyNonS3Protocol(r *http.Request) bool {
	if r.Header.Get("X-Amz-Target") != "" {
		return true
	}
	if r.URL.Query().Get("Action") != "" {
		return true
	}
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/x-amz-json") {
		return true
	}
	if strings.Contains(ct, "application/x-www-form-urlencoded") {
		return true
	}
	return false
}

// isS3PathStyleRequest detects path-style S3 when Action/X-Amz-Target are absent.
func isS3PathStyleRequest(r *http.Request, body []byte, action string) bool {
	if action != "" {
		return false
	}
	if r.Header.Get("X-Amz-Target") != "" {
		return false
	}
	if r.URL.Query().Get("Action") != "" {
		return false
	}
	if len(body) > 0 {
		vals, err := url.ParseQuery(string(body))
		if err == nil && vals.Get("Action") != "" {
			return false
		}
	}
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/x-amz-json") {
		return false
	}
	if strings.Contains(ct, "application/json") {
		return false
	}
	if isLambdaRESTPath(r.URL.Path) {
		return false
	}
	if isBedrockRuntimePath(r.URL.Path) {
		return false
	}
	if isMSKRESTPath(r.URL.Path) {
		return false
	}
	if isEKSRESTPath(r.URL.Path) {
		return false
	}
	return true
}

func isLambdaRESTPath(path string) bool {
	return strings.HasPrefix(path, "/2015-03-31/") ||
		strings.HasPrefix(path, "/2017-03-31/") ||
		strings.HasPrefix(path, "/2018-10-31/") ||
		strings.HasPrefix(path, "/2020-06-30/") ||
		strings.HasPrefix(path, "/2021-10-31/")
}

func isLambdaRESTAPIVersion(v string) bool {
	return v == "2015-03-31" || v == "2017-03-31" || v == "2018-10-31" || v == "2020-06-30" || v == "2021-10-31"
}

// resolveLambdaREST maps AWS Lambda REST paths to catalog actions and injects
// FunctionName / LayerName from the URL into the JSON body when missing (CLI REST shape).
func resolveLambdaREST(r *http.Request, body []byte) (action string, outBody []byte) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || !isLambdaRESTAPIVersion(parts[0]) {
		return "", body
	}
	switch parts[1] {
	case "functions":
		return resolveLambdaFunctionsREST(r.Method, parts, body)
	case "layers":
		return resolveLambdaLayersREST(r.Method, parts, body)
	case "tags":
		return resolveLambdaTagsREST(r.Method, parts, body)
	default:
		return "", body
	}
}

func resolveLambdaTagsREST(method string, parts []string, body []byte) (string, []byte) {
	// GET /2017-03-31/tags/{Resource}
	if method != http.MethodGet || len(parts) != 3 {
		return "", body
	}
	resource, err := url.PathUnescape(parts[2])
	if err != nil {
		resource = parts[2]
	}
	return catalog.ActionLambdaListTags, injectJSONStringField(body, "Resource", resource)
}

func resolveLambdaFunctionsREST(method string, parts []string, body []byte) (string, []byte) {
	name := ""
	if len(parts) >= 3 {
		name = parts[2]
	}
	switch method {
	case http.MethodPost:
		if len(parts) == 2 {
			return catalog.ActionLambdaCreateFunction, body
		}
		if len(parts) == 4 && parts[3] == "invocations" {
			return catalog.ActionLambdaInvoke, invokeRESTBody(body, name)
		}
		// POST /2015-03-31/functions/{FunctionName}/policy → AddPermission
		if len(parts) == 4 && parts[3] == "policy" {
			return catalog.ActionLambdaAddPermission, injectFunctionNameJSON(body, name)
		}
		// POST /2021-10-31/functions/{FunctionName}/url → CreateFunctionUrlConfig
		if len(parts) == 4 && parts[3] == "url" {
			return catalog.ActionLambdaCreateFunctionUrlConfig, injectFunctionNameJSON(body, name)
		}
	case http.MethodGet:
		if len(parts) == 2 {
			return catalog.ActionLambdaListFunctions, body
		}
		if len(parts) == 3 {
			return catalog.ActionLambdaGetFunction, injectFunctionNameJSON(body, name)
		}
		if len(parts) == 4 && parts[3] == "versions" {
			return catalog.ActionLambdaListVersionsByFunction, injectFunctionNameJSON(body, name)
		}
		if len(parts) == 4 && parts[3] == "code-signing-config" {
			return catalog.ActionLambdaGetFunctionCodeSigningConfig, injectFunctionNameJSON(body, name)
		}
		if len(parts) == 4 && parts[3] == "policy" {
			return catalog.ActionLambdaGetPolicy, injectFunctionNameJSON(body, name)
		}
		if len(parts) == 4 && parts[3] == "url" {
			return catalog.ActionLambdaGetFunctionUrlConfig, injectFunctionNameJSON(body, name)
		}
		if len(parts) == 4 && parts[3] == "urls" {
			return catalog.ActionLambdaListFunctionUrlConfigs, injectFunctionNameJSON(body, name)
		}
	case http.MethodDelete:
		if len(parts) == 3 {
			return catalog.ActionLambdaDeleteFunction, injectFunctionNameJSON(body, name)
		}
		// DELETE /2015-03-31/functions/{FunctionName}/policy/{StatementId}
		if len(parts) == 5 && parts[3] == "policy" {
			out := injectFunctionNameJSON(body, name)
			return catalog.ActionLambdaRemovePermission, injectJSONStringField(out, "StatementId", parts[4])
		}
		if len(parts) == 4 && parts[3] == "url" {
			return catalog.ActionLambdaDeleteFunctionUrlConfig, injectFunctionNameJSON(body, name)
		}
	case http.MethodPut:
		if len(parts) == 4 && parts[3] == "code" {
			return catalog.ActionLambdaUpdateFunctionCode, injectFunctionNameJSON(body, name)
		}
		if len(parts) == 4 && parts[3] == "configuration" {
			return catalog.ActionLambdaUpdateFunctionConfiguration, injectFunctionNameJSON(body, name)
		}
	}
	return "", body
}

// resolveLambdaLayersREST maps /{2015-03-31|2018-10-31}/layers/... (AWS CLI uses 2018-10-31).
func resolveLambdaLayersREST(method string, parts []string, body []byte) (string, []byte) {
	// parts: [apiVer, "layers", LayerName?, "versions"?, VersionNumber?]
	if len(parts) < 3 {
		return "", body
	}
	layerName, err := url.PathUnescape(parts[2])
	if err != nil {
		layerName = parts[2]
	}
	switch method {
	case http.MethodPost:
		if len(parts) == 4 && parts[3] == "versions" {
			return catalog.ActionLambdaPublishLayerVersion, injectLayerNameJSON(body, layerName)
		}
	case http.MethodGet:
		if len(parts) == 4 && parts[3] == "versions" {
			return catalog.ActionLambdaListLayerVersions, injectLayerNameJSON(body, layerName)
		}
		if len(parts) == 5 && parts[3] == "versions" {
			return catalog.ActionLambdaGetLayerVersion, injectLayerVersionJSON(body, layerName, parts[4])
		}
	case http.MethodDelete:
		if len(parts) == 5 && parts[3] == "versions" {
			return catalog.ActionLambdaDeleteLayerVersion, injectLayerVersionJSON(body, layerName, parts[4])
		}
	}
	return "", body
}

func injectFunctionNameJSON(body []byte, name string) []byte {
	return injectJSONStringField(body, "FunctionName", name)
}

func injectLayerNameJSON(body []byte, name string) []byte {
	return injectJSONStringField(body, "LayerName", name)
}

func injectLayerVersionJSON(body []byte, name, version string) []byte {
	out := injectLayerNameJSON(body, name)
	params := jsonBodyMap(out)
	if params == nil {
		params = map[string]any{}
	}
	if _, ok := params["VersionNumber"]; !ok {
		if n, err := strconv.ParseInt(version, 10, 64); err == nil {
			params["VersionNumber"] = n
		} else {
			params["VersionNumber"] = version
		}
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return out
	}
	return raw
}

func injectJSONStringField(body []byte, field, value string) []byte {
	if strings.TrimSpace(value) == "" {
		return body
	}
	params := jsonBodyMap(body)
	if params == nil {
		params = map[string]any{}
	}
	if existing, _ := params[field].(string); strings.TrimSpace(existing) == "" {
		params[field] = value
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return body
	}
	return raw
}

func invokeRESTBody(body []byte, name string) []byte {
	event := any(map[string]any{})
	if len(body) > 0 && json.Valid(body) {
		_ = json.Unmarshal(body, &event)
	}
	raw, err := json.Marshal(map[string]any{
		"FunctionName": name,
		"Payload":      event,
	})
	if err != nil {
		return body
	}
	return raw
}

func (s *Server) writeAWSError(
	w http.ResponseWriter,
	requestID string,
	status int,
	code string,
	message string,
	readOnly bool,
	r *http.Request,
	eventID string,
	accessKeyID string,
	accountID string,
	knownKey bool,
) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)

	resp := awsErrorResponse{
		XMLNS: "http://noctaxris.amazonaws.com/doc/2026-07-18/",
		Error: awsError{
			Code:    code,
			Message: message,
			Type:    "Sender",
		},
		RequestID: requestID,
	}
	data, err := xml.Marshal(resp)
	if err == nil {
		_, _ = w.Write([]byte(xml.Header))
		_, _ = w.Write(data)
	}

	eventSource := "noctaxris.amazonaws.com"
	eventName := eventNameForRequest(r)
	if strings.Contains(strings.ToLower(eventName), "getcalleridentity") ||
		strings.Contains(r.Header.Get("X-Amz-Target"), "GetCallerIdentity") {
		eventSource = "sts.amazonaws.com"
		eventName = "GetCallerIdentity"
	}

	ev := audit.Event{
		EventVersion:       eventVersion,
		EventTime:          s.now().UTC().Format(time.RFC3339),
		EventSource:        eventSource,
		EventName:          eventName,
		SourceIPAddress:    s.auditClientIP(r),
		UserAgent:          r.UserAgent(),
		RequestID:          requestID,
		EventID:            eventID,
		EventType:          "AwsApiCall",
		RecipientAccountID: s.cfg.AccountID,
		ReadOnly:           readOnly,
		ErrorCode:          code,
		ErrorMessage:       message,
		RequestParameters:  baseAuditRequestParams(r),
	}

	if knownKey {
		recipient := s.cfg.AccountID
		if accountID != "" {
			recipient = accountID
		}
		ev.RecipientAccountID = recipient
		uid := map[string]any{
			"type":        "IAMUser",
			"accountId":   recipient,
			"accessKeyId": accessKeyID,
		}
		if accessKeyID != "" && accessKeyID == s.cfg.RootAccessKeyID {
			uid["type"] = "Root"
			uid["userName"] = "root"
		}
		ev.UserIdentity = uid
	}

	_ = s.audit.Write(context.Background(), ev)
}

func readBody(r *http.Request, limit int64) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	enc := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding")))
	compressed := io.LimitReader(r.Body, limit+1)
	var src io.Reader = compressed
	switch enc {
	case "", "identity":
		// plaintext under compressed limit
	case "gzip":
		gz, err := gzip.NewReader(compressed)
		if err != nil {
			return nil, fmt.Errorf("gzip body: %w", err)
		}
		defer gz.Close()
		src = io.LimitReader(gz, limit+1)
	case "deflate":
		src = io.LimitReader(flate.NewReader(compressed), limit+1)
	default:
		return nil, fmt.Errorf("unsupported Content-Encoding %q", enc)
	}
	data, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("body too large")
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func resolveAction(r *http.Request, body []byte) string {
	if target := r.Header.Get("X-Amz-Target"); target != "" {
		dot := strings.LastIndex(target, ".")
		if dot < 0 || dot+1 >= len(target) {
			return target
		}
		short := target[dot+1:]
		prefix := target[:dot]
		switch {
		case strings.EqualFold(prefix, "AWSLambda"):
			return lambdaAction(short)
		case strings.EqualFold(prefix, "TrentService"), strings.EqualFold(prefix, "AWSKMS"):
			return normalizeAction(short)
		case strings.EqualFold(prefix, "AmazonSSM"):
			return ssmAction(short)
		case strings.EqualFold(prefix, "AWSEvents"):
			return eventsAction(short)
		case strings.EqualFold(prefix, "secretsmanager"):
			return secretsAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "amazonec2containerregistry_v"):
			return ecrAction(short)
		case strings.EqualFold(prefix, "ecr"):
			return ecrAction(short)
		case strings.EqualFold(prefix, "AmazonECS"),
			strings.HasPrefix(strings.ToLower(prefix), "amazonec2containerservice"):
			return ecsAction(short)
		case strings.Contains(strings.ToLower(prefix), "cloudtrail"):
			return cloudtrailAction(short)
		case strings.Contains(strings.ToLower(prefix), "guardduty"),
			strings.EqualFold(prefix, "NoctaxrisGuardDuty"):
			return guarddutyAction(short)
		case strings.Contains(strings.ToLower(prefix), "macie"),
			strings.EqualFold(prefix, "NoctaxrisMacie"):
			return macieAction(short)
		case strings.Contains(strings.ToLower(prefix), "detective"),
			strings.EqualFold(prefix, "NoctaxrisDetective"):
			return detectiveAction(short)
		case strings.EqualFold(prefix, "AmazonEC2"), strings.EqualFold(prefix, "AWSEC2"),
			strings.EqualFold(prefix, "NoctaxrisEC2"):
			return ec2Action(short)
		case strings.Contains(strings.ToLower(prefix), "securityhub"),
			strings.EqualFold(prefix, "SecurityHub"),
			strings.EqualFold(prefix, "AWSSecurityHub"):
			return securityHubAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "logs_"),
			strings.EqualFold(prefix, "Logs"):
			return logsAction(short)
		case strings.Contains(strings.ToLower(prefix), "resourcegroupstagging"),
			strings.EqualFold(prefix, "tagging"):
			return taggingAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "kinesis"):
			return kinesisAction(short)
		case strings.Contains(strings.ToLower(prefix), "appconfigdata"):
			return appconfigAction(short)
		case strings.Contains(strings.ToLower(prefix), "appconfig"):
			return appconfigAction(short)
		case strings.Contains(strings.ToLower(prefix), "stepfunctions"),
			strings.EqualFold(prefix, "AWSStepFunctions"):
			return sfnAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "codebuild"):
			return codebuildAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "codecommit"):
			return codecommitAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "awsbatch"),
			strings.EqualFold(prefix, "Batch"):
			return batchAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "codepipeline"):
			return codepipelineAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "firehose"):
			return firehoseAction(short)
		case strings.Contains(strings.ToLower(prefix), "scheduler"),
			strings.EqualFold(prefix, "AWSScheduler"):
			return schedulerAction(short)
		case strings.EqualFold(prefix, "AWSGlue"),
			strings.HasPrefix(strings.ToLower(prefix), "glue"):
			return glueAction(short)
		case strings.Contains(strings.ToLower(prefix), "waf"),
			strings.HasPrefix(strings.ToLower(prefix), "awswaf"):
			return wafAction(short)
		case strings.Contains(strings.ToLower(prefix), "dynamodbstreams"):
			return dynamodbstreamsAction(short)
		case strings.Contains(strings.ToLower(prefix), "dynamodb"):
			return dynamoAction(short)
		case strings.Contains(strings.ToLower(prefix), "pipes"):
			return pipesAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "mq."),
			strings.EqualFold(prefix, "AmazonMQ"),
			strings.EqualFold(prefix, "mq"):
			return mqAction(short)
		case strings.EqualFold(prefix, "AmazonMemoryDB"),
			strings.Contains(strings.ToLower(prefix), "memorydb"):
			return memorydbAction(short)
		case strings.EqualFold(prefix, "Kafka_1.0"),
			strings.HasPrefix(strings.ToLower(prefix), "kafka"),
			strings.EqualFold(prefix, "AmazonMSK"),
			strings.EqualFold(prefix, "MSK"):
			return mskAction(short)
		case strings.Contains(strings.ToLower(prefix), "athena"),
			strings.EqualFold(prefix, "AmazonAthena"):
			return athenaAction(short)
		case strings.Contains(strings.ToLower(prefix), "opensearch"),
			strings.EqualFold(prefix, "AmazonOpenSearchService"),
			strings.EqualFold(prefix, "es"):
			return opensearchAction(short)
		case strings.Contains(strings.ToLower(prefix), "rdsdata"),
			strings.EqualFold(prefix, "AmazonRDSDataService"),
			strings.EqualFold(prefix, "RDSDataService"):
			return rdsDataAction(short)
		case strings.Contains(strings.ToLower(prefix), "rds") &&
			!strings.Contains(strings.ToLower(prefix), "rdsdata"):
			return rdsAction(short)
		case strings.Contains(strings.ToLower(prefix), "transfer"):
			return transferAction(short)
		case strings.Contains(strings.ToLower(prefix), "certificatemanager"),
			strings.EqualFold(prefix, "ACM"):
			return acmAction(short)
		case strings.Contains(strings.ToLower(prefix), "controltower"):
			return controlTowerAction(short)
		case strings.Contains(strings.ToLower(prefix), "route53") &&
			!strings.Contains(strings.ToLower(prefix), "autonaming"):
			return route53Action(short)
		case strings.EqualFold(prefix, "NoctaxrisRoute53"):
			return route53Action(short)
		case strings.Contains(strings.ToLower(prefix), "autonaming"),
			strings.Contains(strings.ToLower(prefix), "servicediscovery"):
			return serviceDiscoveryAction(short)
		case strings.Contains(strings.ToLower(prefix), "pricelist"),
			strings.EqualFold(prefix, "AWSPriceListService"),
			strings.EqualFold(prefix, "pricing"):
			return pricingAction(short)
		case strings.Contains(strings.ToLower(prefix), "appsync"):
			return appsyncAction(short)
		case strings.Contains(strings.ToLower(prefix), "apigateway"):
			return apiGatewayV2Action(short)
		case strings.Contains(strings.ToLower(prefix), "cognito"):
			return cognitoAction(short)
		case strings.Contains(strings.ToLower(prefix), "cloudcontrol"),
			strings.EqualFold(prefix, "CloudApiService"):
			return cloudControlAction(short)
		case strings.Contains(strings.ToLower(prefix), "bcm"),
			strings.Contains(strings.ToLower(prefix), "dataexports"):
			return bcmExportAction(short)
		case strings.Contains(strings.ToLower(prefix), "insightsindex"),
			strings.EqualFold(prefix, "AWSInsightsIndexService"),
			strings.EqualFold(prefix, "ce"):
			return costExplorerAction(short)
		case strings.Contains(strings.ToLower(prefix), "budget"):
			return budgetsAction(short)
		case strings.Contains(strings.ToLower(prefix), "origami"),
			strings.EqualFold(prefix, "AWSOrigamiServiceGatewayService"),
			strings.EqualFold(prefix, "cur"):
			return curAction(short)
		case strings.Contains(strings.ToLower(prefix), "iotdata"),
			strings.Contains(strings.ToLower(prefix), "iot-data"),
			strings.EqualFold(prefix, "AWSIotDataService"),
			strings.EqualFold(prefix, "IotDataPlane"):
			return iotAction(short)
		case strings.Contains(strings.ToLower(prefix), "iot"),
			strings.EqualFold(prefix, "AWSIotService"),
			strings.EqualFold(prefix, "Iot"):
			return iotAction(short)
		case strings.Contains(strings.ToLower(prefix), "granite"),
			strings.EqualFold(prefix, "GraniteServiceVersion20100801"),
			strings.Contains(strings.ToLower(prefix), "monitoring"),
			strings.EqualFold(prefix, "CloudWatch"):
			return cloudWatchAction(short)
		case strings.Contains(strings.ToLower(prefix), "lightsail"):
			return lightsailAction(short)
		case strings.Contains(strings.ToLower(prefix), "autoscaling"),
			strings.EqualFold(prefix, "AutoScaling"):
			return asgAction(short)
		case strings.Contains(strings.ToLower(prefix), "elasticbeanstalk"),
			strings.EqualFold(prefix, "ElasticBeanstalk"):
			return beanstalkAction(short)
		case strings.Contains(strings.ToLower(prefix), "backup"),
			strings.EqualFold(prefix, "AWSBackup"):
			return backupAction(short)
		case strings.Contains(strings.ToLower(prefix), "codedeploy"):
			return codeDeployAction(short)
		case strings.Contains(strings.ToLower(prefix), "bedrock"):
			return bedrockAction(short)
		case strings.Contains(strings.ToLower(prefix), "textract"):
			return textractAction(short)
		case strings.Contains(strings.ToLower(prefix), "transcribe"):
			return transcribeAction(short)
		case strings.Contains(strings.ToLower(prefix), "elasticmapreduce"),
			strings.EqualFold(prefix, "ElasticMapReduce"):
			return emrAction(short)
		case strings.EqualFold(prefix, "NoctaxrisLab"):
			return labForensicsAction(short)
		}
		return short
	}
	if v := r.URL.Query().Get("Action"); v != "" {
		return normalizeAction(v)
	}
	if len(body) > 0 {
		vals, err := url.ParseQuery(string(body))
		if err == nil {
			if v := vals.Get("Action"); v != "" {
				return normalizeAction(v)
			}
		}
	}
	// Smithy RPC-v2 for rds-data: POST /Execute (no X-Amz-Target). Prefer JSON body
	// content types so path-style S3 object keys are not remapped.
	if ct := strings.ToLower(r.Header.Get("Content-Type")); strings.Contains(ct, "json") {
		if action := rdsDataActionFromPath(r.URL.Path); strings.HasPrefix(action, "rds-data:") {
			return action
		}
	}
	return ""
}

func normalizeAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "GetCallerIdentity":
		return catalog.ActionSTSGetCallerIdentity
	case "AssumeRole":
		return catalog.ActionSTSAssumeRole
	case "GetSessionToken":
		return catalog.ActionSTSGetSessionToken
	case "GetFederationToken":
		return catalog.ActionSTSGetFederationToken
	case "AssumeRoleWithSAML":
		return catalog.ActionSTSAssumeRoleWithSAML
	case "AssumeRoleWithWebIdentity":
		return catalog.ActionSTSAssumeRoleWithWebIdentity
	case "AssumeRoot":
		return catalog.ActionSTSAssumeRoot
	case "DecodeAuthorizationMessage":
		return catalog.ActionSTSDecodeAuthorizationMessage
	case "GetAccessKeyInfo":
		return catalog.ActionSTSGetAccessKeyInfo
	case "GetDelegatedAccessToken":
		return catalog.ActionSTSGetDelegatedAccessToken
	case "GetWebIdentityToken":
		return catalog.ActionSTSGetWebIdentityToken
	case "CreateAccount":
		return catalog.ActionOrgsCreateAccount
	case "DescribeCreateAccountStatus":
		return catalog.ActionOrgsDescribeCreateAccountStatus
	case "ListAccounts":
		return catalog.ActionOrgsListAccounts
	case "CreateOrganizationalUnit":
		return catalog.ActionOrgsCreateOrganizationalUnit
	case "ListOrganizationalUnitsForParent":
		return catalog.ActionOrgsListOrganizationalUnitsForParent
	case "EnablePolicyType":
		return catalog.ActionOrgsEnablePolicyType
	case "AttachPolicy":
		return catalog.ActionOrgsAttachPolicy
	case "DetachPolicy":
		return catalog.ActionOrgsDetachPolicy
	case "DescribePolicy":
		return catalog.ActionOrgsDescribePolicy
	case "ListPoliciesForTarget":
		return catalog.ActionOrgsListPoliciesForTarget
	case "ListParents":
		return catalog.ActionOrgsListParents
	case "ListAccountsForParent":
		return catalog.ActionOrgsListAccountsForParent
	case "MoveAccount":
		return catalog.ActionOrgsMoveAccount
	case "CreateUser":
		return catalog.ActionIAMCreateUser
	case "GetUser":
		return catalog.ActionIAMGetUser
	case "ListUsers":
		return catalog.ActionIAMListUsers
	case "DeleteUser":
		return catalog.ActionIAMDeleteUser
	case "CreateAccessKey":
		return catalog.ActionIAMCreateAccessKey
	case "DeleteAccessKey":
		return catalog.ActionIAMDeleteAccessKey
	case "ListAccessKeys":
		return catalog.ActionIAMListAccessKeys
	case "UpdateAccessKey":
		return catalog.ActionIAMUpdateAccessKey
	case "GetAccessKeyLastUsed":
		return catalog.ActionIAMGetAccessKeyLastUsed
	case "GenerateCredentialReport":
		return catalog.ActionIAMGenerateCredentialReport
	case "GetCredentialReport":
		return catalog.ActionIAMGetCredentialReport
	case "CreatePolicy":
		return catalog.ActionIAMCreatePolicy
	case "GetPolicy":
		return catalog.ActionIAMGetPolicy
	case "ListPolicies":
		return catalog.ActionIAMListPolicies
	case "DeletePolicy":
		return catalog.ActionIAMDeletePolicy
	case "CreatePolicyVersion":
		return catalog.ActionIAMCreatePolicyVersion
	case "GetPolicyVersion":
		return catalog.ActionIAMGetPolicyVersion
	case "ListPolicyVersions":
		return catalog.ActionIAMListPolicyVersions
	case "DeletePolicyVersion":
		return catalog.ActionIAMDeletePolicyVersion
	case "SetDefaultPolicyVersion":
		return catalog.ActionIAMSetDefaultPolicyVersion
	case "AttachUserPolicy":
		return catalog.ActionIAMAttachUserPolicy
	case "DetachUserPolicy":
		return catalog.ActionIAMDetachUserPolicy
	case "AttachRolePolicy":
		return catalog.ActionIAMAttachRolePolicy
	case "DetachRolePolicy":
		return catalog.ActionIAMDetachRolePolicy
	case "ListAttachedUserPolicies":
		return catalog.ActionIAMListAttachedUserPolicies
	case "ListAttachedRolePolicies":
		return catalog.ActionIAMListAttachedRolePolicies
	case "PutUserPolicy":
		return catalog.ActionIAMPutUserPolicy
	case "GetUserPolicy":
		return catalog.ActionIAMGetUserPolicy
	case "DeleteUserPolicy":
		return catalog.ActionIAMDeleteUserPolicy
	case "ListUserPolicies":
		return catalog.ActionIAMListUserPolicies
	case "PutRolePolicy":
		return catalog.ActionIAMPutRolePolicy
	case "GetRolePolicy":
		return catalog.ActionIAMGetRolePolicy
	case "DeleteRolePolicy":
		return catalog.ActionIAMDeleteRolePolicy
	case "ListRolePolicies":
		return catalog.ActionIAMListRolePolicies
	case "CreateRole":
		return catalog.ActionIAMCreateRole
	case "GetRole":
		return catalog.ActionIAMGetRole
	case "ListRoles":
		return catalog.ActionIAMListRoles
	case "DeleteRole":
		return catalog.ActionIAMDeleteRole
	case "UpdateAssumeRolePolicy":
		return catalog.ActionIAMUpdateAssumeRolePolicy
	case "CreateGroup":
		return catalog.ActionIAMCreateGroup
	case "DeleteGroup":
		return catalog.ActionIAMDeleteGroup
	case "GetGroup":
		return catalog.ActionIAMGetGroup
	case "ListGroups":
		return catalog.ActionIAMListGroups
	case "AddUserToGroup":
		return catalog.ActionIAMAddUserToGroup
	case "RemoveUserFromGroup":
		return catalog.ActionIAMRemoveUserFromGroup
	case "AttachGroupPolicy":
		return catalog.ActionIAMAttachGroupPolicy
	case "DetachGroupPolicy":
		return catalog.ActionIAMDetachGroupPolicy
	case "ListAttachedGroupPolicies":
		return catalog.ActionIAMListAttachedGroupPolicies
	case "PutGroupPolicy":
		return catalog.ActionIAMPutGroupPolicy
	case "GetGroupPolicy":
		return catalog.ActionIAMGetGroupPolicy
	case "DeleteGroupPolicy":
		return catalog.ActionIAMDeleteGroupPolicy
	case "ListGroupPolicies":
		return catalog.ActionIAMListGroupPolicies
	case "PutUserPermissionsBoundary":
		return catalog.ActionIAMPutUserPermissionsBoundary
	case "GetUserPermissionsBoundary":
		return catalog.ActionIAMGetUserPermissionsBoundary
	case "DeleteUserPermissionsBoundary":
		return catalog.ActionIAMDeleteUserPermissionsBoundary
	case "PutRolePermissionsBoundary":
		return catalog.ActionIAMPutRolePermissionsBoundary
	case "GetRolePermissionsBoundary":
		return catalog.ActionIAMGetRolePermissionsBoundary
	case "DeleteRolePermissionsBoundary":
		return catalog.ActionIAMDeleteRolePermissionsBoundary
	case "CreateInstanceProfile":
		return catalog.ActionIAMCreateInstanceProfile
	case "DeleteInstanceProfile":
		return catalog.ActionIAMDeleteInstanceProfile
	case "GetInstanceProfile":
		return catalog.ActionIAMGetInstanceProfile
	case "AddRoleToInstanceProfile":
		return catalog.ActionIAMAddRoleToInstanceProfile
	case "RemoveRoleFromInstanceProfile":
		return catalog.ActionIAMRemoveRoleFromInstanceProfile
	case "ListInstanceProfiles":
		return catalog.ActionIAMListInstanceProfiles
	case "ListInstanceProfilesForRole":
		return catalog.ActionIAMListInstanceProfilesForRole
	case "CreateOpenIDConnectProvider":
		return catalog.ActionIAMCreateOpenIDConnectProvider
	case "DeleteOpenIDConnectProvider":
		return catalog.ActionIAMDeleteOpenIDConnectProvider
	case "ListOpenIDConnectProviders":
		return catalog.ActionIAMListOpenIDConnectProviders
	case "GetOpenIDConnectProvider":
		return catalog.ActionIAMGetOpenIDConnectProvider
	case "CreateSAMLProvider":
		return catalog.ActionIAMCreateSAMLProvider
	case "DeleteSAMLProvider":
		return catalog.ActionIAMDeleteSAMLProvider
	case "ListSAMLProviders":
		return catalog.ActionIAMListSAMLProviders
	case "GetSAMLProvider":
		return catalog.ActionIAMGetSAMLProvider
	case "CreateVirtualMFADevice":
		return catalog.ActionIAMCreateVirtualMFADevice
	case "EnableMFADevice":
		return catalog.ActionIAMEnableMFADevice
	case "ListMFADevices":
		return catalog.ActionIAMListMFADevices
	case "DeactivateMFADevice":
		return catalog.ActionIAMDeactivateMFADevice
	case "CreateKey":
		return catalog.ActionKMSCreateKey
	case "DescribeKey":
		return catalog.ActionKMSDescribeKey
	case "ListKeys":
		return catalog.ActionKMSListKeys
	case "EnableKey":
		return catalog.ActionKMSEnableKey
	case "DisableKey":
		return catalog.ActionKMSDisableKey
	case "GetKeyPolicy":
		return catalog.ActionKMSGetKeyPolicy
	case "PutKeyPolicy":
		return catalog.ActionKMSPutKeyPolicy
	case "Encrypt":
		return catalog.ActionKMSEncrypt
	case "Decrypt":
		return catalog.ActionKMSDecrypt
	case "GenerateDataKey":
		return catalog.ActionKMSGenerateDataKey
	case "GenerateDataKeyWithoutPlaintext":
		return catalog.ActionKMSGenerateDataKeyWithoutPlaintext
	case "CreateGrant":
		return catalog.ActionKMSCreateGrant
	case "ListGrants":
		return catalog.ActionKMSListGrants
	case "RetireGrant":
		return catalog.ActionKMSRetireGrant
	case "RevokeGrant":
		return catalog.ActionKMSRevokeGrant
	case "CreateAlias":
		return catalog.ActionKMSCreateAlias
	case "ListAliases":
		return catalog.ActionKMSListAliases
	case "DeleteAlias":
		return catalog.ActionKMSDeleteAlias
	case "UpdateAlias":
		return catalog.ActionKMSUpdateAlias
	case "ScheduleKeyDeletion":
		return catalog.ActionKMSScheduleKeyDeletion
	case "CancelKeyDeletion":
		return catalog.ActionKMSCancelKeyDeletion
	case "EnableKeyRotation":
		return catalog.ActionKMSEnableKeyRotation
	case "DisableKeyRotation":
		return catalog.ActionKMSDisableKeyRotation
	case "GetKeyRotationStatus":
		return catalog.ActionKMSGetKeyRotationStatus
	case "ListResourceTags":
		return catalog.ActionKMSListResourceTags
	case "TagResource":
		return catalog.ActionKMSTagResource
	case "UntagResource":
		return catalog.ActionKMSUntagResource
	case "CreateTable":
		return catalog.ActionDynamoDBCreateTable
	case "DescribeTable":
		return catalog.ActionDynamoDBDescribeTable
	case "DeleteTable":
		return catalog.ActionDynamoDBDeleteTable
	case "ListTables":
		return catalog.ActionDynamoDBListTables
	case "UpdateTable":
		return catalog.ActionDynamoDBUpdateTable
	case "PutItem":
		return catalog.ActionDynamoDBPutItem
	case "GetItem":
		return catalog.ActionDynamoDBGetItem
	case "DeleteItem":
		return catalog.ActionDynamoDBDeleteItem
	case "UpdateItem":
		return catalog.ActionDynamoDBUpdateItem
	case "Query":
		return catalog.ActionDynamoDBQuery
	case "Scan":
		return catalog.ActionDynamoDBScan
	case "BatchGetItem":
		return catalog.ActionDynamoDBBatchGetItem
	case "BatchWriteItem":
		return catalog.ActionDynamoDBBatchWriteItem
	case "TransactGetItems":
		return catalog.ActionDynamoDBTransactGetItems
	case "TransactWriteItems":
		return catalog.ActionDynamoDBTransactWriteItems
	case "PutResourcePolicy":
		return catalog.ActionDynamoDBPutResourcePolicy
	case "GetResourcePolicy":
		return catalog.ActionDynamoDBGetResourcePolicy
	case "DeleteResourcePolicy":
		return catalog.ActionDynamoDBDeleteResourcePolicy
	case "UpdateTimeToLive":
		return catalog.ActionDynamoDBUpdateTimeToLive
	case "DescribeTimeToLive":
		return catalog.ActionDynamoDBDescribeTimeToLive
	case "DescribeContinuousBackups":
		return catalog.ActionDynamoDBDescribeContinuousBackups
	case "ListTagsOfResource":
		return catalog.ActionDynamoDBListTagsOfResource
	case "CreateQueue":
		return catalog.ActionSQSCreateQueue
	case "GetQueueUrl":
		return catalog.ActionSQSGetQueueUrl
	case "GetQueueAttributes":
		return catalog.ActionSQSGetQueueAttributes
	case "SetQueueAttributes":
		return catalog.ActionSQSSetQueueAttributes
	case "DeleteQueue":
		return catalog.ActionSQSDeleteQueue
	case "ListQueues":
		return catalog.ActionSQSListQueues
	case "PurgeQueue":
		return catalog.ActionSQSPurgeQueue
	case "SendMessage":
		return catalog.ActionSQSSendMessage
	case "ReceiveMessage":
		return catalog.ActionSQSReceiveMessage
	case "DeleteMessage":
		return catalog.ActionSQSDeleteMessage
	case "SendMessageBatch":
		return catalog.ActionSQSSendMessageBatch
	case "DeleteMessageBatch":
		return catalog.ActionSQSDeleteMessageBatch
	case "ChangeMessageVisibility":
		return catalog.ActionSQSChangeMessageVisibility
	case "ListQueueTags":
		return catalog.ActionSQSListQueueTags
	case "TagQueue":
		return catalog.ActionSQSTagQueue
	case "UntagQueue":
		return catalog.ActionSQSUntagQueue
	case "CreateTopic":
		return catalog.ActionSNSCreateTopic
	case "DeleteTopic":
		return catalog.ActionSNSDeleteTopic
	case "ListTopics":
		return catalog.ActionSNSListTopics
	case "GetTopicAttributes":
		return catalog.ActionSNSGetTopicAttributes
	case "SetTopicAttributes":
		return catalog.ActionSNSSetTopicAttributes
	case "Publish":
		return catalog.ActionSNSPublish
	case "Subscribe":
		return catalog.ActionSNSSubscribe
	case "Unsubscribe":
		return catalog.ActionSNSUnsubscribe
	case "ListSubscriptions":
		return catalog.ActionSNSListSubscriptions
	case "ListSubscriptionsByTopic":
		return catalog.ActionSNSListSubscriptionsByTopic
	case "GetSubscriptionAttributes":
		return catalog.ActionSNSGetSubscriptionAttributes
	case "PutParameter":
		return catalog.ActionSSMPutParameter
	case "GetParameter":
		return catalog.ActionSSMGetParameter
	case "GetParameters":
		return catalog.ActionSSMGetParameters
	case "GetParametersByPath":
		return catalog.ActionSSMGetParametersByPath
	case "DeleteParameter":
		return catalog.ActionSSMDeleteParameter
	case "DescribeParameters":
		return catalog.ActionSSMDescribeParameters
	case "AddTagsToResource":
		return catalog.ActionSSMAddTagsToResource
	case "RemoveTagsFromResource":
		return catalog.ActionSSMRemoveTagsFromResource
	case "CreateSecret":
		return catalog.ActionSecretsCreateSecret
	case "GetSecretValue":
		return catalog.ActionSecretsGetSecretValue
	case "PutSecretValue":
		return catalog.ActionSecretsPutSecretValue
	case "DeleteSecret":
		return catalog.ActionSecretsDeleteSecret
	case "RestoreSecret":
		return catalog.ActionSecretsRestoreSecret
	case "RotateSecret":
		return catalog.ActionSecretsRotateSecret
	case "UpdateSecretVersionStage":
		return catalog.ActionSecretsUpdateSecretVersionStage
	case "DescribeSecret":
		return catalog.ActionSecretsDescribeSecret
	case "ListSecrets":
		return catalog.ActionSecretsListSecrets
	case "PutEvents":
		return catalog.ActionEventsPutEvents
	case "CreateEventBus":
		return catalog.ActionEventsCreateEventBus
	case "DeleteEventBus":
		return catalog.ActionEventsDeleteEventBus
	case "DescribeEventBus":
		return catalog.ActionEventsDescribeEventBus
	case "ListEventBuses":
		return catalog.ActionEventsListEventBuses
	case "PutRule":
		return catalog.ActionEventsPutRule
	case "DescribeRule":
		return catalog.ActionEventsDescribeRule
	case "ListRules":
		return catalog.ActionEventsListRules
	case "DeleteRule":
		return catalog.ActionEventsDeleteRule
	case "EnableRule":
		return catalog.ActionEventsEnableRule
	case "DisableRule":
		return catalog.ActionEventsDisableRule
	case "PutTargets":
		return catalog.ActionEventsPutTargets
	case "RemoveTargets":
		return catalog.ActionEventsRemoveTargets
	case "ListTargetsByRule":
		return catalog.ActionEventsListTargetsByRule
	case "PutPermission":
		return catalog.ActionEventsPutPermission
	case "RemovePermission":
		return catalog.ActionEventsRemovePermission
	case "CreateFunction":
		return catalog.ActionLambdaCreateFunction
	case "GetFunction":
		return catalog.ActionLambdaGetFunction
	case "DeleteFunction":
		return catalog.ActionLambdaDeleteFunction
	case "ListFunctions":
		return catalog.ActionLambdaListFunctions
	case "UpdateFunctionCode":
		return catalog.ActionLambdaUpdateFunctionCode
	case "UpdateFunctionConfiguration":
		return catalog.ActionLambdaUpdateFunctionConfiguration
	case "Invoke":
		return catalog.ActionLambdaInvoke
	case "PublishVersion":
		return catalog.ActionLambdaPublishVersion
	case "ListVersionsByFunction":
		return catalog.ActionLambdaListVersionsByFunction
	case "ListTags":
		return catalog.ActionLambdaListTags
	case "GetAlias":
		return catalog.ActionLambdaGetAlias
	case "RegisterTaskDefinition":
		return catalog.ActionECSRegisterTaskDefinition
	case "DescribeTaskDefinition":
		return catalog.ActionECSDescribeTaskDefinition
	case "ListTaskDefinitions":
		return catalog.ActionECSListTaskDefinitions
	case "DeregisterTaskDefinition":
		return catalog.ActionECSDeregisterTaskDefinition
	case "RunTask":
		return catalog.ActionECSRunTask
	case "DescribeTasks":
		return catalog.ActionECSDescribeTasks
	case "ListTasks":
		return catalog.ActionECSListTasks
	case "StopTask":
		return catalog.ActionECSStopTask
	case "DescribeClusters":
		return catalog.ActionECSDescribeClusters
	case "ListClusters":
		return catalog.ActionECSListClusters
	case "CreateService":
		return catalog.ActionECSCreateService
	case "UpdateService":
		return catalog.ActionECSUpdateService
	case "DeleteService":
		return catalog.ActionECSDeleteService
	case "DescribeServices":
		return catalog.ActionECSDescribeServices
	case "ListServices":
		return catalog.ActionECSListServices
	case "CreateEventSourceMapping":
		return catalog.ActionLambdaCreateEventSourceMapping
	case "GetEventSourceMapping":
		return catalog.ActionLambdaGetEventSourceMapping
	case "ListEventSourceMappings":
		return catalog.ActionLambdaListEventSourceMappings
	case "UpdateEventSourceMapping":
		return catalog.ActionLambdaUpdateEventSourceMapping
	case "DeleteEventSourceMapping":
		return catalog.ActionLambdaDeleteEventSourceMapping
	case "CreateFunctionUrlConfig":
		return catalog.ActionLambdaCreateFunctionUrlConfig
	case "GetFunctionUrlConfig":
		return catalog.ActionLambdaGetFunctionUrlConfig
	case "DeleteFunctionUrlConfig":
		return catalog.ActionLambdaDeleteFunctionUrlConfig
	case "ListFunctionUrlConfigs":
		return catalog.ActionLambdaListFunctionUrlConfigs
	case "LookupEvents":
		return catalog.ActionCloudTrailLookupEvents
	case "CreateLogGroup":
		return catalog.ActionLogsCreateLogGroup
	case "CreateLogStream":
		return catalog.ActionLogsCreateLogStream
	case "DeleteLogGroup":
		return catalog.ActionLogsDeleteLogGroup
	case "DeleteLogStream":
		return catalog.ActionLogsDeleteLogStream
	case "DescribeLogStreams":
		return catalog.ActionLogsDescribeLogStreams
	case "PutLogEvents":
		return catalog.ActionLogsPutLogEvents
	case "GetLogEvents":
		return catalog.ActionLogsGetLogEvents
	case "FilterLogEvents":
		return catalog.ActionLogsFilterLogEvents
	case "DescribeLogGroups":
		return catalog.ActionLogsDescribeLogGroups
	case "PutRetentionPolicy":
		return catalog.ActionLogsPutRetentionPolicy
	case "DeleteRetentionPolicy":
		return catalog.ActionLogsDeleteRetentionPolicy
	case "TagResources":
		return catalog.ActionTaggingTagResources
	case "UntagResources":
		return catalog.ActionTaggingUntagResources
	case "GetResources":
		return catalog.ActionTaggingGetResources
	case "CreateStream":
		return catalog.ActionKinesisCreateStream
	case "DeleteStream":
		return catalog.ActionKinesisDeleteStream
	case "DescribeStream":
		return catalog.ActionKinesisDescribeStream
	case "ListStreams":
		return catalog.ActionKinesisListStreams
	case "PutRecord":
		return catalog.ActionKinesisPutRecord
	case "PutRecords":
		return catalog.ActionKinesisPutRecords
	case "GetShardIterator":
		return catalog.ActionKinesisGetShardIterator
	case "GetRecords":
		return catalog.ActionKinesisGetRecords
	case "VerifyEmailIdentity":
		return catalog.ActionSESVerifyEmailIdentity
	case "SendEmail":
		return catalog.ActionSESSendEmail
	case "SendRawEmail":
		return catalog.ActionSESSendRawEmail
	case "ListIdentities":
		return catalog.ActionSESListIdentities
	case "GetSendStatistics":
		return catalog.ActionSESGetSendStatistics
	case "SetIdentityNotificationTopic":
		return catalog.ActionSESSetIdentityNotificationTopic
	// CreateApplication / CreateEnvironment are ambiguous (AppConfig, Elastic Beanstalk,
	// CodeDeploy). Leave short names; route via X-Amz-Target or SigV4 service.
	case "CreateConfigurationProfile":
		return catalog.ActionAppConfigCreateConfigurationProfile
	case "CreateHostedConfigurationVersion":
		return catalog.ActionAppConfigCreateHostedConfigurationVersion
	case "GetConfiguration":
		return catalog.ActionAppConfigGetConfiguration
	case "StartConfigurationSession":
		return catalog.ActionAppConfigDataStartConfigurationSession
	case "GetLatestConfiguration":
		return catalog.ActionAppConfigDataGetLatestConfiguration
	case "CreateStateMachine":
		return catalog.ActionSFNCreateStateMachine
	case "DeleteStateMachine":
		return catalog.ActionSFNDeleteStateMachine
	case "DescribeStateMachine":
		return catalog.ActionSFNDescribeStateMachine
	case "ListStateMachines":
		return catalog.ActionSFNListStateMachines
	case "StartExecution":
		return catalog.ActionSFNStartExecution
	case "DescribeExecution":
		return catalog.ActionSFNDescribeExecution
	case "GetExecutionHistory":
		return catalog.ActionSFNGetExecutionHistory
	default:
		return action
	}
}

func isUnauthenticatedSTSAction(action string) bool {
	switch action {
	case catalog.ActionSTSAssumeRoleWithSAML, "AssumeRoleWithSAML",
		catalog.ActionSTSAssumeRoleWithWebIdentity, "AssumeRoleWithWebIdentity":
		return true
	default:
		return false
	}
}

func isUnauthenticatedCognitoAction(action string) bool {
	switch action {
	case catalog.ActionCognitoInitiateAuth, "InitiateAuth",
		catalog.ActionCognitoConfirmForgotPassword, "ConfirmForgotPassword",
		catalog.ActionCognitoUpdateUserAttributes, "UpdateUserAttributes",
		catalog.ActionCognitoGetUserAttributeVerificationCode, "GetUserAttributeVerificationCode",
		catalog.ActionCognitoVerifyUserAttribute, "VerifyUserAttribute",
		catalog.ActionCognitoRevokeToken, "RevokeToken",
		catalog.ActionCognitoAssociateSoftwareToken, "AssociateSoftwareToken",
		catalog.ActionCognitoVerifySoftwareToken, "VerifySoftwareToken",
		catalog.ActionCognitoRespondToAuthChallenge, "RespondToAuthChallenge":
		return true
	default:
		return false
	}
}

func federationRegion(r *http.Request) string {
	_ = r
	return "us-east-1"
}

func defaultAuthnMessage(code string) string {
	switch code {
	case authn.CodeMissingAuthenticationToken:
		return "The request must contain a valid signature or access key."
	case authn.CodeInvalidClientTokenId:
		return "The security token included in the request is invalid."
	case authn.CodeSignatureDoesNotMatch:
		return "The request signature we calculated does not match the signature you provided."
	case authn.CodeRequestTimeTooSkewed:
		return "The difference between the request time and the server time is too large."
	default:
		return "The request was rejected because authentication failed."
	}
}

func parseAccessKeyID(authorization string) (string, bool) {
	const prefix = "Credential="
	idx := strings.Index(authorization, prefix)
	if idx < 0 {
		return "", false
	}
	rest := authorization[idx+len(prefix):]
	slash := strings.Index(rest, "/")
	if slash <= 0 {
		return "", false
	}
	return rest[:slash], true
}

func eventNameForRequest(r *http.Request) string {
	if target := r.Header.Get("X-Amz-Target"); target != "" {
		if i := strings.LastIndex(target, "."); i >= 0 && i+1 < len(target) {
			return target[i+1:]
		}
		return target
	}
	if v := r.URL.Query().Get("Action"); v != "" {
		return v
	}
	return r.Method + " " + r.URL.Path
}

func peerClientIP(r *http.Request) string {
	if host, _, ok := strings.Cut(r.RemoteAddr, ":"); ok && host != "" {
		return host
	}
	return r.RemoteAddr
}

// clientIP is the TCP peer address used for authz (aws:SourceIp). Do not trust XFF here.
func clientIP(r *http.Request) string {
	return peerClientIP(r)
}

// auditClientIP is sourceIPAddress for CloudTrail-shaped audit lines.
// When NOCTAXRIS_CLOUDTRAIL_TRUST_XFF is enabled and the TCP peer is in
// TrustedProxies, the first X-Forwarded-For hop is used; otherwise the TCP
// peer address is used (secure default).
func (s *Server) auditClientIP(r *http.Request) string {
	if s != nil && s.cfg.CloudTrailTrustXFF {
		peer := peerHostOnly(r)
		if config.PeerInTrustedProxies(peer, s.cfg.TrustedProxies) {
			if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
				first := strings.TrimSpace(strings.Split(xff, ",")[0])
				if first != "" {
					if host, _, err := net.SplitHostPort(first); err == nil && host != "" {
						return host
					}
					return first
				}
			}
		}
	}
	return peerClientIP(r)
}

func newRequestID() string {
	return uuid.NewString()
}
