package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrRDSInstanceExists   = errors.New("DBInstanceAlreadyExists")
	ErrRDSInstanceNotFound = errors.New("DBInstanceNotFound")
	ErrRDSBadRequest       = errors.New("InvalidParameterValue")
)

const DefaultRDSRegion = "us-east-1"

const DefaultRDSPostgresImage = "postgres:16-alpine"

const rdsSchema = `
CREATE TABLE IF NOT EXISTS rds_db_instances (
  account_id TEXT NOT NULL,
  db_instance_identifier TEXT NOT NULL,
  db_instance_arn TEXT NOT NULL,
  engine TEXT NOT NULL,
  engine_version TEXT NOT NULL DEFAULT '16',
  db_instance_class TEXT NOT NULL DEFAULT 'db.t3.micro',
  db_name TEXT NOT NULL DEFAULT '',
  master_username TEXT NOT NULL DEFAULT 'postgres',
  master_user_secret_arn TEXT NOT NULL DEFAULT '',
  endpoint_address TEXT NOT NULL DEFAULT '',
  endpoint_port INTEGER NOT NULL DEFAULT 5432,
  db_instance_status TEXT NOT NULL,
  container_id TEXT NOT NULL DEFAULT '',
  allocated_storage INTEGER NOT NULL DEFAULT 20,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, db_instance_identifier)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_rds_db_instances_arn ON rds_db_instances(db_instance_arn);
`

// RDSDBInstance is a lab RDS DB instance control-plane row.
type RDSDBInstance struct {
	DBInstanceIdentifier string
	DBInstanceARN        string
	Engine               string
	EngineVersion        string
	DBInstanceClass      string
	DBName               string
	MasterUsername       string
	MasterUserSecretARN  string
	EndpointAddress      string
	EndpointPort         int
	DBInstanceStatus     string
	ContainerID          string
	AllocatedStorage     int
	CreatedAt            int64
}

// CreateRDSDBInstanceInput is the create path for nested Postgres labs.
type CreateRDSDBInstanceInput struct {
	DBInstanceIdentifier string
	Engine               string
	EngineVersion        string
	DBInstanceClass      string
	DBName               string
	MasterUsername       string
	// MasterUserPassword creates or updates a Secrets Manager secret when set.
	MasterUserPassword string
	// MasterUserSecretARN accepts an existing secret ARN (takes precedence when
	// MasterUserPassword is empty).
	MasterUserSecretARN string
	AllocatedStorage    int
	// EndpointAddress is the nested-network hostname (without port).
	EndpointAddress string
	EndpointPort    int
	ContainerID     string
	Status          string
}

// EnsureRDSSchema creates RDS tables if missing.
func EnsureRDSSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure rds schema: db is nil")
	}
	if _, err := db.Exec(rdsSchema); err != nil {
		return fmt.Errorf("ensure rds schema: %w", err)
	}
	return nil
}

// EnsureRDSSchema ensures RDS tables on an open store.
func (s *Store) EnsureRDSSchema() error {
	return EnsureRDSSchema(s.db)
}

// RDSDBInstanceARN builds arn:aws:rds:REGION:ACCOUNT:db:IDENTIFIER
func RDSDBInstanceARN(region, accountID, identifier string) string {
	if region == "" {
		region = DefaultRDSRegion
	}
	return fmt.Sprintf("arn:aws:rds:%s:%s:db:%s", region, accountID, identifier)
}

