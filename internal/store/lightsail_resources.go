package store

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func lightsailNormalizeRegion(region string) string {
	if region == "" {
		return DefaultLightsailRegion
	}
	return region
}

func lightsailRequireName(name, field string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: %s required", ErrLightsailBadRequest, field)
	}
	return name, nil
}

// CreateLightsailDisk creates an available block disk (metadata only).
func (s *Store) CreateLightsailDisk(accountID, region, name, az string, sizeInGb int) (LightsailDisk, error) {
	region = lightsailNormalizeRegion(region)
	name, err := lightsailRequireName(name, "diskName")
	if err != nil {
		return LightsailDisk{}, err
	}
	az = strings.TrimSpace(az)
	if az == "" {
		return LightsailDisk{}, fmt.Errorf("%w: availabilityZone required", ErrLightsailBadRequest)
	}
	if sizeInGb < 8 {
		return LightsailDisk{}, fmt.Errorf("%w: sizeInGb must be at least 8", ErrLightsailBadRequest)
	}
	now := time.Now().UTC().UnixMilli()
	iops := sizeInGb * 3
	if iops < 100 {
		iops = 100
	}
	arn := LightsailDiskARN(region, accountID, name)
	_, err = s.db.Exec(
		`INSERT INTO lightsail_disks
		 (account_id, region, name, arn, availability_zone, size_in_gb, iops, path, state, is_attached, attached_to, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, '/dev/xvdf', 'available', 0, '', ?)`,
		accountID, region, name, arn, az, sizeInGb, iops, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return LightsailDisk{}, fmt.Errorf("%w: disk %s already exists", ErrLightsailExists, name)
		}
		return LightsailDisk{}, fmt.Errorf("create lightsail disk: %w", err)
	}
	return LightsailDisk{
		Name: name, ARN: arn, AvailabilityZone: az, SizeInGb: sizeInGb, Iops: iops,
		Path: "/dev/xvdf", State: "available", IsAttached: false, CreatedAt: now, Region: region,
	}, nil
}

func (s *Store) scanLightsailDisk(scanner interface{ Scan(dest ...any) error }) (LightsailDisk, error) {
	var d LightsailDisk
	var attached int
	err := scanner.Scan(
		&d.Name, &d.ARN, &d.AvailabilityZone, &d.SizeInGb, &d.Iops, &d.Path, &d.State,
		&attached, &d.AttachedTo, &d.CreatedAt, &d.Region,
	)
	if err != nil {
		return LightsailDisk{}, err
	}
	d.IsAttached = attached != 0
	return d, nil
}

