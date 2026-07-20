package docdb

import (
	"encoding/xml"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

type responseMetadata struct {
	RequestID string `xml:"RequestId"`
}

// CreateDBClusterXML builds a CreateDBCluster response.
func CreateDBClusterXML(c store.DocDBCluster, requestID string) ([]byte, error) {
	type dbCluster struct {
		XMLName             xml.Name `xml:"DBCluster"`
		DBClusterIdentifier string   `xml:"DBClusterIdentifier"`
		Engine              string   `xml:"Engine"`
		EngineVersion       string   `xml:"EngineVersion"`
		Status              string   `xml:"Status"`
		Endpoint            string   `xml:"Endpoint"`
		Port                int      `xml:"Port"`
		MasterUsername      string   `xml:"MasterUsername,omitempty"`
	}
	type result struct {
		XMLName   xml.Name  `xml:"CreateDBClusterResult"`
		DBCluster dbCluster `xml:"DBCluster"`
	}
	return marshal("CreateDBClusterResponse", result{
		DBCluster: dbCluster{
			DBClusterIdentifier: c.DBClusterIdentifier,
			Engine:              c.Engine,
			EngineVersion:       c.EngineVersion,
			Status:              c.Status,
			Endpoint:            c.EndpointAddress,
			Port:                c.EndpointPort,
			MasterUsername:      c.MasterUsername,
		},
	}, requestID)
}

// DescribeDBClustersXML builds a DescribeDBClusters response.
func DescribeDBClustersXML(clusters []store.DocDBCluster, requestID string) ([]byte, error) {
	type dbCluster struct {
		DBClusterIdentifier string `xml:"DBClusterIdentifier"`
		Engine              string `xml:"Engine"`
		EngineVersion       string `xml:"EngineVersion"`
		Status              string `xml:"Status"`
		Endpoint            string `xml:"Endpoint"`
		Port                int    `xml:"Port"`
		MasterUsername      string `xml:"MasterUsername,omitempty"`
	}
	type result struct {
		XMLName    xml.Name `xml:"DescribeDBClustersResult"`
		DBClusters struct {
			Member []dbCluster `xml:"DBCluster"`
		} `xml:"DBClusters"`
	}
	var r result
	for _, c := range clusters {
		r.DBClusters.Member = append(r.DBClusters.Member, dbCluster{
			DBClusterIdentifier: c.DBClusterIdentifier,
			Engine:              c.Engine,
			EngineVersion:       c.EngineVersion,
			Status:              c.Status,
			Endpoint:            c.EndpointAddress,
			Port:                c.EndpointPort,
			MasterUsername:      c.MasterUsername,
		})
	}
	return marshal("DescribeDBClustersResponse", r, requestID)
}

// DeleteDBClusterXML builds a DeleteDBCluster response.
func DeleteDBClusterXML(c store.DocDBCluster, requestID string) ([]byte, error) {
	type dbCluster struct {
		XMLName             xml.Name `xml:"DBCluster"`
		DBClusterIdentifier string   `xml:"DBClusterIdentifier"`
		Status              string   `xml:"Status"`
		Engine              string   `xml:"Engine"`
	}
	type result struct {
		XMLName   xml.Name  `xml:"DeleteDBClusterResult"`
		DBCluster dbCluster `xml:"DBCluster"`
	}
	return marshal("DeleteDBClusterResponse", result{
		DBCluster: dbCluster{
			DBClusterIdentifier: c.DBClusterIdentifier,
			Status:              "deleting",
			Engine:              c.Engine,
		},
	}, requestID)
}

// ErrorXML builds a DocumentDB error response.
func ErrorXML(code, message, requestID string) ([]byte, error) {
	type errBody struct {
		Type    string `xml:"Type"`
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	type errorResponse struct {
		XMLName   xml.Name `xml:"ErrorResponse"`
		XMLNS     string   `xml:"xmlns,attr"`
		Error     errBody  `xml:"Error"`
		RequestID string   `xml:"RequestId"`
	}
	return xml.Marshal(errorResponse{
		XMLNS:     "http://rds.amazonaws.com/doc/2014-10-31/",
		Error:     errBody{Type: "Sender", Code: code, Message: message},
		RequestID: requestID,
	})
}

func marshal(root string, result any, requestID string) ([]byte, error) {
	type wrap struct {
		XMLName  xml.Name         `xml:""`
		XMLNS    string           `xml:"xmlns,attr"`
		Result   any              `xml:",omitempty"`
		Metadata responseMetadata `xml:"ResponseMetadata"`
	}
	w := wrap{
		XMLName:  xml.Name{Local: root},
		XMLNS:    "http://rds.amazonaws.com/doc/2014-10-31/",
		Result:   result,
		Metadata: responseMetadata{RequestID: requestID},
	}
	out, err := xml.Marshal(w)
	if err != nil {
		return nil, fmt.Errorf("marshal docdb xml: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}
