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
	ErrEC2VPCNotFound     = errors.New("InvalidVpcID.NotFound")
	ErrEC2SubnetNotFound  = errors.New("InvalidSubnetID.NotFound")
	ErrEC2SGNotFound      = errors.New("InvalidGroup.NotFound")
	ErrEC2ENINotFound     = errors.New("InvalidNetworkInterfaceID.NotFound")
	ErrEC2NetworkConflict = errors.New("InvalidParameterValue")
)

const ec2NetworkSchema = `
CREATE TABLE IF NOT EXISTS ec2_vpcs (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  vpc_id TEXT NOT NULL,
  cidr_block TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'available',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, vpc_id)
);
CREATE TABLE IF NOT EXISTS ec2_subnets (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  subnet_id TEXT NOT NULL,
  vpc_id TEXT NOT NULL,
  cidr_block TEXT NOT NULL,
  availability_zone TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'available',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, subnet_id)
);
CREATE TABLE IF NOT EXISTS ec2_security_groups (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  group_id TEXT NOT NULL,
  group_name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  vpc_id TEXT NOT NULL DEFAULT '',
  ingress_json TEXT NOT NULL DEFAULT '[]',
  egress_json TEXT NOT NULL DEFAULT '[]',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, group_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ec2_sg_name_vpc
  ON ec2_security_groups(account_id, region, vpc_id, group_name);
CREATE TABLE IF NOT EXISTS ec2_network_interfaces (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  network_interface_id TEXT NOT NULL,
  subnet_id TEXT NOT NULL DEFAULT '',
  vpc_id TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  private_ip TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'available',
  attachment_instance_id TEXT NOT NULL DEFAULT '',
  availability_zone TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, network_interface_id)
);
`

// EC2Vpc is a control-plane VPC metadata row.
type EC2Vpc struct {
	VpcID     string
	CidrBlock string
	State     string
	CreatedAt int64
	Region    string
}

// EC2Subnet is a control-plane subnet metadata row.
type EC2Subnet struct {
	SubnetID         string
	VpcID            string
	CidrBlock        string
	AvailabilityZone string
	State            string
	CreatedAt        int64
	Region           string
}

// EC2IpRange is a CIDR entry on a security group rule.
type EC2IpRange struct {
	CidrIP string
}

// EC2IpPermission is an ingress or egress rule (persisted only; not enforced on Docker).
type EC2IpPermission struct {
	IpProtocol string
	FromPort   int
	ToPort     int
	IpRanges   []EC2IpRange
}

// EC2SecurityGroup is a control-plane security group with persisted rules.
type EC2SecurityGroup struct {
	GroupID             string
	GroupName           string
	Description         string
	VpcID               string
	OwnerID             string
	IpPermissions       []EC2IpPermission
	IpPermissionsEgress []EC2IpPermission
	CreatedAt           int64
	Region              string
}

// EC2NetworkInterface is ENI metadata (stored stub or synthetic from instance).
type EC2NetworkInterface struct {
	NetworkInterfaceID   string
	SubnetID             string
	VpcID                string
	Description          string
	PrivateIPAddress     string
	Status               string
	AttachmentInstanceID string
	AvailabilityZone     string
	Synthetic            bool
	CreatedAt            int64
	Region               string
}

// EnsureEC2NetworkSchema creates VPC/subnet/SG/ENI tables.
func EnsureEC2NetworkSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure ec2 network schema: db is nil")
	}
	if _, err := db.Exec(ec2NetworkSchema); err != nil {
		return fmt.Errorf("ensure ec2 network schema: %w", err)
	}
	return nil
}

func (s *Store) ensureEC2Network() error {
	if err := s.EnsureEC2Schema(); err != nil {
		return err
	}
	return EnsureEC2NetworkSchema(s.db)
}

func newEC2ResourceID(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")[:17]
}

