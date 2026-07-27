package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrEC2NotFound   = errors.New("InvalidInstanceID.NotFound")
	ErrEC2BadRequest = errors.New("InvalidParameterValue")
)

const DefaultEC2Region = "us-east-1"

// EC2 instance state names and codes (AWS EC2 codes).
const (
	EC2StatePending      = "pending"
	EC2StateRunning      = "running"
	EC2StateStopping     = "stopping"
	EC2StateStopped      = "stopped"
	EC2StateShuttingDown = "shutting-down"
	EC2StateTerminated   = "terminated"

	EC2StateCodePending      = 0
	EC2StateCodeRunning      = 16
	EC2StateCodeShuttingDown = 32
	EC2StateCodeTerminated   = 48
	EC2StateCodeStopping     = 64
	EC2StateCodeStopped      = 80
)

const (
	// DefaultEC2DockerImage is used when ImageId is unknown or empty.
	DefaultEC2DockerImage = "public.ecr.aws/docker/library/alpine:3.20"
)

const ec2Schema = `
CREATE TABLE IF NOT EXISTS ec2_instances (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  instance_id TEXT NOT NULL,
  image_id TEXT NOT NULL,
  docker_image TEXT NOT NULL,
  instance_type TEXT NOT NULL DEFAULT 't3.micro',
  state_name TEXT NOT NULL,
  state_code INTEGER NOT NULL,
  container_id TEXT NOT NULL DEFAULT '',
  private_ip TEXT NOT NULL DEFAULT '',
  public_ip TEXT NOT NULL DEFAULT '',
  availability_zone TEXT NOT NULL DEFAULT 'us-east-1a',
  key_name TEXT NOT NULL DEFAULT '',
  user_data TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, instance_id)
);
`

// EC2Instance is a lab EC2 instance row (nested container-backed).
type EC2Instance struct {
	InstanceID       string
	ImageID          string
	DockerImage      string
	InstanceType     string
	StateName        string
	StateCode        int
	ContainerID      string
	PrivateIP        string
	PublicIP         string
	AvailabilityZone string
	KeyName          string
	UserData         string
	CreatedAt        int64
	Region           string
}

// RunInstancesInput holds RunInstances fields.
type RunInstancesInput struct {
	ImageID          string
	InstanceType     string
	MinCount         int
	MaxCount         int
	KeyName          string
	UserData         string
	AvailabilityZone string
}

// EnsureEC2Schema creates EC2 instance tables if missing.
func EnsureEC2Schema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure ec2 schema: db is nil")
	}
	if _, err := db.Exec(ec2Schema); err != nil {
		return fmt.Errorf("ensure ec2 schema: %w", err)
	}
	return nil
}

// EnsureEC2Schema ensures EC2 tables on an open store.
func (s *Store) EnsureEC2Schema() error {
	return EnsureEC2Schema(s.db)
}

// EC2AMI is a lab catalog image returned by DescribeImages.
type EC2AMI struct {
	ImageID            string
	Name               string
	Description        string
	Architecture       string
	State              string
	OwnerID            string
	ImageOwnerAlias    string
	Public             bool
	RootDeviceType     string
	RootDeviceName     string
	VirtualizationType string
	Hypervisor         string
	ImageType          string
	CreationDate       string
	DockerImage        string
	Aliases            []string
}

// EC2CatalogAMIs returns the lab AMI catalog (friendly IDs used by RunInstances).
func EC2CatalogAMIs() []EC2AMI {
	const owner = "000000000001"
	const created = "2024-01-01T00:00:00.000Z"
	base := func(id, name, desc, docker string, aliases ...string) EC2AMI {
		return EC2AMI{
			ImageID: id, Name: name, Description: desc, DockerImage: docker,
			Aliases: aliases, Architecture: "x86_64", State: "available",
			OwnerID: owner, ImageOwnerAlias: "amazon", Public: true,
			RootDeviceType: "ebs", RootDeviceName: "/dev/xvda",
			VirtualizationType: "hvm", Hypervisor: "xen", ImageType: "machine",
			CreationDate: created,
		}
	}
	return []EC2AMI{
		base("ami-alpine", "noctaxris/alpine-3.20", "Lab Alpine 3.20",
			DefaultEC2DockerImage, "ami-0abcdef123456789a"),
		base("ami-amazonlinux2023", "noctaxris/amazonlinux-2023", "Lab Amazon Linux 2023",
			"public.ecr.aws/amazonlinux/amazonlinux:2023", "ami-0abcdef1234567891"),
		base("ami-ubuntu2204", "noctaxris/ubuntu-22.04", "Lab Ubuntu 22.04",
			"public.ecr.aws/docker/library/ubuntu:22.04", "ami-0abcdef1234567892"),
	}
}