// GetLightsailDisk returns one disk.
func (s *Store) GetLightsailDisk(accountID, region, name string) (LightsailDisk, error) {
	region = lightsailNormalizeRegion(region)
	row := s.db.QueryRow(
		`SELECT name, arn, availability_zone, size_in_gb, iops, path, state, is_attached, attached_to, created_at, region
		 FROM lightsail_disks WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, strings.TrimSpace(name),
	)
	d, err := s.scanLightsailDisk(row)
	if errors.Is(err, sql.ErrNoRows) {
		return LightsailDisk{}, ErrLightsailNotFound
	}
	if err != nil {
		return LightsailDisk{}, fmt.Errorf("get lightsail disk: %w", err)
	}
	return d, nil
}

// GetLightsailDisks lists disks for an account/region.
func (s *Store) GetLightsailDisks(accountID, region string) ([]LightsailDisk, error) {
	region = lightsailNormalizeRegion(region)
	rows, err := s.db.Query(
		`SELECT name, arn, availability_zone, size_in_gb, iops, path, state, is_attached, attached_to, created_at, region
		 FROM lightsail_disks WHERE account_id = ? AND region = ? ORDER BY created_at, name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("list lightsail disks: %w", err)
	}
	defer rows.Close()
	var out []LightsailDisk
	for rows.Next() {
		d, err := s.scanLightsailDisk(rows)
		if err != nil {
			return nil, fmt.Errorf("list lightsail disks scan: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AttachLightsailDisk attaches a disk to an instance.
func (s *Store) AttachLightsailDisk(accountID, region, diskName, instanceName, diskPath string) (LightsailDisk, error) {
	region = lightsailNormalizeRegion(region)
	diskName, err := lightsailRequireName(diskName, "diskName")
	if err != nil {
		return LightsailDisk{}, err
	}
	instanceName, err = lightsailRequireName(instanceName, "instanceName")
	if err != nil {
		return LightsailDisk{}, err
	}
	disk, err := s.GetLightsailDisk(accountID, region, diskName)
	if err != nil {
		return LightsailDisk{}, err
	}
	if disk.IsAttached {
		return LightsailDisk{}, fmt.Errorf("%w: disk %s is already attached to instance %s", ErrLightsailBadRequest, diskName, disk.AttachedTo)
	}
	if _, err := s.GetLightsailInstance(accountID, region, instanceName); err != nil {
		return LightsailDisk{}, err
	}
	diskPath = strings.TrimSpace(diskPath)
	if diskPath == "" {
		diskPath = "/dev/xvdf"
	}
	_, err = s.db.Exec(
		`UPDATE lightsail_disks SET is_attached = 1, attached_to = ?, state = 'in-use', path = ?
		 WHERE account_id = ? AND region = ? AND name = ?`,
		instanceName, diskPath, accountID, region, diskName,
	)
	if err != nil {
		return LightsailDisk{}, fmt.Errorf("attach lightsail disk: %w", err)
	}
	disk.IsAttached = true
	disk.AttachedTo = instanceName
	disk.State = "in-use"
	disk.Path = diskPath
	return disk, nil
}

// DetachLightsailDisk detaches a disk from its instance.
func (s *Store) DetachLightsailDisk(accountID, region, diskName string) (LightsailDisk, error) {
	region = lightsailNormalizeRegion(region)
	diskName, err := lightsailRequireName(diskName, "diskName")
	if err != nil {
		return LightsailDisk{}, err
	}
	disk, err := s.GetLightsailDisk(accountID, region, diskName)
	if err != nil {
		return LightsailDisk{}, err
	}
	if !disk.IsAttached {
		return LightsailDisk{}, fmt.Errorf("%w: disk %s is not attached to any instance", ErrLightsailBadRequest, diskName)
	}
	_, err = s.db.Exec(
		`UPDATE lightsail_disks SET is_attached = 0, attached_to = '', state = 'available'
		 WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, diskName,
	)
	if err != nil {
		return LightsailDisk{}, fmt.Errorf("detach lightsail disk: %w", err)
	}
	disk.IsAttached = false
	disk.AttachedTo = ""
	disk.State = "available"
	return disk, nil
}

// DeleteLightsailDisk deletes an available (detached) disk.
func (s *Store) DeleteLightsailDisk(accountID, region, diskName string) (LightsailDisk, error) {
	region = lightsailNormalizeRegion(region)
	diskName, err := lightsailRequireName(diskName, "diskName")
	if err != nil {
		return LightsailDisk{}, err
	}
	disk, err := s.GetLightsailDisk(accountID, region, diskName)
	if err != nil {
		return LightsailDisk{}, err
	}
	if disk.IsAttached {
		return LightsailDisk{}, fmt.Errorf("%w: disk %s is attached to instance %s and cannot be deleted", ErrLightsailBadRequest, diskName, disk.AttachedTo)
	}
	res, err := s.db.Exec(
		`DELETE FROM lightsail_disks WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, diskName,
	)
	if err != nil {
		return LightsailDisk{}, fmt.Errorf("delete lightsail disk: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return LightsailDisk{}, fmt.Errorf("delete lightsail disk: %w", err)
	}
	if n == 0 {
		return LightsailDisk{}, ErrLightsailNotFound
	}
	return disk, nil
}

// AllocateLightsailStaticIP allocates a static IP name (lab address).
func (s *Store) AllocateLightsailStaticIP(accountID, region, name string) (LightsailStaticIP, error) {
	region = lightsailNormalizeRegion(region)
	name, err := lightsailRequireName(name, "staticIpName")
	if err != nil {
		return LightsailStaticIP{}, err
	}
	now := time.Now().UTC().UnixMilli()
	ipAddr, _ := lightsailIPs(name)
	arn := LightsailStaticIPARN(region, accountID, name)
	_, err = s.db.Exec(
		`INSERT INTO lightsail_static_ips
		 (account_id, region, name, arn, ip_address, is_attached, attached_to, created_at)
		 VALUES (?, ?, ?, ?, ?, 0, '', ?)`,
		accountID, region, name, arn, ipAddr, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return LightsailStaticIP{}, fmt.Errorf("%w: static IP %s already exists", ErrLightsailExists, name)
		}
		return LightsailStaticIP{}, fmt.Errorf("allocate lightsail static ip: %w", err)
	}
	return LightsailStaticIP{
		Name: name, ARN: arn, IPAddress: ipAddr, IsAttached: false, CreatedAt: now, Region: region,
	}, nil
}

func (s *Store) scanLightsailStaticIP(scanner interface{ Scan(dest ...any) error }) (LightsailStaticIP, error) {
	var ip LightsailStaticIP
	var attached int
	err := scanner.Scan(&ip.Name, &ip.ARN, &ip.IPAddress, &attached, &ip.AttachedTo, &ip.CreatedAt, &ip.Region)
	if err != nil {
		return LightsailStaticIP{}, err
	}
	ip.IsAttached = attached != 0
	return ip, nil
}

// GetLightsailStaticIP returns one static IP.
func (s *Store) GetLightsailStaticIP(accountID, region, name string) (LightsailStaticIP, error) {
	region = lightsailNormalizeRegion(region)
	row := s.db.QueryRow(
		`SELECT name, arn, ip_address, is_attached, attached_to, created_at, region
		 FROM lightsail_static_ips WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, strings.TrimSpace(name),
	)
	ip, err := s.scanLightsailStaticIP(row)
	if errors.Is(err, sql.ErrNoRows) {
		return LightsailStaticIP{}, ErrLightsailNotFound
	}
	if err != nil {
		return LightsailStaticIP{}, fmt.Errorf("get lightsail static ip: %w", err)
	}
	return ip, nil
}

// GetLightsailStaticIPs lists static IPs.
func (s *Store) GetLightsailStaticIPs(accountID, region string) ([]LightsailStaticIP, error) {
	region = lightsailNormalizeRegion(region)
	rows, err := s.db.Query(
		`SELECT name, arn, ip_address, is_attached, attached_to, created_at, region
		 FROM lightsail_static_ips WHERE account_id = ? AND region = ? ORDER BY created_at, name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("list lightsail static ips: %w", err)
	}
	defer rows.Close()
	var out []LightsailStaticIP
	for rows.Next() {
		ip, err := s.scanLightsailStaticIP(rows)
		if err != nil {
			return nil, fmt.Errorf("list lightsail static ips scan: %w", err)
		}
		out = append(out, ip)
	}
	return out, rows.Err()
}

// AttachLightsailStaticIP attaches a static IP to an instance and updates the instance public IP.
func (s *Store) AttachLightsailStaticIP(accountID, region, staticIPName, instanceName string) (LightsailStaticIP, error) {
	region = lightsailNormalizeRegion(region)
	staticIPName, err := lightsailRequireName(staticIPName, "staticIpName")
	if err != nil {
		return LightsailStaticIP{}, err
	}
	instanceName, err = lightsailRequireName(instanceName, "instanceName")
	if err != nil {
		return LightsailStaticIP{}, err
	}
	ip, err := s.GetLightsailStaticIP(accountID, region, staticIPName)
	if err != nil {
		return LightsailStaticIP{}, err
	}
	if ip.IsAttached {
		return LightsailStaticIP{}, fmt.Errorf("%w: static IP %s is already attached to instance %s", ErrLightsailBadRequest, staticIPName, ip.AttachedTo)
	}
	if _, err := s.GetLightsailInstance(accountID, region, instanceName); err != nil {
		return LightsailStaticIP{}, err
	}
	_, err = s.db.Exec(
		`UPDATE lightsail_static_ips SET is_attached = 1, attached_to = ?
		 WHERE account_id = ? AND region = ? AND name = ?`,
		instanceName, accountID, region, staticIPName,
	)
	if err != nil {
		return LightsailStaticIP{}, fmt.Errorf("attach lightsail static ip: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE lightsail_instances SET public_ip = ?, is_static_ip = 1
		 WHERE account_id = ? AND region = ? AND name = ?`,
		ip.IPAddress, accountID, region, instanceName,
	)
	if err != nil {
		return LightsailStaticIP{}, fmt.Errorf("attach lightsail static ip instance: %w", err)
	}
	ip.IsAttached = true
	ip.AttachedTo = instanceName
	return ip, nil
}

func (s *Store) clearInstanceStaticIP(accountID, region, instanceName string) error {
	if instanceName == "" {
		return nil
	}
	pub, _ := lightsailIPs(instanceName)
	_, err := s.db.Exec(
		`UPDATE lightsail_instances SET public_ip = ?, is_static_ip = 0
		 WHERE account_id = ? AND region = ? AND name = ?`,
		pub, accountID, region, instanceName,
	)
	if err != nil {
		return fmt.Errorf("clear lightsail instance static ip: %w", err)
	}
	return nil
}

// DetachLightsailStaticIP detaches a static IP from its instance.
func (s *Store) DetachLightsailStaticIP(accountID, region, staticIPName string) (LightsailStaticIP, error) {
	region = lightsailNormalizeRegion(region)
	staticIPName, err := lightsailRequireName(staticIPName, "staticIpName")
	if err != nil {
		return LightsailStaticIP{}, err
	}
	ip, err := s.GetLightsailStaticIP(accountID, region, staticIPName)
	if err != nil {
		return LightsailStaticIP{}, err
	}
	if ip.IsAttached {
		if err := s.clearInstanceStaticIP(accountID, region, ip.AttachedTo); err != nil {
			return LightsailStaticIP{}, err
		}
	}
	_, err = s.db.Exec(
		`UPDATE lightsail_static_ips SET is_attached = 0, attached_to = ''
		 WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, staticIPName,
	)
	if err != nil {
		return LightsailStaticIP{}, fmt.Errorf("detach lightsail static ip: %w", err)
	}
	ip.IsAttached = false
	ip.AttachedTo = ""
	return ip, nil
}

// ReleaseLightsailStaticIP releases a static IP (detaches first if needed).
func (s *Store) ReleaseLightsailStaticIP(accountID, region, staticIPName string) (LightsailStaticIP, error) {
	region = lightsailNormalizeRegion(region)
	staticIPName, err := lightsailRequireName(staticIPName, "staticIpName")
	if err != nil {
		return LightsailStaticIP{}, err
	}
	ip, err := s.GetLightsailStaticIP(accountID, region, staticIPName)
	if err != nil {
		return LightsailStaticIP{}, err
	}
	if ip.IsAttached {
		if err := s.clearInstanceStaticIP(accountID, region, ip.AttachedTo); err != nil {
			return LightsailStaticIP{}, err
		}
	}
	res, err := s.db.Exec(
		`DELETE FROM lightsail_static_ips WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, staticIPName,
	)
	if err != nil {
		return LightsailStaticIP{}, fmt.Errorf("release lightsail static ip: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return LightsailStaticIP{}, fmt.Errorf("release lightsail static ip: %w", err)
	}
	if n == 0 {
		return LightsailStaticIP{}, ErrLightsailNotFound
	}
	ip.IsAttached = false
	ip.AttachedTo = ""
	return ip, nil
}

func lightsailPublicKeyBase64(name string) string {
	return base64.StdEncoding.EncodeToString([]byte("ssh-rsa NOCTAXRIS-" + name))
}

func LightsailPrivateKeyBase64(name string) string {
	return base64.StdEncoding.EncodeToString([]byte(
		"-----BEGIN PRIVATE KEY-----\nnoctaxris-" + name + "\n-----END PRIVATE KEY-----\n",
	))
}

func lightsailKeyFingerprint(name string) string {
	return base64.StdEncoding.EncodeToString([]byte("fingerprint:" + name))
}

// CreateLightsailKeyPair creates a lab key pair and returns private key material once.
func (s *Store) CreateLightsailKeyPair(accountID, region, name string) (LightsailKeyPair, string, error) {
	region = lightsailNormalizeRegion(region)
	name, err := lightsailRequireName(name, "keyPairName")
	if err != nil {
		return LightsailKeyPair{}, "", err
	}
	now := time.Now().UTC().UnixMilli()
	arn := LightsailKeyPairARN(region, accountID, name)
	pub := lightsailPublicKeyBase64(name)
	fp := lightsailKeyFingerprint(name)
	_, err = s.db.Exec(
		`INSERT INTO lightsail_key_pairs
		 (account_id, region, name, arn, fingerprint, public_key_base64, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, region, name, arn, fp, pub, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return LightsailKeyPair{}, "", fmt.Errorf("%w: key pair %s already exists", ErrLightsailExists, name)
		}
		return LightsailKeyPair{}, "", fmt.Errorf("create lightsail key pair: %w", err)
	}
	return LightsailKeyPair{
		Name: name, ARN: arn, Fingerprint: fp, PublicKeyBase64: pub, CreatedAt: now, Region: region,
	}, LightsailPrivateKeyBase64(name), nil
}

// GetLightsailKeyPair returns one key pair (without private key).
func (s *Store) GetLightsailKeyPair(accountID, region, name string) (LightsailKeyPair, error) {
	region = lightsailNormalizeRegion(region)
	var kp LightsailKeyPair
	err := s.db.QueryRow(
		`SELECT name, arn, fingerprint, public_key_base64, created_at, region
		 FROM lightsail_key_pairs WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, strings.TrimSpace(name),
	).Scan(&kp.Name, &kp.ARN, &kp.Fingerprint, &kp.PublicKeyBase64, &kp.CreatedAt, &kp.Region)
	if errors.Is(err, sql.ErrNoRows) {
		return LightsailKeyPair{}, ErrLightsailNotFound
	}
	if err != nil {
		return LightsailKeyPair{}, fmt.Errorf("get lightsail key pair: %w", err)
	}
	return kp, nil
}

// GetLightsailKeyPairs lists key pairs.
func (s *Store) GetLightsailKeyPairs(accountID, region string) ([]LightsailKeyPair, error) {
	region = lightsailNormalizeRegion(region)
	rows, err := s.db.Query(
		`SELECT name, arn, fingerprint, public_key_base64, created_at, region
		 FROM lightsail_key_pairs WHERE account_id = ? AND region = ? ORDER BY created_at, name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("list lightsail key pairs: %w", err)
	}
	defer rows.Close()
	var out []LightsailKeyPair
	for rows.Next() {
		var kp LightsailKeyPair
		if err := rows.Scan(&kp.Name, &kp.ARN, &kp.Fingerprint, &kp.PublicKeyBase64, &kp.CreatedAt, &kp.Region); err != nil {
			return nil, fmt.Errorf("list lightsail key pairs scan: %w", err)
		}
		out = append(out, kp)
	}
	return out, rows.Err()
}

// DeleteLightsailKeyPair deletes a key pair.
func (s *Store) DeleteLightsailKeyPair(accountID, region, name string) (LightsailKeyPair, error) {
	region = lightsailNormalizeRegion(region)
	name, err := lightsailRequireName(name, "keyPairName")
	if err != nil {
		return LightsailKeyPair{}, err
	}
	kp, err := s.GetLightsailKeyPair(accountID, region, name)
	if err != nil {
		return LightsailKeyPair{}, err
	}
	res, err := s.db.Exec(
		`DELETE FROM lightsail_key_pairs WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, name,
	)
	if err != nil {
		return LightsailKeyPair{}, fmt.Errorf("delete lightsail key pair: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return LightsailKeyPair{}, fmt.Errorf("delete lightsail key pair: %w", err)
	}
	if n == 0 {
		return LightsailKeyPair{}, ErrLightsailNotFound
	}
	return kp, nil
}

func lightsailParseCidrs(raw any) []string {
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		if len(out) == 0 {
			return []string{"0.0.0.0/0"}
		}
		return out
	case []string:
		if len(v) == 0 {
			return []string{"0.0.0.0/0"}
		}
		return v
	default:
		return []string{"0.0.0.0/0"}
	}
}

func lightsailPortFromMap(m map[string]any) (LightsailPortState, error) {
	from, okFrom := lightsailAsInt(m["fromPort"])
	if !okFrom {
		from, okFrom = lightsailAsInt(m["FromPort"])
	}
	to, okTo := lightsailAsInt(m["toPort"])
	if !okTo {
		to, okTo = lightsailAsInt(m["ToPort"])
	}
	proto, _ := m["protocol"].(string)
	if proto == "" {
		proto, _ = m["Protocol"].(string)
	}
	proto = strings.ToLower(strings.TrimSpace(proto))
	if !okFrom || !okTo || proto == "" {
		return LightsailPortState{}, fmt.Errorf("%w: portInfo requires fromPort, toPort, and protocol", ErrLightsailBadRequest)
	}
	if from < -1 || from > 65535 || to < -1 || to > 65535 {
		return LightsailPortState{}, fmt.Errorf("%w: port range out of bounds", ErrLightsailBadRequest)
	}
	switch proto {
	case "tcp", "udp", "all", "icmp", "icmpv6":
	default:
		return LightsailPortState{}, fmt.Errorf("%w: unsupported protocol %s", ErrLightsailBadRequest, proto)
	}
	cidrs := lightsailParseCidrs(m["cidrs"])
	if raw, ok := m["Cidrs"]; ok {
		cidrs = lightsailParseCidrs(raw)
	}
	return LightsailPortState{FromPort: from, ToPort: to, Protocol: proto, Cidrs: cidrs, State: "open"}, nil
}

func lightsailAsInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

// OpenLightsailInstancePublicPorts merges an open port onto an instance.
func (s *Store) OpenLightsailInstancePublicPorts(accountID, region, instanceName string, portInfo map[string]any) (LightsailInstance, LightsailPortState, error) {
	region = lightsailNormalizeRegion(region)
	instanceName, err := lightsailRequireName(instanceName, "instanceName")
	if err != nil {
		return LightsailInstance{}, LightsailPortState{}, err
	}
	if portInfo == nil {
		return LightsailInstance{}, LightsailPortState{}, fmt.Errorf("%w: portInfo required", ErrLightsailBadRequest)
	}
	inst, err := s.GetLightsailInstance(accountID, region, instanceName)
	if err != nil {
		return LightsailInstance{}, LightsailPortState{}, err
	}
	port, err := lightsailPortFromMap(portInfo)
	if err != nil {
		return LightsailInstance{}, LightsailPortState{}, err
	}
	cidrsJSON, err := json.Marshal(port.Cidrs)
	if err != nil {
		return LightsailInstance{}, LightsailPortState{}, fmt.Errorf("marshal cidrs: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO lightsail_instance_ports
		 (account_id, region, instance_name, from_port, to_port, protocol, cidrs_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, region, instance_name, from_port, to_port, protocol)
		 DO UPDATE SET cidrs_json = excluded.cidrs_json`,
		accountID, region, instanceName, port.FromPort, port.ToPort, port.Protocol, string(cidrsJSON),
	)
	if err != nil {
		return LightsailInstance{}, LightsailPortState{}, fmt.Errorf("open lightsail ports: %w", err)
	}
	return inst, port, nil
}

// CloseLightsailInstancePublicPorts removes a matching open port from an instance.
func (s *Store) CloseLightsailInstancePublicPorts(accountID, region, instanceName string, portInfo map[string]any) (LightsailInstance, error) {
	region = lightsailNormalizeRegion(region)
	instanceName, err := lightsailRequireName(instanceName, "instanceName")
	if err != nil {
		return LightsailInstance{}, err
	}
	if portInfo == nil {
		return LightsailInstance{}, fmt.Errorf("%w: portInfo required", ErrLightsailBadRequest)
	}
	inst, err := s.GetLightsailInstance(accountID, region, instanceName)
	if err != nil {
		return LightsailInstance{}, err
	}
	port, err := lightsailPortFromMap(portInfo)
	if err != nil {
		return LightsailInstance{}, err
	}
	_, err = s.db.Exec(
		`DELETE FROM lightsail_instance_ports
		 WHERE account_id = ? AND region = ? AND instance_name = ? AND from_port = ? AND to_port = ? AND protocol = ?`,
		accountID, region, instanceName, port.FromPort, port.ToPort, port.Protocol,
	)
	if err != nil {
		return LightsailInstance{}, fmt.Errorf("close lightsail ports: %w", err)
	}
	return inst, nil
}

// GetLightsailInstancePortStates lists open ports for an instance.
func (s *Store) GetLightsailInstancePortStates(accountID, region, instanceName string) ([]LightsailPortState, error) {
	region = lightsailNormalizeRegion(region)
	instanceName, err := lightsailRequireName(instanceName, "instanceName")
	if err != nil {
		return nil, err
	}
	if _, err := s.GetLightsailInstance(accountID, region, instanceName); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT from_port, to_port, protocol, cidrs_json
		 FROM lightsail_instance_ports
		 WHERE account_id = ? AND region = ? AND instance_name = ?
		 ORDER BY from_port, to_port, protocol`,
		accountID, region, instanceName,
	)
	if err != nil {
		return nil, fmt.Errorf("list lightsail ports: %w", err)
	}
	defer rows.Close()
	var out []LightsailPortState
	for rows.Next() {
		var p LightsailPortState
		var cidrsJSON string
		if err := rows.Scan(&p.FromPort, &p.ToPort, &p.Protocol, &cidrsJSON); err != nil {
			return nil, fmt.Errorf("list lightsail ports scan: %w", err)
		}
		_ = json.Unmarshal([]byte(cidrsJSON), &p.Cidrs)
		if len(p.Cidrs) == 0 {
			p.Cidrs = []string{"0.0.0.0/0"}
		}
		p.State = "open"
		out = append(out, p)
	}
	return out, rows.Err()
}