// CreateRDSDBInstance inserts a DB instance row. Nested engine start is the caller's job.
func (s *Store) CreateRDSDBInstance(accountID, region string, in CreateRDSDBInstanceInput) (RDSDBInstance, error) {
	id := strings.ToLower(strings.TrimSpace(in.DBInstanceIdentifier))
	if id == "" {
		return RDSDBInstance{}, fmt.Errorf("%w: DBInstanceIdentifier is required", ErrRDSBadRequest)
	}
	engine := strings.ToLower(strings.TrimSpace(in.Engine))
	if engine == "" {
		engine = "postgres"
	}
	if engine != "postgres" {
		return RDSDBInstance{}, fmt.Errorf("%w: Engine must be postgres for this lab core", ErrRDSBadRequest)
	}
	if region == "" {
		region = DefaultRDSRegion
	}
	class := strings.TrimSpace(in.DBInstanceClass)
	if class == "" {
		class = "db.t3.micro"
	}
	version := strings.TrimSpace(in.EngineVersion)
	if version == "" {
		version = "16"
	}
	user := strings.TrimSpace(in.MasterUsername)
	if user == "" {
		user = "postgres"
	}
	port := in.EndpointPort
	if port <= 0 {
		port = 5432
	}
	storage := in.AllocatedStorage
	if storage <= 0 {
		storage = 20
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "creating"
	}
	dbName := strings.TrimSpace(in.DBName)
	if dbName == "" {
		dbName = "postgres"
	}

	var existing string
	err := s.db.QueryRow(
		`SELECT db_instance_identifier FROM rds_db_instances WHERE account_id = ? AND db_instance_identifier = ?`,
		accountID, id,
	).Scan(&existing)
	if err == nil {
		return RDSDBInstance{}, ErrRDSInstanceExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RDSDBInstance{}, fmt.Errorf("create rds instance: %w", err)
	}

	secretARN := strings.TrimSpace(in.MasterUserSecretARN)
	password := strings.TrimSpace(in.MasterUserPassword)
	if secretARN == "" {
		if password == "" {
			password, err = randomLabPassword()
			if err != nil {
				return RDSDBInstance{}, fmt.Errorf("create rds instance: password: %w", err)
			}
		}
		secretName := RDSMasterSecretName(id)
		secretString, err := RDSMasterSecretJSON(user, password)
		if err != nil {
			return RDSDBInstance{}, err
		}
		sec, err := s.CreateSecret(accountID, region, secretName, secretString, nil, "", "Noctaxris RDS master user", "")
		if err != nil {
			if errors.Is(err, ErrSecretAlreadyExists) {
				if _, putErr := s.PutSecretValue(accountID, secretName, secretString, nil); putErr != nil {
					return RDSDBInstance{}, fmt.Errorf("create rds instance: put secret: %w", putErr)
				}
				sec, err = s.DescribeSecret(accountID, secretName)
				if err != nil {
					return RDSDBInstance{}, fmt.Errorf("create rds instance: describe secret: %w", err)
				}
			} else {
				return RDSDBInstance{}, fmt.Errorf("create rds instance: create secret: %w", err)
			}
		}
		secretARN = sec.ARN
	} else {
		if _, err := s.GetSecretValue(accountID, secretARN); err != nil {
			return RDSDBInstance{}, fmt.Errorf("%w: MasterUserSecretARN is missing or invalid", ErrRDSBadRequest)
		}
	}

	endpoint := strings.TrimSpace(in.EndpointAddress)
	if endpoint == "" {
		endpoint = fmt.Sprintf("noctaxris-data-rds-%s", id)
	}
	arn := RDSDBInstanceARN(region, accountID, id)
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO rds_db_instances
		 (account_id, db_instance_identifier, db_instance_arn, engine, engine_version, db_instance_class,
		  db_name, master_username, master_user_secret_arn, endpoint_address, endpoint_port,
		  db_instance_status, container_id, allocated_storage, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, id, arn, engine, version, class, dbName, user, secretARN, endpoint, port,
		status, strings.TrimSpace(in.ContainerID), storage, now,
	)
	if err != nil {
		return RDSDBInstance{}, fmt.Errorf("create rds instance: insert: %w", err)
	}
	return RDSDBInstance{
		DBInstanceIdentifier: id,
		DBInstanceARN:        arn,
		Engine:               engine,
		EngineVersion:        version,
		DBInstanceClass:      class,
		DBName:               dbName,
		MasterUsername:       user,
		MasterUserSecretARN:  secretARN,
		EndpointAddress:      endpoint,
		EndpointPort:         port,
		DBInstanceStatus:     status,
		ContainerID:          strings.TrimSpace(in.ContainerID),
		AllocatedStorage:     storage,
		CreatedAt:            now,
	}, nil
}

