package codedeploy

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateApplicationJSON builds CreateApplication response.
func CreateApplicationJSON(a store.CodeDeployApplication) ([]byte, error) {
	return json.Marshal(map[string]any{"applicationId": a.ApplicationID})
}

// CreateDeploymentGroupJSON builds CreateDeploymentGroup response.
func CreateDeploymentGroupJSON(g store.CodeDeployDeploymentGroup) ([]byte, error) {
	return json.Marshal(map[string]any{"deploymentGroupId": g.DeploymentGroupID})
}

// CreateDeploymentJSON builds CreateDeployment response.
func CreateDeploymentJSON(d store.CodeDeployDeployment) ([]byte, error) {
	return json.Marshal(map[string]any{"deploymentId": d.DeploymentID})
}

// GetDeploymentJSON builds GetDeployment response.
func GetDeploymentJSON(d store.CodeDeployDeployment) ([]byte, error) {
	return json.Marshal(map[string]any{
		"deploymentInfo": map[string]any{
			"applicationName":     d.ApplicationName,
			"deploymentGroupName": d.DeploymentGroupName,
			"deploymentId":        d.DeploymentID,
			"status":              d.Status,
			"description":         d.Description,
		},
	})
}

// ListDeploymentsJSON builds ListDeployments response.
func ListDeploymentsJSON(deps []store.CodeDeployDeployment) ([]byte, error) {
	ids := make([]string, 0, len(deps))
	for _, d := range deps {
		ids = append(ids, d.DeploymentID)
	}
	return json.Marshal(map[string]any{"deployments": ids})
}
