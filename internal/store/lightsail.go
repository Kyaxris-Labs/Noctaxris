package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrLightsailNotFound   = errors.New("NotFoundException")
	ErrLightsailExists     = errors.New("InvalidResourceNameException")
	ErrLightsailBadRequest = errors.New("InvalidInputException")
)

const DefaultLightsailRegion = "us-east-1"

const lightsailSchema = `
CREATE TABLE IF NOT EXISTS lightsail_instances (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  blueprint_id TEXT NOT NULL,
  bundle_id TEXT NOT NULL,
  availability_zone TEXT NOT NULL,
  state_code INTEGER NOT NULL,
  state_name TEXT NOT NULL,
  public_ip TEXT NOT NULL DEFAULT '',
  private_ip TEXT NOT NULL DEFAULT '',
  username TEXT NOT NULL DEFAULT 'ubuntu',
  is_static_ip INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, name)
);
CREATE TABLE IF NOT EXISTS lightsail_disks (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  availability_zone TEXT NOT NULL,
  size_in_gb INTEGER NOT NULL,
  iops INTEGER NOT NULL,
  path TEXT NOT NULL DEFAULT '/dev/xvdf',
  state TEXT NOT NULL,
  is_attached INTEGER NOT NULL DEFAULT 0,
  attached_to TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, name)
);
CREATE TABLE IF NOT EXISTS lightsail_static_ips (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  ip_address TEXT NOT NULL,
  is_attached INTEGER NOT NULL DEFAULT 0,
  attached_to TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, name)
);
CREATE TABLE IF NOT EXISTS lightsail_key_pairs (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  public_key_base64 TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, name)
);
CREATE TABLE IF NOT EXISTS lightsail_instance_ports (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  instance_name TEXT NOT NULL,
  from_port INTEGER NOT NULL,
  to_port INTEGER NOT NULL,
  protocol TEXT NOT NULL,
  cidrs_json TEXT NOT NULL DEFAULT '["0.0.0.0/0"]',
  PRIMARY KEY (account_id, region, instance_name, from_port, to_port, protocol)
);
`

// LightsailInstance is a lab Lightsail instance row (state machine only).
type LightsailInstance struct {
	Name             string
	ARN              string
	BlueprintID      string
	BundleID         string
	AvailabilityZone string
	StateCode        int
	StateName        string
	PublicIP         string
	PrivateIP        string
	Username         string
	IsStaticIP       bool
	CreatedAt        int64
	Region           string
}

// LightsailDisk is a lab block storage disk (metadata only).
type LightsailDisk struct {
	Name             string
	ARN              string
	AvailabilityZone string
	SizeInGb         int
	Iops             int
	Path             string
	State            string
	IsAttached       bool
	AttachedTo       string
	CreatedAt        int64
	Region           string
}

// LightsailStaticIP is a lab static IP (metadata only).
type LightsailStaticIP struct {
	Name       string
	ARN        string
	IPAddress  string
	IsAttached bool
	AttachedTo string
	CreatedAt  int64
	Region     string
}

// LightsailKeyPair is a lab SSH key pair (dummy material).
type LightsailKeyPair struct {
	Name            string
	ARN             string
	Fingerprint     string
	PublicKeyBase64 string
	CreatedAt       int64
	Region          string
}

// LightsailPortState is an open public port on a lab instance.
type LightsailPortState struct {
	FromPort int
	ToPort   int
	Protocol string
	Cidrs    []string
	State    string
}

// LightsailBlueprint is a static lab blueprint.
type LightsailBlueprint struct {
	BlueprintID   string
	Name          string
	Group         string
	Type          string
	Version       string
	Platform      string
	IsActive      bool
}

// LightsailBundle is a static lab bundle.
type LightsailBundle struct {
	BundleID        string
	Name            string
	Price           float64
	CPUCount        int
	DiskSizeInGb    int
	MemoryInGb      float64
	TransferPerMonthInGb int
	IsActive        bool
}