func syntheticENIID(instanceID string) string {
	h := 0
	for _, c := range instanceID {
		h = (h*31 + int(c)) & 0xfffffff
	}
	return fmt.Sprintf("eni-%017x", h)
}

// CreateVpc stores a VPC with State=available.
func (s *Store) CreateVpc(accountID, region, cidrBlock string) (EC2Vpc, error) {
	if err := s.ensureEC2Network(); err != nil {
		return EC2Vpc{}, err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	cidrBlock = strings.TrimSpace(cidrBlock)
	if cidrBlock == "" {
		return EC2Vpc{}, fmt.Errorf("%w: CidrBlock is required", ErrEC2BadRequest)
	}
	id := newEC2ResourceID("vpc-")
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO ec2_vpcs (account_id, region, vpc_id, cidr_block, state, created_at)
		 VALUES (?, ?, ?, ?, 'available', ?)`,
		accountID, region, id, cidrBlock, now,
	)
	if err != nil {
		return EC2Vpc{}, fmt.Errorf("create vpc: %w", err)
	}
	return EC2Vpc{VpcID: id, CidrBlock: cidrBlock, State: "available", CreatedAt: now, Region: region}, nil
}

// DeleteVpc removes a VPC metadata row.
func (s *Store) DeleteVpc(accountID, region, vpcID string) error {
	if err := s.ensureEC2Network(); err != nil {
		return err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	vpcID = strings.TrimSpace(vpcID)
	if vpcID == "" {
		return fmt.Errorf("%w: VpcId is required", ErrEC2BadRequest)
	}
	res, err := s.db.Exec(
		`DELETE FROM ec2_vpcs WHERE account_id = ? AND region = ? AND vpc_id = ?`,
		accountID, region, vpcID,
	)
	if err != nil {
		return fmt.Errorf("delete vpc: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrEC2VPCNotFound
	}
	return nil
}

// DescribeVpcs returns VPCs filtered by optional VpcId values.
func (s *Store) DescribeVpcs(accountID, region string, vpcIDs []string) ([]EC2Vpc, error) {
	if err := s.ensureEC2Network(); err != nil {
		return nil, err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	rows, err := s.queryEC2IDs(
		`SELECT vpc_id, cidr_block, state, created_at, region FROM ec2_vpcs
		 WHERE account_id = ? AND region = ?`,
		accountID, region, vpcIDs, "vpc_id",
	)
	if err != nil {
		return nil, fmt.Errorf("describe vpcs: %w", err)
	}
	defer rows.Close()
	var out []EC2Vpc
	for rows.Next() {
		var v EC2Vpc
		if err := rows.Scan(&v.VpcID, &v.CidrBlock, &v.State, &v.CreatedAt, &v.Region); err != nil {
			return nil, fmt.Errorf("scan vpc: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CreateSubnet stores a subnet under an existing VPC.
func (s *Store) CreateSubnet(accountID, region, vpcID, cidrBlock, az string) (EC2Subnet, error) {
	if err := s.ensureEC2Network(); err != nil {
		return EC2Subnet{}, err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	vpcID = strings.TrimSpace(vpcID)
	cidrBlock = strings.TrimSpace(cidrBlock)
	if vpcID == "" || cidrBlock == "" {
		return EC2Subnet{}, fmt.Errorf("%w: VpcId and CidrBlock are required", ErrEC2BadRequest)
	}
	vpcs, err := s.DescribeVpcs(accountID, region, []string{vpcID})
	if err != nil {
		return EC2Subnet{}, err
	}
	if len(vpcs) == 0 {
		return EC2Subnet{}, ErrEC2VPCNotFound
	}
	az = strings.TrimSpace(az)
	if az == "" {
		az = region + "a"
	}
	id := newEC2ResourceID("subnet-")
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO ec2_subnets
		 (account_id, region, subnet_id, vpc_id, cidr_block, availability_zone, state, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'available', ?)`,
		accountID, region, id, vpcID, cidrBlock, az, now,
	)
	if err != nil {
		return EC2Subnet{}, fmt.Errorf("create subnet: %w", err)
	}
	return EC2Subnet{
		SubnetID: id, VpcID: vpcID, CidrBlock: cidrBlock, AvailabilityZone: az,
		State: "available", CreatedAt: now, Region: region,
	}, nil
}

