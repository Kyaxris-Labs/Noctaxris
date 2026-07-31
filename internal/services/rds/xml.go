package rds

import (
	"encoding/xml"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/awsprotocol"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const xmlns = "http://rds.amazonaws.com/doc/2014-10-31/"

type responseMetadata struct {
	RequestID string `xml:"RequestId"`
}

func marshal(name string, result any, requestID string) ([]byte, error) {
	type envelope struct {
		XMLName  xml.Name         `xml:""`
		Result   any              `xml:",any"`
		Metadata responseMetadata `xml:"ResponseMetadata"`
	}
	env := envelope{
		XMLName:  xml.Name{Local: name, Space: xmlns},
		Result:   result,
		Metadata: responseMetadata{RequestID: requestID},
	}
	data, err := xml.Marshal(env)
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), data...), nil
}

func instanceXML(inst store.RDSDBInstance) any {
	type endpoint struct {
		Address string `xml:"Address"`
		Port    int    `xml:"Port"`
	}
	type masterSecret struct {
		SecretARN string `xml:"SecretArn"`
	}
	type dbInstance struct {
		DBInstanceIdentifier string       `xml:"DBInstanceIdentifier"`
		DBInstanceArn        string       `xml:"DBInstanceArn"`
		Engine               string       `xml:"Engine"`
		EngineVersion        string       `xml:"EngineVersion"`
		DBInstanceClass      string       `xml:"DBInstanceClass"`
		DBName               string       `xml:"DBName,omitempty"`
		MasterUsername       string       `xml:"MasterUsername"`
		DBInstanceStatus     string       `xml:"DBInstanceStatus"`
		AllocatedStorage     int          `xml:"AllocatedStorage"`
		Endpoint             *endpoint    `xml:"Endpoint,omitempty"`
		MasterUserSecret     masterSecret `xml:"MasterUserSecret"`
		InstanceCreateTime   string       `xml:"InstanceCreateTime"`
	}
	out := dbInstance{
		DBInstanceIdentifier: inst.DBInstanceIdentifier,
		DBInstanceArn:        inst.DBInstanceARN,
		Engine:               inst.Engine,
		EngineVersion:        inst.EngineVersion,
		DBInstanceClass:      inst.DBInstanceClass,
		DBName:               inst.DBName,
		MasterUsername:       inst.MasterUsername,
		DBInstanceStatus:     inst.DBInstanceStatus,
		AllocatedStorage:     inst.AllocatedStorage,
		MasterUserSecret:     masterSecret{SecretARN: inst.MasterUserSecretARN},
		InstanceCreateTime:   time.UnixMilli(inst.CreatedAt).UTC().Format(time.RFC3339),
	}
	if inst.EndpointAddress != "" {
		out.Endpoint = &endpoint{Address: inst.EndpointAddress, Port: inst.EndpointPort}
	}
	return out
}

// CreateDBInstanceXML builds CreateDBInstanceResponse.
func CreateDBInstanceXML(inst store.RDSDBInstance, requestID string) ([]byte, error) {
	type result struct {
		XMLName    xml.Name `xml:"CreateDBInstanceResult"`
		DBInstance any      `xml:"DBInstance"`
	}
	return marshal("CreateDBInstanceResponse", result{DBInstance: instanceXML(inst)}, requestID)
}

// DeleteDBInstanceXML builds DeleteDBInstanceResponse.
func DeleteDBInstanceXML(inst store.RDSDBInstance, requestID string) ([]byte, error) {
	type result struct {
		XMLName    xml.Name `xml:"DeleteDBInstanceResult"`
		DBInstance any      `xml:"DBInstance"`
	}
	return marshal("DeleteDBInstanceResponse", result{DBInstance: instanceXML(inst)}, requestID)
}

// DescribeDBInstancesXML builds DescribeDBInstancesResponse.
func DescribeDBInstancesXML(instances []store.RDSDBInstance, requestID string) ([]byte, error) {
	type result struct {
		XMLName     xml.Name `xml:"DescribeDBInstancesResult"`
		DBInstances struct {
			Member []any `xml:"DBInstance"`
		} `xml:"DBInstances"`
	}
	var r result
	for _, inst := range instances {
		r.DBInstances.Member = append(r.DBInstances.Member, instanceXML(inst))
	}
	return marshal("DescribeDBInstancesResponse", r, requestID)
}

// ErrorXML builds a Query protocol error response.
func ErrorXML(code, message, requestID string) []byte {
	body, err := awsprotocol.MarshalQueryError(awsprotocol.QueryErrorIAM, xmlns, code, message, requestID)
	if err != nil {
		return nil
	}
	return body
}
