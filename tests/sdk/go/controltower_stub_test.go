package sdk_test

import (
	"strings"
	"testing"
)

func TestControlTowerListEmptyGetNotFound(t *testing.T) {
	requireReady(t)

	listStatus, listBody, listParsed := signedJSONTarget(t, "controltower", "ControlTower.ListLandingZones", map[string]any{})
	if listStatus != 200 {
		t.Fatalf("ListLandingZones status=%d body=%s", listStatus, listBody)
	}
	zones, _ := listParsed["landingZones"].([]any)
	if len(zones) != 0 {
		t.Fatalf("landingZones=%v want empty", zones)
	}

	getStatus, getBody, _ := signedJSONTarget(t, "controltower", "ControlTower.GetLandingZone", map[string]any{
		"landingZoneIdentifier": "arn:aws:controltower:us-east-1:000000000001:landingzone/lz-deadbeef",
	})
	if getStatus != 400 {
		t.Fatalf("GetLandingZone status=%d body=%s want 400", getStatus, getBody)
	}
	if !strings.Contains(string(getBody), "ResourceNotFoundException") {
		t.Fatalf("GetLandingZone body=%s want ResourceNotFoundException", getBody)
	}
}
