package firehose

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateDeliveryStreamJSON builds a CreateDeliveryStream response.
func CreateDeliveryStreamJSON(st store.FirehoseStream) ([]byte, error) {
	return json.Marshal(map[string]any{"DeliveryStreamARN": st.StreamARN})
}

// DescribeDeliveryStreamJSON builds a DescribeDeliveryStream response.
func DescribeDeliveryStreamJSON(st store.FirehoseStream) ([]byte, error) {
	dest := map[string]any{}
	switch st.DestType {
	case "S3":
		dest["S3DestinationDescription"] = map[string]any{
			"BucketARN": "arn:aws:s3:::" + st.DestBucket,
			"Prefix":    st.DestPrefix,
			"RoleARN":   st.RoleARN,
		}
	case "Lambda":
		dest["LambdaDestinationDescription"] = map[string]any{
			"LambdaArn": st.DestLambdaARN,
			"RoleARN":   st.RoleARN,
		}
	case "OpenSearch":
		domainARN := store.OpenSearchDomainARN(store.DefaultFirehoseRegion, accountIDFromFirehoseARN(st.StreamARN), st.DestOpenSearchDomain)
		osDesc := map[string]any{
			"DomainARN": domainARN,
			"IndexName": st.DestOpenSearchIndex,
			"RoleARN":   st.RoleARN,
		}
		// AWS shape (primary) plus lab alias for Create symmetry.
		dest["AmazonopensearchserviceDestinationDescription"] = osDesc
		dest["OpenSearchDestinationDescription"] = osDesc
	case "VPCFlow":
		vpcDesc := map[string]any{
			"BucketARN": "arn:aws:s3:::" + st.DestBucket,
			"Prefix":    st.DestPrefix,
			"RoleARN":   st.RoleARN,
		}
		dest["VpcFlowLogsDestinationDescription"] = vpcDesc
		dest["NoctaxrisVpcFlowDestinationDescription"] = vpcDesc
	}
	return json.Marshal(map[string]any{
		"DeliveryStreamDescription": map[string]any{
			"DeliveryStreamName":   st.Name,
			"DeliveryStreamARN":    st.StreamARN,
			"DeliveryStreamStatus": "ACTIVE",
			"DeliveryStreamType":   "DirectPut",
			"CreateTimestamp":      float64(st.CreatedAt) / 1000.0,
			"Destinations":         []any{dest},
		},
	})
}

// accountIDFromFirehoseARN extracts the 12-digit account from a delivery stream ARN.
func accountIDFromFirehoseARN(arn string) string {
	// arn:aws:firehose:REGION:ACCOUNT:deliverystream/NAME
	parts := strings.Split(arn, ":")
	if len(parts) >= 5 {
		return parts[4]
	}
	return ""
}

// ListDeliveryStreamsJSON builds a ListDeliveryStreams response.
func ListDeliveryStreamsJSON(streams []store.FirehoseStream) ([]byte, error) {
	names := make([]string, 0, len(streams))
	for _, st := range streams {
		names = append(names, st.Name)
	}
	return json.Marshal(map[string]any{"DeliveryStreamNames": names, "HasMoreDeliveryStreams": false})
}

// DeleteDeliveryStreamJSON is an empty OK body.
func DeleteDeliveryStreamJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// PutRecordJSON builds a PutRecord response.
func PutRecordJSON(recordID string) ([]byte, error) {
	return json.Marshal(map[string]any{"RecordId": recordID})
}

// PutRecordBatchJSON builds a PutRecordBatch response.
func PutRecordBatchJSON(failed []int, total int) ([]byte, error) {
	responses := make([]map[string]any, total)
	for i := 0; i < total; i++ {
		responses[i] = map[string]any{"RecordId": time.Now().UTC().Format("20060102150405") + "-" + string(rune('a'+i%26))}
	}
	for _, idx := range failed {
		if idx >= 0 && idx < total {
			responses[idx] = map[string]any{"ErrorCode": "ServiceUnavailable", "ErrorMessage": "failed"}
		}
	}
	return json.Marshal(map[string]any{
		"FailedPutCount":   len(failed),
		"RequestResponses": responses,
	})
}
