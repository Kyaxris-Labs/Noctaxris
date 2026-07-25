package route53

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateHostedZoneJSON builds CreateHostedZone response.
func CreateHostedZoneJSON(z store.Route53HostedZone) ([]byte, error) {
	return json.Marshal(map[string]any{
		"HostedZone": map[string]any{
			"Id":   "/hostedzone/" + z.ID,
			"Name": z.Name,
			"Config": map[string]any{
				"PrivateZone": z.PrivateZone,
			},
			"CallerReference": z.CallerRef,
		},
		"ChangeInfo": map[string]any{
			"Id":     "C" + z.ID,
			"Status": "INSYNC",
		},
	})
}

// DeleteHostedZoneJSON is an empty OK body with ChangeInfo.
func DeleteHostedZoneJSON(zoneID string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ChangeInfo": map[string]any{"Id": "C" + zoneID, "Status": "INSYNC"},
	})
}

// ListHostedZonesJSON builds ListHostedZones response.
func ListHostedZonesJSON(zones []store.Route53HostedZone) ([]byte, error) {
	items := make([]map[string]any, 0, len(zones))
	for _, z := range zones {
		items = append(items, map[string]any{
			"Id":   "/hostedzone/" + z.ID,
			"Name": z.Name,
			"Config": map[string]any{
				"PrivateZone": z.PrivateZone,
			},
		})
	}
	return json.Marshal(map[string]any{"HostedZones": items})
}

// ChangeResourceRecordSetsJSON builds ChangeResourceRecordSets response.
func ChangeResourceRecordSetsJSON(changeID string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ChangeInfo": map[string]any{"Id": changeID, "Status": "INSYNC"},
	})
}

// InjectQueryLogsJSON builds lab InjectQueryLogs response.
func InjectQueryLogsJSON(delivered int, logGroup, logStream string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Delivered":     delivered,
		"LogGroupName":  logGroup,
		"LogStreamName": logStream,
	})
}

// ListResourceRecordSetsJSON builds ListResourceRecordSets response.
func ListResourceRecordSetsJSON(sets []store.Route53ResourceRecordSet) ([]byte, error) {
	items := make([]map[string]any, 0, len(sets))
	for _, rs := range sets {
		recs := make([]map[string]any, 0, len(rs.Records))
		for _, v := range rs.Records {
			recs = append(recs, map[string]any{"Value": v})
		}
		items = append(items, map[string]any{
			"Name":            rs.Name,
			"Type":            rs.Type,
			"TTL":             rs.TTL,
			"ResourceRecords": recs,
		})
	}
	return json.Marshal(map[string]any{"ResourceRecordSets": items})
}
