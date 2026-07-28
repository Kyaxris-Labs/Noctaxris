package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// IoTMQTTDeviceContext is an ACTIVE certificate attached to a thing (MQTT authn).
type IoTMQTTDeviceContext struct {
	AccountID     string
	Region        string
	ThingName     string
	CertificateID string
}

// ResolveIoTMQTTDeviceByCertificate maps certificateId to account, thing, and cert when
// the certificate is ACTIVE and attached to exactly one thing principal.
func (s *Store) ResolveIoTMQTTDeviceByCertificate(certificateID string) (IoTMQTTDeviceContext, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTMQTTDeviceContext{}, err
	}
	certificateID = strings.TrimSpace(certificateID)
	if certificateID == "" {
		return IoTMQTTDeviceContext{}, fmt.Errorf("%w: certificateId required", ErrIoTBadRequest)
	}
	rows, err := s.db.Query(
		`SELECT c.account_id, c.region, c.certificate_id, c.certificate_arn, c.status, tp.thing_name
		 FROM iot_certificates c
		 JOIN iot_thing_principals tp
		   ON tp.account_id = c.account_id AND tp.region = c.region AND tp.principal = c.certificate_arn
		 WHERE c.certificate_id = ?`,
		certificateID,
	)
	if err != nil {
		return IoTMQTTDeviceContext{}, fmt.Errorf("resolve mqtt device: %w", err)
	}
	defer rows.Close()
	var out []IoTMQTTDeviceContext
	for rows.Next() {
		var account, region, certID, arn, status, thing string
		if err := rows.Scan(&account, &region, &certID, &arn, &status, &thing); err != nil {
			return IoTMQTTDeviceContext{}, fmt.Errorf("resolve mqtt device scan: %w", err)
		}
		if status != "ACTIVE" {
			continue
		}
		out = append(out, IoTMQTTDeviceContext{
			AccountID:     account,
			Region:        region,
			ThingName:     thing,
			CertificateID: certID,
		})
	}
	if err := rows.Err(); err != nil {
		return IoTMQTTDeviceContext{}, err
	}
	if len(out) == 0 {
		return IoTMQTTDeviceContext{}, ErrIoTNotFound
	}
	if len(out) > 1 {
		return IoTMQTTDeviceContext{}, fmt.Errorf("%w: certificate attached to multiple things", ErrIoTBadRequest)
	}
	return out[0], nil
}

// InferSingleActiveCertificateForThing returns the certificateId when the thing has exactly
// one ACTIVE attached certificate (lab MQTT bridge inference when Mosquitto ACL already gated publish).
func (s *Store) InferSingleActiveCertificateForThing(accountID, region, thingName string) (string, error) {
	active, err := s.listActiveCertificateIDsForThing(accountID, region, thingName)
	if err != nil {
		return "", err
	}
	if len(active) == 0 {
		return "", ErrIoTNotFound
	}
	if len(active) > 1 {
		return "", fmt.Errorf("%w: thing has multiple ACTIVE certificates", ErrIoTBadRequest)
	}
	return active[0], nil
}