// DeleteSubnet removes a subnet metadata row.
func (s *Store) DeleteSubnet(accountID, region, subnetID string) error {
	if err := s.ensureEC2Network(); err != nil {
		return err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	subnetID = strings.TrimSpace(subnetID)
	if subnetID == "" {
		return fmt.Errorf("%w: SubnetId is required", ErrEC2BadRequest)
	}
	res, err := s.db.Exec(
		`DELETE FROM ec2_subnets WHERE account_id = ? AND region = ? AND subnet_id = ?`,
		accountID, region, subnetID,
	)
	if err != nil {
		return fmt.Errorf("delete subnet: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrEC2SubnetNotFound
	}
	return nil
}

// DescribeSubnets returns subnets filtered by optional SubnetId values.
func (s *Store) DescribeSubnets(accountID, region string, subnetIDs []string) ([]EC2Subnet, error) {
	if err := s.ensureEC2Network(); err != nil {
		return nil, err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	rows, err := s.queryEC2IDs(
		`SELECT subnet_id, vpc_id, cidr_block, availability_zone, state, created_at, region
		 FROM ec2_subnets WHERE account_id = ? AND region = ?`,
		accountID, region, subnetIDs, "subnet_id",
	)
	if err != nil {
		return nil, fmt.Errorf("describe subnets: %w", err)
	}
	defer rows.Close()
	var out []EC2Subnet
	for rows.Next() {
		var sn EC2Subnet
		if err := rows.Scan(&sn.SubnetID, &sn.VpcID, &sn.CidrBlock, &sn.AvailabilityZone,
			&sn.State, &sn.CreatedAt, &sn.Region); err != nil {
			return nil, fmt.Errorf("scan subnet: %w", err)
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

// CreateSecurityGroup stores a security group; VPC SGs get default allow-all egress.
func (s *Store) CreateSecurityGroup(accountID, region, groupName, description, vpcID string) (EC2SecurityGroup, error) {
	if err := s.ensureEC2Network(); err != nil {
		return EC2SecurityGroup{}, err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	groupName = strings.TrimSpace(groupName)
	description = strings.TrimSpace(description)
	vpcID = strings.TrimSpace(vpcID)
	if groupName == "" || description == "" {
		return EC2SecurityGroup{}, fmt.Errorf("%w: GroupName and GroupDescription are required", ErrEC2BadRequest)
	}
	if vpcID != "" {
		vpcs, err := s.DescribeVpcs(accountID, region, []string{vpcID})
		if err != nil {
			return EC2SecurityGroup{}, err
		}
		if len(vpcs) == 0 {
			return EC2SecurityGroup{}, ErrEC2VPCNotFound
		}
	}
	id := newEC2ResourceID("sg-")
	now := time.Now().UTC().UnixMilli()
	egress := []EC2IpPermission{}
	if vpcID != "" {
		egress = []EC2IpPermission{{
			IpProtocol: "-1", FromPort: -1, ToPort: -1,
			IpRanges: []EC2IpRange{{CidrIP: "0.0.0.0/0"}},
		}}
	}
	egressJSON, _ := json.Marshal(egress)
	_, err := s.db.Exec(
		`INSERT INTO ec2_security_groups
		 (account_id, region, group_id, group_name, description, vpc_id, ingress_json, egress_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, '[]', ?, ?)`,
		accountID, region, id, groupName, description, vpcID, string(egressJSON), now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return EC2SecurityGroup{}, fmt.Errorf("%w: security group name already exists in VPC", ErrEC2NetworkConflict)
		}
		return EC2SecurityGroup{}, fmt.Errorf("create security group: %w", err)
	}
	return EC2SecurityGroup{
		GroupID: id, GroupName: groupName, Description: description, VpcID: vpcID,
		OwnerID: accountID, IpPermissions: nil, IpPermissionsEgress: egress,
		CreatedAt: now, Region: region,
	}, nil
}

// DeleteSecurityGroup removes a security group metadata row.
func (s *Store) DeleteSecurityGroup(accountID, region, groupID, groupName string) error {
	if err := s.ensureEC2Network(); err != nil {
		return err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	groupID = strings.TrimSpace(groupID)
	groupName = strings.TrimSpace(groupName)
	if groupID == "" && groupName == "" {
		return fmt.Errorf("%w: GroupId or GroupName is required", ErrEC2BadRequest)
	}
	var res sql.Result
	var err error
	if groupID != "" {
		res, err = s.db.Exec(
			`DELETE FROM ec2_security_groups WHERE account_id = ? AND region = ? AND group_id = ?`,
			accountID, region, groupID,
		)
	} else {
		res, err = s.db.Exec(
			`DELETE FROM ec2_security_groups WHERE account_id = ? AND region = ? AND group_name = ?`,
			accountID, region, groupName,
		)
	}
	if err != nil {
		return fmt.Errorf("delete security group: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrEC2SGNotFound
	}
	return nil
}

// DescribeSecurityGroups returns security groups filtered by optional GroupId values.
func (s *Store) DescribeSecurityGroups(accountID, region string, groupIDs, groupNames []string) ([]EC2SecurityGroup, error) {
	if err := s.ensureEC2Network(); err != nil {
		return nil, err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	q := `SELECT group_id, group_name, description, vpc_id, ingress_json, egress_json, created_at, region
	      FROM ec2_security_groups WHERE account_id = ? AND region = ?`
	args := []any{accountID, region}
	if len(groupIDs) > 0 {
		ph, a := idPlaceholders(groupIDs)
		if len(ph) > 0 {
			q += ` AND group_id IN (` + strings.Join(ph, ",") + `)`
			args = append(args, a...)
		}
	}
	if len(groupNames) > 0 {
		ph, a := idPlaceholders(groupNames)
		if len(ph) > 0 {
			q += ` AND group_name IN (` + strings.Join(ph, ",") + `)`
			args = append(args, a...)
		}
	}
	q += ` ORDER BY created_at`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("describe security groups: %w", err)
	}
	defer rows.Close()
	var out []EC2SecurityGroup
	for rows.Next() {
		var sg EC2SecurityGroup
		var ingressJSON, egressJSON string
		if err := rows.Scan(&sg.GroupID, &sg.GroupName, &sg.Description, &sg.VpcID,
			&ingressJSON, &egressJSON, &sg.CreatedAt, &sg.Region); err != nil {
			return nil, fmt.Errorf("scan security group: %w", err)
		}
		sg.OwnerID = accountID
		_ = json.Unmarshal([]byte(ingressJSON), &sg.IpPermissions)
		_ = json.Unmarshal([]byte(egressJSON), &sg.IpPermissionsEgress)
		if sg.IpPermissions == nil {
			sg.IpPermissions = []EC2IpPermission{}
		}
		if sg.IpPermissionsEgress == nil {
			sg.IpPermissionsEgress = []EC2IpPermission{}
		}
		out = append(out, sg)
	}
	return out, rows.Err()
}

// AuthorizeSecurityGroupIngress appends ingress rules (metadata only).
func (s *Store) AuthorizeSecurityGroupIngress(accountID, region, groupID string, rules []EC2IpPermission) error {
	return s.mutateSGRules(accountID, region, groupID, true, true, rules)
}

// AuthorizeSecurityGroupEgress appends egress rules (metadata only).
func (s *Store) AuthorizeSecurityGroupEgress(accountID, region, groupID string, rules []EC2IpPermission) error {
	return s.mutateSGRules(accountID, region, groupID, false, true, rules)
}

// RevokeSecurityGroupIngress removes matching ingress rules.
func (s *Store) RevokeSecurityGroupIngress(accountID, region, groupID string, rules []EC2IpPermission) error {
	return s.mutateSGRules(accountID, region, groupID, true, false, rules)
}

// RevokeSecurityGroupEgress removes matching egress rules.
func (s *Store) RevokeSecurityGroupEgress(accountID, region, groupID string, rules []EC2IpPermission) error {
	return s.mutateSGRules(accountID, region, groupID, false, false, rules)
}

func (s *Store) mutateSGRules(accountID, region, groupID string, ingress, authorize bool, rules []EC2IpPermission) error {
	if err := s.ensureEC2Network(); err != nil {
		return err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("%w: GroupId is required", ErrEC2BadRequest)
	}
	if len(rules) == 0 {
		return fmt.Errorf("%w: IpPermissions required", ErrEC2BadRequest)
	}
	list, err := s.DescribeSecurityGroups(accountID, region, []string{groupID}, nil)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return ErrEC2SGNotFound
	}
	sg := list[0]
	cur := sg.IpPermissions
	col := "ingress_json"
	if !ingress {
		cur = sg.IpPermissionsEgress
		col = "egress_json"
	}
	if authorize {
		cur = append(cur, rules...)
	} else {
		cur = revokePermissions(cur, rules)
	}
	raw, err := json.Marshal(cur)
	if err != nil {
		return fmt.Errorf("marshal sg rules: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE ec2_security_groups SET `+col+` = ? WHERE account_id = ? AND region = ? AND group_id = ?`,
		string(raw), accountID, region, groupID,
	)
	if err != nil {
		return fmt.Errorf("update sg rules: %w", err)
	}
	return nil
}

func revokePermissions(cur, revoke []EC2IpPermission) []EC2IpPermission {
	out := make([]EC2IpPermission, 0, len(cur))
	for _, p := range cur {
		drop := false
		for _, r := range revoke {
			if permissionEqual(p, r) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, p)
		}
	}
	return out
}

func permissionEqual(a, b EC2IpPermission) bool {
	if a.IpProtocol != b.IpProtocol || a.FromPort != b.FromPort || a.ToPort != b.ToPort {
		return false
	}
	if len(a.IpRanges) != len(b.IpRanges) {
		return false
	}
	for i := range a.IpRanges {
		if a.IpRanges[i].CidrIP != b.IpRanges[i].CidrIP {
			return false
		}
	}
	return true
}

// CreateNetworkInterface stores an available ENI stub (Terraform-shaped ID).
func (s *Store) CreateNetworkInterface(accountID, region, subnetID, description, privateIP string) (EC2NetworkInterface, error) {
	if err := s.ensureEC2Network(); err != nil {
		return EC2NetworkInterface{}, err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	subnetID = strings.TrimSpace(subnetID)
	if subnetID == "" {
		return EC2NetworkInterface{}, fmt.Errorf("%w: SubnetId is required", ErrEC2BadRequest)
	}
	subs, err := s.DescribeSubnets(accountID, region, []string{subnetID})
	if err != nil {
		return EC2NetworkInterface{}, err
	}
	if len(subs) == 0 {
		return EC2NetworkInterface{}, ErrEC2SubnetNotFound
	}
	sub := subs[0]
	id := newEC2ResourceID("eni-")
	now := time.Now().UTC().UnixMilli()
	priv := strings.TrimSpace(privateIP)
	if priv == "" {
		_, priv = ec2LabIPs(id)
	}
	_, err = s.db.Exec(
		`INSERT INTO ec2_network_interfaces
		 (account_id, region, network_interface_id, subnet_id, vpc_id, description,
		  private_ip, status, attachment_instance_id, availability_zone, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'available', '', ?, ?)`,
		accountID, region, id, subnetID, sub.VpcID, strings.TrimSpace(description),
		priv, sub.AvailabilityZone, now,
	)
	if err != nil {
		return EC2NetworkInterface{}, fmt.Errorf("create network interface: %w", err)
	}
	return EC2NetworkInterface{
		NetworkInterfaceID: id, SubnetID: subnetID, VpcID: sub.VpcID,
		Description: strings.TrimSpace(description), PrivateIPAddress: priv,
		Status: "available", AvailabilityZone: sub.AvailabilityZone,
		CreatedAt: now, Region: region,
	}, nil
}

// DescribeNetworkInterfaces returns stored ENIs plus synthetic ENIs for instances with private IPs.
func (s *Store) DescribeNetworkInterfaces(accountID, region string, eniIDs []string) ([]EC2NetworkInterface, error) {
	if err := s.ensureEC2Network(); err != nil {
		return nil, err
	}
	if region == "" {
		region = DefaultEC2Region
	}
	want := make(map[string]struct{})
	for _, id := range eniIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = struct{}{}
		}
	}
	rows, err := s.queryEC2IDs(
		`SELECT network_interface_id, subnet_id, vpc_id, description, private_ip, status,
		        attachment_instance_id, availability_zone, created_at, region
		 FROM ec2_network_interfaces WHERE account_id = ? AND region = ?`,
		accountID, region, eniIDs, "network_interface_id",
	)
	if err != nil {
		return nil, fmt.Errorf("describe network interfaces: %w", err)
	}
	defer rows.Close()
	var out []EC2NetworkInterface
	seen := make(map[string]struct{})
	for rows.Next() {
		var n EC2NetworkInterface
		if err := rows.Scan(&n.NetworkInterfaceID, &n.SubnetID, &n.VpcID, &n.Description,
			&n.PrivateIPAddress, &n.Status, &n.AttachmentInstanceID, &n.AvailabilityZone,
			&n.CreatedAt, &n.Region); err != nil {
			return nil, fmt.Errorf("scan eni: %w", err)
		}
		out = append(out, n)
		seen[n.NetworkInterfaceID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	instances, err := s.DescribeInstances(accountID, region, nil)
	if err != nil {
		return nil, err
	}
	for _, inst := range instances {
		if inst.StateName == EC2StateTerminated || strings.TrimSpace(inst.PrivateIP) == "" {
			continue
		}
		eniID := syntheticENIID(inst.InstanceID)
		if len(want) > 0 {
			if _, ok := want[eniID]; !ok {
				continue
			}
		}
		if _, ok := seen[eniID]; ok {
			continue
		}
		status := "in-use"
		if inst.StateName == EC2StatePending || inst.StateName == EC2StateStopped {
			status = "in-use"
		}
		out = append(out, EC2NetworkInterface{
			NetworkInterfaceID:   eniID,
			PrivateIPAddress:     inst.PrivateIP,
			Status:               status,
			AttachmentInstanceID: inst.InstanceID,
			AvailabilityZone:     inst.AvailabilityZone,
			Synthetic:            true,
			CreatedAt:            inst.CreatedAt,
			Region:               region,
		})
	}
	return out, nil
}

func idPlaceholders(ids []string) ([]string, []any) {
	ph := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		ph = append(ph, "?")
		args = append(args, id)
	}
	return ph, args
}

func (s *Store) queryEC2IDs(base string, accountID, region string, ids []string, col string) (*sql.Rows, error) {
	args := []any{accountID, region}
	q := base
	if len(ids) > 0 {
		ph, a := idPlaceholders(ids)
		if len(ph) == 0 {
			return s.db.Query(base+` AND 1=0`, accountID, region)
		}
		q += ` AND ` + col + ` IN (` + strings.Join(ph, ",") + `)`
		args = append(args, a...)
	}
	q += ` ORDER BY created_at`
	return s.db.Query(q, args...)
}