// DescribeEC2Images returns catalog AMIs, optionally filtered by ImageId values
// (matches primary ID or alias). Unknown IDs are omitted (empty set).
func DescribeEC2Images(imageIDs []string) []EC2AMI {
	all := EC2CatalogAMIs()
	if len(imageIDs) == 0 {
		return all
	}
	want := make(map[string]struct{}, len(imageIDs))
	for _, id := range imageIDs {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" {
			want[id] = struct{}{}
		}
	}
	if len(want) == 0 {
		return all
	}
	out := make([]EC2AMI, 0, len(want))
	for _, ami := range all {
		if _, ok := want[strings.ToLower(ami.ImageID)]; ok {
			out = append(out, ami)
			continue
		}
		for _, alias := range ami.Aliases {
			if _, ok := want[strings.ToLower(alias)]; ok {
				out = append(out, ami)
				break
			}
		}
	}
	return out
}

// ResolveEC2AMI maps a lab AMI ID (or alias) to an allowlisted Docker image.
// Unknown AMI IDs fall back to DefaultEC2DockerImage.
func ResolveEC2AMI(imageID string) (dockerImage string) {
	id := strings.ToLower(strings.TrimSpace(imageID))
	if id == "" {
		return DefaultEC2DockerImage
	}
	for _, ami := range EC2CatalogAMIs() {
		if id == strings.ToLower(ami.ImageID) {
			return ami.DockerImage
		}
		for _, alias := range ami.Aliases {
			if id == strings.ToLower(alias) {
				return ami.DockerImage
			}
		}
	}
	return DefaultEC2DockerImage
}

func newEC2InstanceID() string {
	return "i-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:17]
}

func ec2LabIPs(instanceID string) (publicIP, privateIP string) {
	h := 0
	for _, c := range instanceID {
		h = (h*31 + int(c)) & 0xffff
	}
	return fmt.Sprintf("203.0.113.%d", (h%200)+10), fmt.Sprintf("172.31.%d.%d", (h>>8)&0xff, h&0xff)
}

func ec2StateCode(name string) int {
	switch name {
	case EC2StatePending:
		return EC2StateCodePending
	case EC2StateRunning:
		return EC2StateCodeRunning
	case EC2StateShuttingDown:
		return EC2StateCodeShuttingDown
	case EC2StateTerminated:
		return EC2StateCodeTerminated
	case EC2StateStopping:
		return EC2StateCodeStopping
	case EC2StateStopped:
		return EC2StateCodeStopped
	default:
		return EC2StateCodePending
	}
}