// ResolveIoTMQTTDeviceForThingShadow picks an ACTIVE cert on the thing for MQTT shadow authz.
// When certificateID is set, that cert must be ACTIVE and attached to the thing.
// When empty: one ACTIVE cert is used; with multiple, the unique cert whose IoT policies Allow
// action/resource is used; zero or multiple Allows fails closed.
func (s *Store) ResolveIoTMQTTDeviceForThingShadow(accountID, region, thingName, certificateID, action, resource string) (IoTMQTTDeviceContext, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTMQTTDeviceContext{}, err
	}
	region = iotRegion(region)
	thingName = strings.TrimSpace(thingName)
	certificateID = strings.TrimSpace(certificateID)
	if thingName == "" {
		return IoTMQTTDeviceContext{}, fmt.Errorf("%w: thingName required", ErrIoTBadRequest)
	}
	if certificateID != "" {
		dev, err := s.ResolveIoTMQTTDeviceByCertificate(certificateID)
		if err != nil {
			return IoTMQTTDeviceContext{}, err
		}
		if !strings.EqualFold(dev.ThingName, thingName) ||
			dev.AccountID != accountID || iotRegion(dev.Region) != region {
			return IoTMQTTDeviceContext{}, fmt.Errorf("%w: certificate not attached to thing", ErrIoTBadRequest)
		}
		return dev, nil
	}
	active, err := s.listActiveCertificateIDsForThing(accountID, region, thingName)
	if err != nil {
		return IoTMQTTDeviceContext{}, err
	}
	if len(active) == 0 {
		return IoTMQTTDeviceContext{}, ErrIoTNotFound
	}
	if len(active) == 1 {
		return IoTMQTTDeviceContext{
			AccountID:     accountID,
			Region:        region,
			ThingName:     thingName,
			CertificateID: active[0],
		}, nil
	}
	var allowed []string
	for _, certID := range active {
		if s.EvaluateIoTDevicePolicy(accountID, region, certID, action, resource) {
			allowed = append(allowed, certID)
		}
	}
	if len(allowed) == 0 {
		return IoTMQTTDeviceContext{}, ErrIoTNotFound
	}
	if len(allowed) > 1 {
		return IoTMQTTDeviceContext{}, fmt.Errorf("%w: multiple ACTIVE certificates Allow this MQTT action; pass certificateId (MQTT ClientId = certificateId) or detach extras", ErrIoTBadRequest)
	}
	return IoTMQTTDeviceContext{
		AccountID:     accountID,
		Region:        region,
		ThingName:     thingName,
		CertificateID: allowed[0],
	}, nil
}

func (s *Store) listActiveCertificateIDsForThing(accountID, region, thingName string) ([]string, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return nil, err
	}
	region = iotRegion(region)
	thingName = strings.TrimSpace(thingName)
	principals, err := s.ListIoTThingPrincipals(accountID, region, thingName)
	if err != nil {
		return nil, err
	}
	var active []string
	for _, p := range principals {
		certID, err := s.activeCertificateIDForPrincipal(accountID, region, p)
		if err != nil {
			if errors.Is(err, ErrIoTNotFound) {
				continue
			}
			return nil, err
		}
		active = append(active, certID)
	}
	return active, nil
}

func (s *Store) activeCertificateIDForPrincipal(accountID, region, principalARN string) (string, error) {
	var certID, status string
	err := s.db.QueryRow(
		`SELECT certificate_id, status FROM iot_certificates
		 WHERE account_id = ? AND region = ? AND certificate_arn = ?`,
		accountID, iotRegion(region), strings.TrimSpace(principalARN),
	).Scan(&certID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrIoTNotFound
	}
	if err != nil {
		return "", fmt.Errorf("active cert for principal: %w", err)
	}
	if status != "ACTIVE" {
		return "", ErrIoTNotFound
	}
	return certID, nil
}

// IoTThingGlobal locates a thing by name within the process (lab singleton scope).
type IoTThingGlobal struct {
	AccountID string
	Region    string
	ThingName string
}

// DescribeIoTThingByNameGlobal finds a thing by thing name across accounts (fails when ambiguous).
func (s *Store) DescribeIoTThingByNameGlobal(thingName string) (IoTThingGlobal, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTThingGlobal{}, err
	}
	thingName = strings.TrimSpace(thingName)
	if thingName == "" {
		return IoTThingGlobal{}, fmt.Errorf("%w: thingName required", ErrIoTBadRequest)
	}
	rows, err := s.db.Query(
		`SELECT account_id, region, thing_name FROM iot_things WHERE thing_name = ?`,
		thingName,
	)
	if err != nil {
		return IoTThingGlobal{}, fmt.Errorf("describe thing global: %w", err)
	}
	defer rows.Close()
	var out []IoTThingGlobal
	for rows.Next() {
		var g IoTThingGlobal
		if err := rows.Scan(&g.AccountID, &g.Region, &g.ThingName); err != nil {
			return IoTThingGlobal{}, err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return IoTThingGlobal{}, err
	}
	if len(out) == 0 {
		return IoTThingGlobal{}, ErrIoTNotFound
	}
	if len(out) > 1 {
		return IoTThingGlobal{}, fmt.Errorf("%w: thing name ambiguous across accounts", ErrIoTBadRequest)
	}
	return out[0], nil
}
