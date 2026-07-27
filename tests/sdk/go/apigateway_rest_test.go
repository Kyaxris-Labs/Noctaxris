package sdk_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/aws/aws-sdk-go-v2/service/apigateway/types"
)

func newAPIGW(t *testing.T, cfg aws.Config) *apigateway.Client {
	t.Helper()
	return apigateway.NewFromConfig(cfg, func(o *apigateway.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func TestRestAPIMOCKExecutePath(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newAPIGW(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	httpClient := &http.Client{Timeout: 30 * time.Second}

	apiOut, err := client.CreateRestApi(ctx, &apigateway.CreateRestApiInput{
		Name: aws.String(prefix + "-rest"),
	})
	if err != nil {
		t.Fatalf("CreateRestApi: %v", err)
	}
	if apiOut.Id == nil || *apiOut.Id == "" {
		t.Fatal("CreateRestApi missing Id")
	}
	apiID := *apiOut.Id
	t.Cleanup(func() {
		_, _ = client.DeleteRestApi(context.Background(), &apigateway.DeleteRestApiInput{RestApiId: aws.String(apiID)})
	})

	resOut, err := client.GetResources(ctx, &apigateway.GetResourcesInput{RestApiId: aws.String(apiID)})
	if err != nil {
		t.Fatalf("GetResources: %v", err)
	}
	var rootID string
	for _, item := range resOut.Items {
		if item.Path != nil && *item.Path == "/" && item.Id != nil {
			rootID = *item.Id
			break
		}
	}
	if rootID == "" {
		t.Fatal("missing root resource")
	}

	child, err := client.CreateResource(ctx, &apigateway.CreateResourceInput{
		RestApiId: aws.String(apiID),
		ParentId:  aws.String(rootID),
		PathPart:  aws.String("hello"),
	})
	if err != nil {
		t.Fatalf("CreateResource: %v", err)
	}
	if child.Id == nil {
		t.Fatal("CreateResource missing Id")
	}
	resourceID := *child.Id

	_, err = client.PutMethod(ctx, &apigateway.PutMethodInput{
		RestApiId:         aws.String(apiID),
		ResourceId:        aws.String(resourceID),
		HttpMethod:        aws.String("GET"),
		AuthorizationType: aws.String("NONE"),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NOCTAXRIS_ALLOW_OPEN_DATA_PLANE") {
			t.Skip("REST MOCK soft-skip: set NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1 (compose.lab-open.yaml) for AuthType NONE on non-loopback listen")
		}
		t.Fatalf("PutMethod: %v", err)
	}

	_, err = client.PutIntegration(ctx, &apigateway.PutIntegrationInput{
		RestApiId:  aws.String(apiID),
		ResourceId: aws.String(resourceID),
		HttpMethod: aws.String("GET"),
		Type:       types.IntegrationTypeMock,
		RequestTemplates: map[string]string{
			"application/json": `{"message":"sdk-mock-ok"}`,
		},
	})
	if err != nil {
		t.Fatalf("PutIntegration MOCK: %v", err)
	}

	_, err = client.CreateDeployment(ctx, &apigateway.CreateDeploymentInput{
		RestApiId: aws.String(apiID),
		StageName: aws.String("dev"),
	})
	if err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	url := fmt.Sprintf("%s/restapis/%s/dev/_user_request_/hello", endpoint(), apiID)
	resp, err := httpClient.Get(url)
	if err != nil {
		t.Fatalf("execute GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("execute status=%d body=%q", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "sdk-mock-ok") {
		t.Fatalf("body=%q", body)
	}
}
