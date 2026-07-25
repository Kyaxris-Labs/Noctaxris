package detective

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateGraphJSON builds CreateGraph response (GraphArn per AWS API).
func CreateGraphJSON(graphARN string) ([]byte, error) {
	return json.Marshal(map[string]any{"GraphArn": graphARN})
}

// ListGraphsJSON builds ListGraphs response (GraphList members use Arn and CreatedTime).
func ListGraphsJSON(graphs []store.DetectiveGraph) ([]byte, error) {
	list := make([]map[string]any, 0, len(graphs))
	for _, g := range graphs {
		created := g.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
		list = append(list, map[string]any{
			"Arn":         g.GraphARN,
			"CreatedTime": created,
		})
	}
	if list == nil {
		list = []map[string]any{}
	}
	return json.Marshal(map[string]any{"GraphList": list})
}

// AcceptInvitationJSON builds an empty AcceptInvitation body.
func AcceptInvitationJSON() ([]byte, error) {
	return json.Marshal(map[string]any{})
}

// SearchGraphJSON builds lab SearchGraph response joining CT and GuardDuty hits.
func SearchGraphJSON(result store.DetectiveSearchResult) ([]byte, error) {
	ct := make([]json.RawMessage, 0, len(result.CloudTrailEvents))
	for _, line := range result.CloudTrailEvents {
		ct = append(ct, line)
	}
	if ct == nil {
		ct = []json.RawMessage{}
	}
	gd := result.GuardDutyFindings
	if gd == nil {
		gd = []store.GuardDutyFinding{}
	}
	payload := map[string]any{
		"GraphArn":          result.GraphARN,
		"CloudTrailEvents":  ct,
		"GuardDutyFindings": gd,
	}
	if result.MatchedResourceArn != "" {
		payload["MatchedResourceArn"] = result.MatchedResourceArn
	}
	if result.MatchedIpAddress != "" {
		payload["MatchedIpAddress"] = result.MatchedIpAddress
	}
	return json.Marshal(payload)
}

// FormatGraphCreatedTime formats CreatedTime like ListGraphs samples.
func FormatGraphCreatedTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}
