package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrServiceDiscoveryNotFound   = errors.New("NamespaceNotFound")
	ErrServiceDiscoveryBadRequest = errors.New("InvalidInput")
	ErrServiceDiscoveryExists     = errors.New("NamespaceAlreadyExists")
)

const DefaultServiceDiscoveryRegion = "us-east-1"

const serviceDiscoverySchema = `
CREATE TABLE IF NOT EXISTS sd_namespaces (
  account_id TEXT NOT NULL,
  id TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  arn TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sd_ns_name ON sd_namespaces(account_id, name);
CREATE TABLE IF NOT EXISTS sd_services (
  account_id TEXT NOT NULL,
  id TEXT NOT NULL,
  namespace_id TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sd_svc_name ON sd_services(account_id, namespace_id, name);
CREATE TABLE IF NOT EXISTS sd_instances (
  account_id TEXT NOT NULL,
  service_id TEXT NOT NULL,
  instance_id TEXT NOT NULL,
  attributes_json TEXT NOT NULL DEFAULT '{}',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, service_id, instance_id)
);
`

// SDNamespace is a Cloud Map namespace.
type SDNamespace struct {
	ID          string
	Name        string
	Type        string // DNS_PRIVATE or HTTP
	ARN         string
	Description string
	CreatedAt   int64
}

// SDService is a Cloud Map service.
type SDService struct {
	ID          string
	NamespaceID string
	Name        string
	ARN         string
	Description string
	CreatedAt   int64
}

// SDInstance is a registered instance.
type SDInstance struct {
	InstanceID string
	ServiceID  string
	Attributes map[string]string
	CreatedAt  int64
}

// EnsureServiceDiscoverySchema creates Cloud Map tables if missing.
func EnsureServiceDiscoverySchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure servicediscovery schema: db is nil")
	}
	if _, err := db.Exec(serviceDiscoverySchema); err != nil {
		return fmt.Errorf("ensure servicediscovery schema: %w", err)
	}
	return nil
}

// EnsureServiceDiscoverySchema ensures Cloud Map tables on an open store.
func (s *Store) EnsureServiceDiscoverySchema() error {
	return EnsureServiceDiscoverySchema(s.db)
}

// CreateSDNamespace creates a private DNS or HTTP namespace.
func (s *Store) CreateSDNamespace(accountID, region, name, nsType, description string) (SDNamespace, error) {
	name = strings.TrimSpace(name)
	nsType = strings.ToUpper(strings.TrimSpace(nsType))
	if name == "" {
		return SDNamespace{}, fmt.Errorf("%w: Name required", ErrServiceDiscoveryBadRequest)
	}
	if nsType == "" {
		nsType = "DNS_PRIVATE"
	}
	if nsType != "DNS_PRIVATE" && nsType != "HTTP" {
		return SDNamespace{}, fmt.Errorf("%w: Type must be DNS_PRIVATE or HTTP", ErrServiceDiscoveryBadRequest)
	}
	if region == "" {
		region = DefaultServiceDiscoveryRegion
	}
	id := "ns-" + uuid.NewString()
	arn := fmt.Sprintf("arn:aws:servicediscovery:%s:%s:namespace/%s", region, accountID, id)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO sd_namespaces (account_id, id, name, type, arn, description, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, id, name, nsType, arn, description, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return SDNamespace{}, ErrServiceDiscoveryExists
		}
		return SDNamespace{}, fmt.Errorf("create namespace: %w", err)
	}
	return SDNamespace{ID: id, Name: name, Type: nsType, ARN: arn, Description: description, CreatedAt: now}, nil
}

// GetSDNamespace returns a namespace by ID.
func (s *Store) GetSDNamespace(accountID, id string) (SDNamespace, error) {
	var n SDNamespace
	err := s.db.QueryRow(
		`SELECT id, name, type, arn, description, created_at FROM sd_namespaces
		 WHERE account_id = ? AND id = ?`,
		accountID, id,
	).Scan(&n.ID, &n.Name, &n.Type, &n.ARN, &n.Description, &n.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SDNamespace{}, ErrServiceDiscoveryNotFound
	}
	if err != nil {
		return SDNamespace{}, fmt.Errorf("get namespace: %w", err)
	}
	return n, nil
}

