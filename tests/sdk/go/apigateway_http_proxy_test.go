package sdk_test

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apitypes "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
)

func TestHTTPProxyIntegrationSoftSkipUnlessEnv(t *testing.T) {
	requireReady(t)
	if os.Getenv("NOCTAXRIS_APIGW_HTTP_PROXY") != "1" {
		t.Skip("HTTP_PROXY soft-skip: set NOCTAXRIS_APIGW_HTTP_PROXY=1 and allowlist on the API process")
	}
	if os.Getenv("NOCTAXRIS_APIGW_HTTP_PROXY_ALLOWLIST") == "" {
		t.Skip("HTTP_PROXY soft-skip: set NOCTAXRIS_APIGW_HTTP_PROXY_ALLOWLIST on the API process")
	}
	cfg := loadAWSConfig(t)
	apigw := newAPIGWv2(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	apiOut, err := apigw.CreateApi(ctx, &apigatewayv2.CreateApiInput{
		Name:         aws.String(prefix + "-proxy-api"),
		ProtocolType: apitypes.ProtocolTypeHttp,
	})
	if err != nil {
		t.Fatalf("CreateApi: %v", err)
	}
	apiID := *apiOut.ApiId
	t.Cleanup(func() {
		_, _ = apigw.DeleteApi(context.Background(), &apigatewayv2.DeleteApiInput{ApiId: aws.String(apiID)})
	})

	allow := os.Getenv("NOCTAXRIS_APIGW_HTTP_PROXY_ALLOWLIST")
	uri := "https://example.com/lab"
	// Prefer first allowlist entry if it looks like a URL prefix.
	for _, part := range splitComma(allow) {
		if len(part) > 8 && (part[:7] == "http://" || part[:8] == "https://") {
			uri = part
			if uri[len(uri)-1] != '/' {
				uri += "/sdk-smoke"
			} else {
				uri += "sdk-smoke"
			}
			break
		}
	}
	_, err = apigw.CreateIntegration(ctx, &apigatewayv2.CreateIntegrationInput{
		ApiId:           aws.String(apiID),
		IntegrationType: apitypes.IntegrationTypeHttpProxy,
		IntegrationUri:  aws.String(uri),
	})
	if err != nil {
		t.Fatalf("CreateIntegration HTTP_PROXY: %v", err)
	}
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			for len(part) > 0 && (part[0] == ' ' || part[0] == '\t') {
				part = part[1:]
			}
			for len(part) > 0 && (part[len(part)-1] == ' ' || part[len(part)-1] == '\t') {
				part = part[:len(part)-1]
			}
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}
