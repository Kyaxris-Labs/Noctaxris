package elasticache

import (
	"encoding/xml"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

type responseMetadata struct {
	RequestID string `xml:"RequestId"`
}

// CreateCacheClusterXML builds a CreateCacheCluster response.
func CreateCacheClusterXML(c store.ElastiCacheCluster, requestID string) ([]byte, error) {
	type cacheCluster struct {
		XMLName           xml.Name `xml:"CacheCluster"`
		CacheClusterID    string   `xml:"CacheClusterId"`
		CacheClusterStatus string  `xml:"CacheClusterStatus"`
		CacheNodeType     string   `xml:"CacheNodeType"`
		Engine            string   `xml:"Engine"`
		EngineVersion     string   `xml:"EngineVersion"`
		NumCacheNodes     int      `xml:"NumCacheNodes"`
		ConfigurationEndpoint *endpoint `xml:"ConfigurationEndpoint"`
	}
	type result struct {
		XMLName      xml.Name     `xml:"CreateCacheClusterResult"`
		CacheCluster cacheCluster `xml:"CacheCluster"`
	}
	return marshal("CreateCacheClusterResponse", result{
		CacheCluster: cacheCluster{
			CacheClusterID:     c.CacheClusterID,
			CacheClusterStatus: c.Status,
			CacheNodeType:      c.CacheNodeType,
			Engine:             c.Engine,
			EngineVersion:      c.EngineVersion,
			NumCacheNodes:      c.NumCacheNodes,
			ConfigurationEndpoint: &endpoint{
				Address: c.EndpointAddress,
				Port:    c.EndpointPort,
			},
		},
	}, requestID)
}

type endpoint struct {
	Address string `xml:"Address"`
	Port    int    `xml:"Port"`
}

// DescribeCacheClustersXML builds a DescribeCacheClusters response.
func DescribeCacheClustersXML(clusters []store.ElastiCacheCluster, requestID string) ([]byte, error) {
	type cacheNode struct {
		CacheNodeID string    `xml:"CacheNodeId"`
		Endpoint    *endpoint `xml:"Endpoint"`
	}
	type cacheCluster struct {
		CacheClusterID     string `xml:"CacheClusterId"`
		CacheClusterStatus string `xml:"CacheClusterStatus"`
		CacheNodeType      string `xml:"CacheNodeType"`
		Engine             string `xml:"Engine"`
		EngineVersion      string `xml:"EngineVersion"`
		NumCacheNodes      int    `xml:"NumCacheNodes"`
		CacheNodes         struct {
			Member []cacheNode `xml:"CacheNode"`
		} `xml:"CacheNodes"`
	}
	type result struct {
		XMLName       xml.Name `xml:"DescribeCacheClustersResult"`
		CacheClusters struct {
			Member []cacheCluster `xml:"CacheCluster"`
		} `xml:"CacheClusters"`
	}
	var r result
	for _, c := range clusters {
		cc := cacheCluster{
			CacheClusterID:     c.CacheClusterID,
			CacheClusterStatus: c.Status,
			CacheNodeType:      c.CacheNodeType,
			Engine:             c.Engine,
			EngineVersion:      c.EngineVersion,
			NumCacheNodes:      c.NumCacheNodes,
		}
		cc.CacheNodes.Member = append(cc.CacheNodes.Member, cacheNode{
			CacheNodeID: "0001",
			Endpoint: &endpoint{
				Address: c.EndpointAddress,
				Port:    c.EndpointPort,
			},
		})
		r.CacheClusters.Member = append(r.CacheClusters.Member, cc)
	}
	return marshal("DescribeCacheClustersResponse", r, requestID)
}

// DeleteCacheClusterXML builds a DeleteCacheCluster response.
func DeleteCacheClusterXML(c store.ElastiCacheCluster, requestID string) ([]byte, error) {
	type cacheCluster struct {
		XMLName            xml.Name `xml:"CacheCluster"`
		CacheClusterID     string   `xml:"CacheClusterId"`
		CacheClusterStatus string   `xml:"CacheClusterStatus"`
	}
	type result struct {
		XMLName      xml.Name     `xml:"DeleteCacheClusterResult"`
		CacheCluster cacheCluster `xml:"CacheCluster"`
	}
	return marshal("DeleteCacheClusterResponse", result{
		CacheCluster: cacheCluster{
			CacheClusterID:     c.CacheClusterID,
			CacheClusterStatus: "deleting",
		},
	}, requestID)
}

// ErrorXML builds an ElastiCache error response.
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
		XMLNS:     "http://elasticache.amazonaws.com/doc/2015-02-02/",
		Error:     errBody{Type: "Sender", Code: code, Message: message},
		RequestID: requestID,
	})
}

func marshal(root string, result any, requestID string) ([]byte, error) {
	type wrap struct {
		XMLName  xml.Name          `xml:""`
		XMLNS    string            `xml:"xmlns,attr"`
		Result   any               `xml:",omitempty"`
		Metadata responseMetadata  `xml:"ResponseMetadata"`
	}
	w := wrap{
		XMLName:  xml.Name{Local: root},
		XMLNS:    "http://elasticache.amazonaws.com/doc/2015-02-02/",
		Result:   result,
		Metadata: responseMetadata{RequestID: requestID},
	}
	out, err := xml.Marshal(w)
	if err != nil {
		return nil, fmt.Errorf("marshal elasticache xml: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}
