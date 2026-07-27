package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	rdssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/rds"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	rdsEventSource = "rds.amazonaws.com"
	rdsXMLType     = "text/xml"
)

func (s *Server) handleRDS(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	action = rdsAction(action)

	switch action {
	case catalog.ActionRDSCreateDBInstance:
		s.rdsCreateDBInstance(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionRDSDescribeDBInstances:
		s.rdsDescribeDBInstances(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionRDSDeleteDBInstance:
		s.rdsDeleteDBInstance(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeRDSError(w, r, body, requestID, http.StatusBadRequest, "InvalidAction",
			"This RDS action is not implemented.", readOnly, eventID, verified)
	}
}

func rdsAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateDBInstance":
		return catalog.ActionRDSCreateDBInstance
	case "DescribeDBInstances":
		return catalog.ActionRDSDescribeDBInstances
	case "DeleteDBInstance":
		return catalog.ActionRDSDeleteDBInstance
	default:
		return action
	}
}

func (s *Server) rdsRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultRDSRegion
}

func (s *Server) rdsCreateDBInstance(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string][]string,
) {
	_ = body
	identifier := firstParam(params, "DBInstanceIdentifier")
	engine := firstParam(params, "Engine")
	if !s.authorize(verified, catalog.ActionRDSCreateDBInstance, "*") {
		s.writeRDSError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform rds:CreateDBInstance.", readOnly, eventID, verified)
		return
	}
	class := firstParam(params, "DBInstanceClass")
	version := firstParam(params, "EngineVersion")
	dbName := firstParam(params, "DBName")
	user := firstParam(params, "MasterUsername")
	password := firstParam(params, "MasterUserPassword")
	secretARN := firstParam(params, "MasterUserSecretArn")
	if secretARN == "" {
		secretARN = firstParam(params, "MasterUserSecretARN")
	}
	storage := 20
	if v := firstParam(params, "AllocatedStorage"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			storage = n
		}
	}

	region := s.rdsRegion(verified)
	inst, err := s.store.CreateRDSDBInstance(verified.AccountID, region, store.CreateRDSDBInstanceInput{
		DBInstanceIdentifier: identifier,
		Engine:               engine,
		EngineVersion:        version,
		DBInstanceClass:      class,
		DBName:               dbName,
		MasterUsername:       user,
		MasterUserPassword:   password,
		MasterUserSecretARN:  secretARN,
		AllocatedStorage:     storage,
		Status:               "creating",
	})
	if errors.Is(err, store.ErrRDSInstanceExists) {
		s.writeRDSError(w, r, body, requestID, http.StatusBadRequest, "DBInstanceAlreadyExists",
			"DB instance already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrRDSBadRequest) {
		s.writeRDSError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeRDSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create DB instance.", readOnly, eventID, verified)
		return
	}

	inst = s.rdsTryStartNested(r.Context(), verified, inst, password)

	payload, err := rdssvc.CreateDBInstanceXML(inst, requestID)
	if err != nil {
		s.writeRDSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeRDSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, rdsEventSource, "CreateDBInstance", readOnly)
}

