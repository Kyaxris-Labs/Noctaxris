package ec2

import "encoding/json"

// CreateFlowLogsJSON builds CreateFlowLogs response (FlowLogIds lite).
func CreateFlowLogsJSON(flowLogIDs []string, unsuccessful []map[string]any) ([]byte, error) {
	if flowLogIDs == nil {
		flowLogIDs = []string{}
	}
	if unsuccessful == nil {
		unsuccessful = []map[string]any{}
	}
	return json.Marshal(map[string]any{
		"FlowLogIds":     flowLogIDs,
		"Unsuccessful":   unsuccessful,
		"ClientToken":    "",
	})
}

// InjectFlowLogsJSON builds lab InjectFlowLogs response.
func InjectFlowLogsJSON(delivered int, flowLogID string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"FlowLogId":       flowLogID,
		"DeliveredRecords": delivered,
	})
}
