package codec

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Compress gzip-compresses data using the default gzip writer settings.
func Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		_ = zw.Close()
		return nil, fmt.Errorf("gzip write: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	return buf.Bytes(), nil
}

// Decompress gunzips data produced by Compress or compatible gzip writers.
func Decompress(data []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var out bytes.Buffer
	if _, err := out.ReadFrom(zr); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// AwslogsEnvelopeEncode wraps DATA_MESSAGE JSON as AWS Lambda subscription
// shape: {"awslogs":{"data":"<base64(gzip(json))>"}}.
func AwslogsEnvelopeEncode(dataMessageJSON []byte) ([]byte, error) {
	compressed, err := Compress(dataMessageJSON)
	if err != nil {
		return nil, fmt.Errorf("awslogs %w", err)
	}
	return json.Marshal(map[string]any{
		"awslogs": map[string]string{
			"data": base64.StdEncoding.EncodeToString(compressed),
		},
	})
}

// AwslogsEnvelopeDecode extracts and gunzips awslogs.data from an envelope.
func AwslogsEnvelopeDecode(envelope []byte) ([]byte, error) {
	var wrap struct {
		Awslogs struct {
			Data string `json:"data"`
		} `json:"awslogs"`
	}
	if err := json.Unmarshal(envelope, &wrap); err != nil {
		return nil, err
	}
	if wrap.Awslogs.Data == "" {
		return nil, fmt.Errorf("awslogs.data missing")
	}
	raw, err := base64.StdEncoding.DecodeString(wrap.Awslogs.Data)
	if err != nil {
		return nil, err
	}
	return Decompress(raw)
}