func (s *Server) rdsTryStartNested(
	ctx context.Context, verified *authn.Verified, inst store.RDSDBInstance, plaintextPassword string,
) store.RDSDBInstance {
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		// Without DinD, control-plane create stays creating (documented).
		return inst
	}
	password := plaintextPassword
	if password == "" && inst.MasterUserSecretARN != "" {
		sec, secErr := s.store.GetSecretValue(verified.AccountID, inst.MasterUserSecretARN)
		if secErr == nil {
			_, password, _ = store.ParseRDSMasterSecret(sec.SecretString)
		}
	}
	if password == "" {
		if strings.TrimSpace(s.cfg.DockerHost) != "" {
			_ = s.store.UpdateRDSDBInstanceRuntime(
				verified.AccountID, inst.DBInstanceIdentifier, "failed", "", "", 0,
			)
			if updated, getErr := s.store.DescribeRDSDBInstance(verified.AccountID, inst.DBInstanceIdentifier); getErr == nil {
				return updated
			}
		}
		return inst
	}
	name := "noctaxris-data-rds-" + inst.DBInstanceIdentifier
	startCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	dp, err := cli.StartDataPlane(startCtx, compute.DataPlaneOpts{
		Kind:  compute.DataKindRDS,
		Image: store.DefaultRDSPostgresImage,
		Name:  name,
		Env: map[string]string{
			"POSTGRES_USER":     inst.MasterUsername,
			"POSTGRES_PASSWORD": password,
			"POSTGRES_DB":       inst.DBName,
		},
		ContainerPort: 5432,
	})
	if err != nil {
		// DinD configured but start failed: surface failed status (not silent creating).
		log.Printf("rds nested start failed id=%s: %v", inst.DBInstanceIdentifier, err)
		_ = s.store.UpdateRDSDBInstanceRuntime(
			verified.AccountID, inst.DBInstanceIdentifier, "failed", "", "", 0,
		)
		if updated, getErr := s.store.DescribeRDSDBInstance(verified.AccountID, inst.DBInstanceIdentifier); getErr == nil {
			return updated
		}
		return inst
	}
	if waitErr := cli.WaitDataPlaneHealthy(startCtx, dp.ContainerID); waitErr != nil {
		evidence := waitErr.Error()
		if logs, logErr := cli.DataPlaneLogs(startCtx, dp.ContainerID); logErr == nil && strings.TrimSpace(logs) != "" {
			evidence = evidence + "\n" + logs
		}
		log.Printf("rds nested healthy wait failed id=%s: %s", inst.DBInstanceIdentifier, evidence)
		_ = s.store.UpdateRDSDBInstanceRuntime(
			verified.AccountID, inst.DBInstanceIdentifier, "failed", "", "", 0,
		)
		if updated, getErr := s.store.DescribeRDSDBInstance(verified.AccountID, inst.DBInstanceIdentifier); getErr == nil {
			return updated
		}
		return inst
	}
	host := strings.Split(dp.Endpoint, ":")[0]
	_ = s.store.UpdateRDSDBInstanceRuntime(
		verified.AccountID, inst.DBInstanceIdentifier, "available", dp.ContainerID, host, 5432,
	)
	updated, err := s.store.DescribeRDSDBInstance(verified.AccountID, inst.DBInstanceIdentifier)
	if err != nil {
		return inst
	}
	return updated
}

func (s *Server) rdsDescribeDBInstances(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string][]string,
) {
	if !s.authorize(verified, catalog.ActionRDSDescribeDBInstances, "*") {
		s.writeRDSError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform rds:DescribeDBInstances.", readOnly, eventID, verified)
		return
	}
	identifier := firstParam(params, "DBInstanceIdentifier")
	var instances []store.RDSDBInstance
	var err error
	if identifier != "" {
		inst, dErr := s.store.DescribeRDSDBInstance(verified.AccountID, identifier)
		if errors.Is(dErr, store.ErrRDSInstanceNotFound) {
			s.writeRDSError(w, r, body, requestID, http.StatusBadRequest, "DBInstanceNotFound",
				"DB instance not found.", readOnly, eventID, verified)
			return
		}
		if dErr != nil {
			s.writeRDSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to describe DB instances.", readOnly, eventID, verified)
			return
		}
		instances = []store.RDSDBInstance{inst}
	} else {
		instances, err = s.store.ListRDSDBInstances(verified.AccountID)
		if err != nil {
			s.writeRDSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to describe DB instances.", readOnly, eventID, verified)
			return
		}
	}
	payload, err := rdssvc.DescribeDBInstancesXML(instances, requestID)
	if err != nil {
		s.writeRDSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeRDSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, rdsEventSource, "DescribeDBInstances", readOnly)
}

func (s *Server) rdsDeleteDBInstance(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string][]string,
) {
	identifier := firstParam(params, "DBInstanceIdentifier")
	if !s.authorize(verified, catalog.ActionRDSDeleteDBInstance, "*") {
		s.writeRDSError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform rds:DeleteDBInstance.", readOnly, eventID, verified)
		return
	}
	inst, err := s.store.DeleteRDSDBInstance(verified.AccountID, identifier)
	if errors.Is(err, store.ErrRDSInstanceNotFound) {
		s.writeRDSError(w, r, body, requestID, http.StatusBadRequest, "DBInstanceNotFound",
			"DB instance not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeRDSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete DB instance.", readOnly, eventID, verified)
		return
	}
	if inst.ContainerID != "" {
		_ = tryStopNestedDataEngine(s, inst.ContainerID)
	}
	inst.DBInstanceStatus = "deleted"
	payload, err := rdssvc.DeleteDBInstanceXML(inst, requestID)
	if err != nil {
		s.writeRDSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeRDSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, rdsEventSource, "DeleteDBInstance", readOnly)
}

func firstParam(params map[string][]string, key string) string {
	vals := params[key]
	if len(vals) == 0 {
		return ""
	}
	return strings.TrimSpace(vals[0])
}

func (s *Server) writeRDSOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", rdsXMLType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeRDSError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", rdsXMLType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(status)
	_, _ = w.Write(rdssvc.ErrorXML(code, message, requestID))
	_ = body
	_ = r
	_ = readOnly
	_ = eventID
	_ = verified
}
