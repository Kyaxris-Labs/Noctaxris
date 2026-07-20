package mq

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateBrokerJSON builds a CreateBroker success body.
func CreateBrokerJSON(b store.MQBroker) ([]byte, error) {
	return json.Marshal(map[string]any{
		"BrokerId":  b.BrokerID,
		"BrokerArn": b.BrokerARN,
	})
}

// DescribeBrokerJSON builds a DescribeBroker success body.
func DescribeBrokerJSON(b store.MQBroker) ([]byte, error) {
	return json.Marshal(map[string]any{
		"BrokerId":         b.BrokerID,
		"BrokerName":       b.BrokerName,
		"BrokerArn":        b.BrokerARN,
		"BrokerState":      b.BrokerState,
		"EngineType":       b.EngineType,
		"EngineVersion":    b.EngineVersion,
		"DeploymentMode":   b.DeploymentMode,
		"HostInstanceType": b.HostInstanceType,
		"BrokerInstances": []map[string]any{
			{
				"ConsoleURL": b.StubEndpoint,
				"Endpoints":  []string{b.StubEndpoint},
				"IpAddress":  "127.0.0.1",
			},
		},
	})
}

// ListBrokersJSON builds a ListBrokers success body.
func ListBrokersJSON(brokers []store.MQBroker) ([]byte, error) {
	summaries := make([]map[string]any, 0, len(brokers))
	for _, b := range brokers {
		summaries = append(summaries, map[string]any{
			"BrokerId":    b.BrokerID,
			"BrokerName":  b.BrokerName,
			"BrokerArn":   b.BrokerARN,
			"BrokerState": b.BrokerState,
			"EngineType":  b.EngineType,
			"HostInstanceType": b.HostInstanceType,
			"DeploymentMode":   b.DeploymentMode,
		})
	}
	return json.Marshal(map[string]any{"BrokerSummaries": summaries})
}