// RunInstances creates MinCount..MaxCount pending instance rows.
func (s *Store) RunInstances(accountID, region string, in RunInstancesInput) ([]EC2Instance, error) {
	if err := s.EnsureEC2Schema(); err != nil {
		return nil, err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	minCount := in.MinCount
	maxCount := in.MaxCount
	if minCount <= 0 {
		minCount = 1
	}
	if maxCount <= 0 {
		maxCount = minCount
	}
	if maxCount < minCount {
		return nil, fmt.Errorf("%w: MaxCount must be >= MinCount", ErrEC2BadRequest)
	}
	if maxCount > 10 {
		return nil, fmt.Errorf("%w: MaxCount must be <= 10", ErrEC2BadRequest)
	}
	imageID := strings.TrimSpace(in.ImageID)
	if imageID == "" {
		imageID = "ami-alpine"
	}
	dockerImage := ResolveEC2AMI(imageID)
	instanceType := strings.TrimSpace(in.InstanceType)
	if instanceType == "" {
		instanceType = "t3.micro"
	}
	az := strings.TrimSpace(in.AvailabilityZone)
	if az == "" {
		az = region + "a"
	}
	now := time.Now().UTC().UnixMilli()
	out := make([]EC2Instance, 0, maxCount)
	for i := 0; i < maxCount; i++ {
		id := newEC2InstanceID()
		pub, priv := ec2LabIPs(id)
		_, err := s.db.Exec(
			`INSERT INTO ec2_instances
			 (account_id, region, instance_id, image_id, docker_image, instance_type,
			  state_name, state_code, container_id, private_ip, public_ip,
			  availability_zone, key_name, user_data, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)`,
			accountID, region, id, imageID, dockerImage, instanceType,
			EC2StatePending, EC2StateCodePending, priv, pub, az,
			strings.TrimSpace(in.KeyName), in.UserData, now,
		)
		if err != nil {
			return nil, fmt.Errorf("run instances: %w", err)
		}
		out = append(out, EC2Instance{
			InstanceID: id, ImageID: imageID, DockerImage: dockerImage,
			InstanceType: instanceType, StateName: EC2StatePending, StateCode: EC2StateCodePending,
			PrivateIP: priv, PublicIP: pub, AvailabilityZone: az,
			KeyName: strings.TrimSpace(in.KeyName), UserData: in.UserData,
			CreatedAt: now, Region: region,
		})
	}
	return out, nil
}

// DescribeInstances returns instances filtered by optional instance IDs.
func (s *Store) DescribeInstances(accountID, region string, instanceIDs []string) ([]EC2Instance, error) {
	if err := s.EnsureEC2Schema(); err != nil {
		return nil, err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	var (
		rows *sql.Rows
		err  error
	)
	if len(instanceIDs) == 0 {
		rows, err = s.db.Query(
			`SELECT instance_id, image_id, docker_image, instance_type, state_name, state_code,
			        container_id, private_ip, public_ip, availability_zone, key_name, user_data, created_at, region
			 FROM ec2_instances WHERE account_id = ? AND region = ? ORDER BY created_at`,
			accountID, region,
		)
	} else {
		args := []any{accountID, region}
		placeholders := make([]string, 0, len(instanceIDs))
		for _, id := range instanceIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			placeholders = append(placeholders, "?")
			args = append(args, id)
		}
		if len(placeholders) == 0 {
			return nil, nil
		}
		q := `SELECT instance_id, image_id, docker_image, instance_type, state_name, state_code,
		             container_id, private_ip, public_ip, availability_zone, key_name, user_data, created_at, region
		      FROM ec2_instances WHERE account_id = ? AND region = ? AND instance_id IN (` +
			strings.Join(placeholders, ",") + `) ORDER BY created_at`
		rows, err = s.db.Query(q, args...)
	}
	if err != nil {
		return nil, fmt.Errorf("describe instances: %w", err)
	}
	defer rows.Close()
	return scanEC2Instances(rows)
}

// GetEC2Instance returns one instance by ID.
func (s *Store) GetEC2Instance(accountID, region, instanceID string) (EC2Instance, error) {
	list, err := s.DescribeInstances(accountID, region, []string{instanceID})
	if err != nil {
		return EC2Instance{}, err
	}
	if len(list) == 0 {
		return EC2Instance{}, ErrEC2NotFound
	}
	return list[0], nil
}

// SetEC2InstanceState updates state name/code.
func (s *Store) SetEC2InstanceState(accountID, region, instanceID, stateName string) error {
	if err := s.EnsureEC2Schema(); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE ec2_instances SET state_name = ?, state_code = ?
		 WHERE account_id = ? AND region = ? AND instance_id = ?`,
		stateName, ec2StateCode(stateName), accountID, region, instanceID,
	)
	if err != nil {
		return fmt.Errorf("set ec2 state: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrEC2NotFound
	}
	return nil
}

// SetEC2ContainerID records the DinD container ID and optionally promotes state.
func (s *Store) SetEC2ContainerID(accountID, region, instanceID, containerID, stateName string) error {
	if err := s.EnsureEC2Schema(); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE ec2_instances SET container_id = ?, state_name = ?, state_code = ?
		 WHERE account_id = ? AND region = ? AND instance_id = ?`,
		containerID, stateName, ec2StateCode(stateName), accountID, region, instanceID,
	)
	if err != nil {
		return fmt.Errorf("set ec2 container id: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrEC2NotFound
	}
	return nil
}

// ClearEC2ContainerID clears the runtime ID (after terminate/rm).
func (s *Store) ClearEC2ContainerID(accountID, region, instanceID string) error {
	if err := s.EnsureEC2Schema(); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`UPDATE ec2_instances SET container_id = ''
		 WHERE account_id = ? AND region = ? AND instance_id = ?`,
		accountID, region, instanceID,
	)
	if err != nil {
		return fmt.Errorf("clear ec2 container id: %w", err)
	}
	return nil
}

func scanEC2Instances(rows *sql.Rows) ([]EC2Instance, error) {
	var out []EC2Instance
	for rows.Next() {
		var inst EC2Instance
		if err := rows.Scan(
			&inst.InstanceID, &inst.ImageID, &inst.DockerImage, &inst.InstanceType,
			&inst.StateName, &inst.StateCode, &inst.ContainerID, &inst.PrivateIP, &inst.PublicIP,
			&inst.AvailabilityZone, &inst.KeyName, &inst.UserData, &inst.CreatedAt, &inst.Region,
		); err != nil {
			return nil, fmt.Errorf("scan ec2 instance: %w", err)
		}
		out = append(out, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan ec2 instances: %w", err)
	}
	return out, nil
}