// EnsureLightsailSchema creates Lightsail tables if missing.
func EnsureLightsailSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure lightsail schema: db is nil")
	}
	if _, err := db.Exec(lightsailSchema); err != nil {
		return fmt.Errorf("ensure lightsail schema: %w", err)
	}
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE lightsail_instances ADD COLUMN is_static_ip INTEGER NOT NULL DEFAULT 0`,
	}); err != nil {
		return fmt.Errorf("ensure lightsail schema: migrate: %w", err)
	}
	return nil
}

// EnsureLightsailSchema ensures Lightsail tables on an open store.
func (s *Store) EnsureLightsailSchema() error {
	return EnsureLightsailSchema(s.db)
}

// LightsailBlueprints returns the static lab blueprint catalog.
func LightsailBlueprints() []LightsailBlueprint {
	return []LightsailBlueprint{
		{BlueprintID: "ubuntu_22_04", Name: "Ubuntu", Group: "ubuntu_22_04", Type: "os", Version: "22.04", Platform: "LINUX_UNIX", IsActive: true},
		{BlueprintID: "amazon_linux_2023", Name: "Amazon Linux", Group: "amazon_linux_2023", Type: "os", Version: "2023", Platform: "LINUX_UNIX", IsActive: true},
		{BlueprintID: "debian_12", Name: "Debian", Group: "debian_12", Type: "os", Version: "12", Platform: "LINUX_UNIX", IsActive: true},
	}
}

// LightsailBundles returns the static lab bundle catalog.
func LightsailBundles() []LightsailBundle {
	return []LightsailBundle{
		{BundleID: "nano_3_0", Name: "Nano", Price: 3.50, CPUCount: 2, DiskSizeInGb: 20, MemoryInGb: 0.5, TransferPerMonthInGb: 1024, IsActive: true},
		{BundleID: "micro_3_0", Name: "Micro", Price: 5.00, CPUCount: 2, DiskSizeInGb: 40, MemoryInGb: 1, TransferPerMonthInGb: 2048, IsActive: true},
		{BundleID: "small_3_0", Name: "Small", Price: 10.00, CPUCount: 2, DiskSizeInGb: 60, MemoryInGb: 2, TransferPerMonthInGb: 3072, IsActive: true},
	}
}

func LightsailInstanceARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultLightsailRegion
	}
	return fmt.Sprintf("arn:aws:lightsail:%s:%s:Instance/%s", region, accountID, name)
}

func LightsailDiskARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultLightsailRegion
	}
	return fmt.Sprintf("arn:aws:lightsail:%s:%s:Disk/%s", region, accountID, name)
}

func LightsailStaticIPARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultLightsailRegion
	}
	return fmt.Sprintf("arn:aws:lightsail:%s:%s:StaticIp/%s", region, accountID, name)
}

func LightsailKeyPairARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultLightsailRegion
	}
	return fmt.Sprintf("arn:aws:lightsail:%s:%s:KeyPair/%s", region, accountID, name)
}

func lightsailUsername(blueprintID string) string {
	switch {
	case strings.Contains(blueprintID, "amazon"):
		return "ec2-user"
	case strings.Contains(blueprintID, "debian"):
		return "admin"
	default:
		return "ubuntu"
	}
}

func lightsailIPs(name string) (publicIP, privateIP string) {
	h := 0
	for _, c := range name {
		h = (h*31 + int(c)) & 0xffff
	}
	return fmt.Sprintf("203.0.113.%d", (h%200)+10), fmt.Sprintf("172.26.%d.%d", (h>>8)&0xff, h&0xff)
}

// CreateLightsailInstances creates one or more instances in running state.
func (s *Store) CreateLightsailInstances(accountID, region string, names []string, az, blueprintID, bundleID string) ([]LightsailInstance, error) {
	if region == "" {
		region = DefaultLightsailRegion
	}
	blueprintID = strings.TrimSpace(blueprintID)
	bundleID = strings.TrimSpace(bundleID)
	az = strings.TrimSpace(az)
	if len(names) == 0 {
		return nil, fmt.Errorf("%w: instanceNames required", ErrLightsailBadRequest)
	}
	if az == "" || blueprintID == "" || bundleID == "" {
		return nil, fmt.Errorf("%w: availabilityZone, blueprintId, and bundleId required", ErrLightsailBadRequest)
	}
	knownBP, knownBundle := false, false
	for _, b := range LightsailBlueprints() {
		if b.BlueprintID == blueprintID {
			knownBP = true
			break
		}
	}
	for _, b := range LightsailBundles() {
		if b.BundleID == bundleID {
			knownBundle = true
			break
		}
	}
	if !knownBP {
		return nil, fmt.Errorf("%w: unknown blueprintId", ErrLightsailBadRequest)
	}
	if !knownBundle {
		return nil, fmt.Errorf("%w: unknown bundleId", ErrLightsailBadRequest)
	}

	now := time.Now().UTC().UnixMilli()
	out := make([]LightsailInstance, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, fmt.Errorf("%w: instance name required", ErrLightsailBadRequest)
		}
		pub, priv := lightsailIPs(name)
		arn := LightsailInstanceARN(region, accountID, name)
		_, err := s.db.Exec(
			`INSERT INTO lightsail_instances
			 (account_id, region, name, arn, blueprint_id, bundle_id, availability_zone,
			  state_code, state_name, public_ip, private_ip, username, is_static_ip, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 16, 'running', ?, ?, ?, 0, ?)`,
			accountID, region, name, arn, blueprintID, bundleID, az, pub, priv, lightsailUsername(blueprintID), now,
		)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return nil, fmt.Errorf("%w: instance %s already exists", ErrLightsailExists, name)
			}
			return nil, fmt.Errorf("create lightsail instance: %w", err)
		}
		out = append(out, LightsailInstance{
			Name: name, ARN: arn, BlueprintID: blueprintID, BundleID: bundleID,
			AvailabilityZone: az, StateCode: 16, StateName: "running",
			PublicIP: pub, PrivateIP: priv, Username: lightsailUsername(blueprintID),
			IsStaticIP: false, CreatedAt: now, Region: region,
		})
	}
	return out, nil
}

// GetLightsailInstance returns one instance.
func (s *Store) GetLightsailInstance(accountID, region, name string) (LightsailInstance, error) {
	if region == "" {
		region = DefaultLightsailRegion
	}
	var inst LightsailInstance
	var isStatic int
	err := s.db.QueryRow(
		`SELECT name, arn, blueprint_id, bundle_id, availability_zone, state_code, state_name,
		        public_ip, private_ip, username, is_static_ip, created_at, region
		 FROM lightsail_instances WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, strings.TrimSpace(name),
	).Scan(&inst.Name, &inst.ARN, &inst.BlueprintID, &inst.BundleID, &inst.AvailabilityZone,
		&inst.StateCode, &inst.StateName, &inst.PublicIP, &inst.PrivateIP, &inst.Username,
		&isStatic, &inst.CreatedAt, &inst.Region)
	if errors.Is(err, sql.ErrNoRows) {
		return LightsailInstance{}, ErrLightsailNotFound
	}
	if err != nil {
		return LightsailInstance{}, fmt.Errorf("get lightsail instance: %w", err)
	}
	inst.IsStaticIP = isStatic != 0
	return inst, nil
}