// UpdateRDSDBInstanceRuntime updates nested container fields after StartDataPlane.
func (s *Store) UpdateRDSDBInstanceRuntime(accountID, identifier, status, containerID, endpointAddress string, endpointPort int) error {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if identifier == "" {
		return fmt.Errorf("%w: DBInstanceIdentifier is required", ErrRDSBadRequest)
	}
	if endpointPort <= 0 {
		endpointPort = 5432
	}
	res, err := s.db.Exec(
		`UPDATE rds_db_instances SET db_instance_status = ?, container_id = ?, endpoint_address = ?, endpoint_port = ?
		 WHERE account_id = ? AND db_instance_identifier = ?`,
		status, strings.TrimSpace(containerID), strings.TrimSpace(endpointAddress), endpointPort, accountID, identifier,
	)
	if err != nil {
		return fmt.Errorf("update rds instance runtime: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrRDSInstanceNotFound
	}
	return nil
}

// DescribeRDSDBInstance returns an instance by identifier.
func (s *Store) DescribeRDSDBInstance(accountID, identifier string) (RDSDBInstance, error) {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	return s.scanRDSInstance(
		`SELECT db_instance_identifier, db_instance_arn, engine, engine_version, db_instance_class, db_name,
		        master_username, master_user_secret_arn, endpoint_address, endpoint_port, db_instance_status,
		        container_id, allocated_storage, created_at
		 FROM rds_db_instances WHERE account_id = ? AND db_instance_identifier = ?`,
		accountID, identifier,
	)
}

// DescribeRDSDBInstanceByARN returns an instance by ARN.
func (s *Store) DescribeRDSDBInstanceByARN(accountID, arn string) (RDSDBInstance, error) {
	arn = strings.TrimSpace(arn)
	return s.scanRDSInstance(
		`SELECT db_instance_identifier, db_instance_arn, engine, engine_version, db_instance_class, db_name,
		        master_username, master_user_secret_arn, endpoint_address, endpoint_port, db_instance_status,
		        container_id, allocated_storage, created_at
		 FROM rds_db_instances WHERE account_id = ? AND db_instance_arn = ?`,
		accountID, arn,
	)
}

func (s *Store) scanRDSInstance(query string, args ...any) (RDSDBInstance, error) {
	var inst RDSDBInstance
	err := s.db.QueryRow(query, args...).Scan(
		&inst.DBInstanceIdentifier, &inst.DBInstanceARN, &inst.Engine, &inst.EngineVersion,
		&inst.DBInstanceClass, &inst.DBName, &inst.MasterUsername, &inst.MasterUserSecretARN,
		&inst.EndpointAddress, &inst.EndpointPort, &inst.DBInstanceStatus, &inst.ContainerID,
		&inst.AllocatedStorage, &inst.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RDSDBInstance{}, ErrRDSInstanceNotFound
	}
	if err != nil {
		return RDSDBInstance{}, fmt.Errorf("describe rds instance: %w", err)
	}
	return inst, nil
}

// ListRDSDBInstances lists instances for an account.
func (s *Store) ListRDSDBInstances(accountID string) ([]RDSDBInstance, error) {
	rows, err := s.db.Query(
		`SELECT db_instance_identifier, db_instance_arn, engine, engine_version, db_instance_class, db_name,
		        master_username, master_user_secret_arn, endpoint_address, endpoint_port, db_instance_status,
		        container_id, allocated_storage, created_at
		 FROM rds_db_instances WHERE account_id = ? ORDER BY db_instance_identifier`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list rds instances: %w", err)
	}
	defer rows.Close()
	out := []RDSDBInstance{}
	for rows.Next() {
		var inst RDSDBInstance
		if err := rows.Scan(
			&inst.DBInstanceIdentifier, &inst.DBInstanceARN, &inst.Engine, &inst.EngineVersion,
			&inst.DBInstanceClass, &inst.DBName, &inst.MasterUsername, &inst.MasterUserSecretARN,
			&inst.EndpointAddress, &inst.EndpointPort, &inst.DBInstanceStatus, &inst.ContainerID,
			&inst.AllocatedStorage, &inst.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("list rds instances: scan: %w", err)
		}
		out = append(out, inst)
	}
	return out, rows.Err()
}

// DeleteRDSDBInstance deletes an instance by identifier and returns the prior row.
func (s *Store) DeleteRDSDBInstance(accountID, identifier string) (RDSDBInstance, error) {
	inst, err := s.DescribeRDSDBInstance(accountID, identifier)
	if err != nil {
		return RDSDBInstance{}, err
	}
	_, err = s.db.Exec(
		`DELETE FROM rds_db_instances WHERE account_id = ? AND db_instance_identifier = ?`,
		accountID, inst.DBInstanceIdentifier,
	)
	if err != nil {
		return RDSDBInstance{}, fmt.Errorf("delete rds instance: %w", err)
	}
	return inst, nil
}

// RDSMasterSecretName returns the Secrets Manager name used for RDS master creds.
func RDSMasterSecretName(dbInstanceIdentifier string) string {
	return "noctaxris/rds/" + strings.ToLower(strings.TrimSpace(dbInstanceIdentifier))
}

// RDSMasterSecretJSON builds the JSON shape Data API / engines expect.
func RDSMasterSecretJSON(username, password string) (string, error) {
	b, err := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		return "", fmt.Errorf("rds master secret json: %w", err)
	}
	return string(b), nil
}

// ParseRDSMasterSecret extracts username/password from a secret string.
func ParseRDSMasterSecret(secretString string) (username, password string, err error) {
	secretString = strings.TrimSpace(secretString)
	if secretString == "" {
		return "", "", fmt.Errorf("empty secret")
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(secretString), &obj); err != nil {
		return "", "", fmt.Errorf("secret is not JSON: %w", err)
	}
	user, _ := obj["username"].(string)
	pass, _ := obj["password"].(string)
	if user == "" || pass == "" {
		return "", "", fmt.Errorf("secret missing username or password")
	}
	return user, pass, nil
}

func randomLabPassword() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "lab-" + hex.EncodeToString(buf), nil
}