// ListSDNamespaces lists namespaces.
func (s *Store) ListSDNamespaces(accountID string) ([]SDNamespace, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, arn, description, created_at FROM sd_namespaces
		 WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	defer rows.Close()
	var out []SDNamespace
	for rows.Next() {
		var n SDNamespace
		if err := rows.Scan(&n.ID, &n.Name, &n.Type, &n.ARN, &n.Description, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("list namespaces scan: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CreateSDService creates a service in a namespace.
func (s *Store) CreateSDService(accountID, region, namespaceID, name, description string) (SDService, error) {
	name = strings.TrimSpace(name)
	namespaceID = strings.TrimSpace(namespaceID)
	if name == "" || namespaceID == "" {
		return SDService{}, fmt.Errorf("%w: Name and NamespaceId required", ErrServiceDiscoveryBadRequest)
	}
	if _, err := s.GetSDNamespace(accountID, namespaceID); err != nil {
		return SDService{}, err
	}
	if region == "" {
		region = DefaultServiceDiscoveryRegion
	}
	id := "srv-" + uuid.NewString()
	arn := fmt.Sprintf("arn:aws:servicediscovery:%s:%s:service/%s", region, accountID, id)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO sd_services (account_id, id, namespace_id, name, arn, description, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, id, namespaceID, name, arn, description, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return SDService{}, fmt.Errorf("%w: service already exists", ErrServiceDiscoveryExists)
		}
		return SDService{}, fmt.Errorf("create service: %w", err)
	}
	return SDService{ID: id, NamespaceID: namespaceID, Name: name, ARN: arn, Description: description, CreatedAt: now}, nil
}

// GetSDService returns a service by ID.
func (s *Store) GetSDService(accountID, id string) (SDService, error) {
	var svc SDService
	err := s.db.QueryRow(
		`SELECT id, namespace_id, name, arn, description, created_at FROM sd_services
		 WHERE account_id = ? AND id = ?`,
		accountID, id,
	).Scan(&svc.ID, &svc.NamespaceID, &svc.Name, &svc.ARN, &svc.Description, &svc.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SDService{}, ErrServiceDiscoveryNotFound
	}
	if err != nil {
		return SDService{}, fmt.Errorf("get service: %w", err)
	}
	return svc, nil
}

// RegisterSDInstance registers an instance under a service.
func (s *Store) RegisterSDInstance(accountID, serviceID, instanceID string, attrs map[string]string) (SDInstance, error) {
	serviceID = strings.TrimSpace(serviceID)
	instanceID = strings.TrimSpace(instanceID)
	if serviceID == "" || instanceID == "" {
		return SDInstance{}, fmt.Errorf("%w: ServiceId and InstanceId required", ErrServiceDiscoveryBadRequest)
	}
	if _, err := s.GetSDService(accountID, serviceID); err != nil {
		return SDInstance{}, err
	}
	if attrs == nil {
		attrs = map[string]string{}
	}
	raw, _ := json.Marshal(attrs)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO sd_instances (account_id, service_id, instance_id, attributes_json, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, service_id, instance_id) DO UPDATE SET attributes_json = excluded.attributes_json`,
		accountID, serviceID, instanceID, string(raw), now,
	)
	if err != nil {
		return SDInstance{}, fmt.Errorf("register instance: %w", err)
	}
	return SDInstance{InstanceID: instanceID, ServiceID: serviceID, Attributes: attrs, CreatedAt: now}, nil
}

// DeregisterSDInstance removes an instance.
func (s *Store) DeregisterSDInstance(accountID, serviceID, instanceID string) error {
	res, err := s.db.Exec(
		`DELETE FROM sd_instances WHERE account_id = ? AND service_id = ? AND instance_id = ?`,
		accountID, serviceID, instanceID,
	)
	if err != nil {
		return fmt.Errorf("deregister instance: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrServiceDiscoveryNotFound
	}
	return nil
}

// DiscoverSDInstances finds instances by namespace name + service name.
func (s *Store) DiscoverSDInstances(accountID, namespaceName, serviceName string) ([]SDInstance, error) {
	namespaceName = strings.TrimSpace(namespaceName)
	serviceName = strings.TrimSpace(serviceName)
	if namespaceName == "" || serviceName == "" {
		return nil, fmt.Errorf("%w: NamespaceName and ServiceName required", ErrServiceDiscoveryBadRequest)
	}
	rows, err := s.db.Query(
		`SELECT i.instance_id, i.service_id, i.attributes_json, i.created_at
		 FROM sd_instances i
		 JOIN sd_services svc ON svc.account_id = i.account_id AND svc.id = i.service_id
		 JOIN sd_namespaces ns ON ns.account_id = svc.account_id AND ns.id = svc.namespace_id
		 WHERE i.account_id = ? AND ns.name = ? AND svc.name = ?
		 ORDER BY i.instance_id`,
		accountID, namespaceName, serviceName,
	)
	if err != nil {
		return nil, fmt.Errorf("discover instances: %w", err)
	}
	defer rows.Close()
	var out []SDInstance
	for rows.Next() {
		var inst SDInstance
		var raw string
		if err := rows.Scan(&inst.InstanceID, &inst.ServiceID, &raw, &inst.CreatedAt); err != nil {
			return nil, fmt.Errorf("discover instances scan: %w", err)
		}
		_ = json.Unmarshal([]byte(raw), &inst.Attributes)
		if inst.Attributes == nil {
			inst.Attributes = map[string]string{}
		}
		out = append(out, inst)
	}
	return out, rows.Err()
}