// GetLightsailInstances lists instances for an account/region.
func (s *Store) GetLightsailInstances(accountID, region string) ([]LightsailInstance, error) {
	if region == "" {
		region = DefaultLightsailRegion
	}
	rows, err := s.db.Query(
		`SELECT name, arn, blueprint_id, bundle_id, availability_zone, state_code, state_name,
		        public_ip, private_ip, username, is_static_ip, created_at, region
		 FROM lightsail_instances WHERE account_id = ? AND region = ? ORDER BY created_at, name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("list lightsail instances: %w", err)
	}
	defer rows.Close()
	var out []LightsailInstance
	for rows.Next() {
		var inst LightsailInstance
		var isStatic int
		if err := rows.Scan(&inst.Name, &inst.ARN, &inst.BlueprintID, &inst.BundleID, &inst.AvailabilityZone,
			&inst.StateCode, &inst.StateName, &inst.PublicIP, &inst.PrivateIP, &inst.Username,
			&isStatic, &inst.CreatedAt, &inst.Region); err != nil {
			return nil, fmt.Errorf("list lightsail scan: %w", err)
		}
		inst.IsStaticIP = isStatic != 0
		out = append(out, inst)
	}
	return out, rows.Err()
}

func (s *Store) setLightsailState(accountID, region, name, stateName string, stateCode int) (LightsailInstance, error) {
	inst, err := s.GetLightsailInstance(accountID, region, name)
	if err != nil {
		return LightsailInstance{}, err
	}
	_, err = s.db.Exec(
		`UPDATE lightsail_instances SET state_code = ?, state_name = ? WHERE account_id = ? AND region = ? AND name = ?`,
		stateCode, stateName, accountID, region, strings.TrimSpace(name),
	)
	if err != nil {
		return LightsailInstance{}, fmt.Errorf("set lightsail state: %w", err)
	}
	inst.StateCode = stateCode
	inst.StateName = stateName
	return inst, nil
}

// StartLightsailInstance sets state to running.
func (s *Store) StartLightsailInstance(accountID, region, name string) (LightsailInstance, error) {
	return s.setLightsailState(accountID, region, name, "running", 16)
}

// StopLightsailInstance sets state to stopped.
func (s *Store) StopLightsailInstance(accountID, region, name string) (LightsailInstance, error) {
	return s.setLightsailState(accountID, region, name, "stopped", 80)
}

// RebootLightsailInstance keeps running (state machine only).
func (s *Store) RebootLightsailInstance(accountID, region, name string) (LightsailInstance, error) {
	return s.setLightsailState(accountID, region, name, "running", 16)
}

// DeleteLightsailInstance deletes an instance and detaches disks/static IPs/ports.
func (s *Store) DeleteLightsailInstance(accountID, region, name string) error {
	if region == "" {
		region = DefaultLightsailRegion
	}
	name = strings.TrimSpace(name)
	res, err := s.db.Exec(
		`DELETE FROM lightsail_instances WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, name,
	)
	if err != nil {
		return fmt.Errorf("delete lightsail instance: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete lightsail instance: %w", err)
	}
	if n == 0 {
		return ErrLightsailNotFound
	}
	_, _ = s.db.Exec(
		`UPDATE lightsail_disks SET is_attached = 0, attached_to = '', state = 'available'
		 WHERE account_id = ? AND region = ? AND attached_to = ?`,
		accountID, region, name,
	)
	_, _ = s.db.Exec(
		`UPDATE lightsail_static_ips SET is_attached = 0, attached_to = ''
		 WHERE account_id = ? AND region = ? AND attached_to = ?`,
		accountID, region, name,
	)
	_, _ = s.db.Exec(
		`DELETE FROM lightsail_instance_ports WHERE account_id = ? AND region = ? AND instance_name = ?`,
		accountID, region, name,
	)
	return nil
}
