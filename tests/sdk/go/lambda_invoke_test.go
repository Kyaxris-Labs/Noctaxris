package sdk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

func requireNested(t *testing.T) {
	t.Helper()
	if os.Getenv("NOCTAXRIS_NESTED") != "1" {
		t.Skip("nested Lambda Invoke — set NOCTAXRIS_NESTED=1 (Compose noctaxris-engine healthy)")
	}
	requireReady(t)
}

func TestLambdaNestedInvoke(t *testing.T) {
	requireNested(t)
	cfg := loadAWSConfig(t)
	iamClient := newIAM(t, cfg)
	lam := newLambda(t, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	prefix := uniquePrefix(t)

	roleName := prefix + "-lambda-role"
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleOut, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trust),
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if roleOut.Role == nil || roleOut.Role.Arn == nil {
		t.Fatal("CreateRole missing Arn")
	}
	roleARN := *roleOut.Role.Arn
	t.Cleanup(func() {
		_, _ = iamClient.DeleteRole(context.Background(), &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	})

	fnName := prefix + "-invoke-fn"
	_, err = lam.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Runtime:      types.RuntimePython312,
		Role:         aws.String(roleARN),
		Handler:      aws.String("index.handler"),
		Code:         &types.FunctionCode{ZipFile: minimalPythonZip(t)},
	})
	if err != nil {
		t.Fatalf("CreateFunction: %v", err)
	}
	t.Cleanup(func() {
		_, _ = lam.DeleteFunction(context.Background(), &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})
	})

	inv, err := lam.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: aws.String(fnName),
		Payload:      []byte("{}"),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if inv.FunctionError != nil && *inv.FunctionError != "" {
		t.Fatalf("Invoke FunctionError=%s payload=%s", *inv.FunctionError, string(inv.Payload))
	}
	payload := inv.Payload
	if len(payload) == 0 {
		t.Fatal("Invoke missing Payload")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(payload)).Decode(&body); err != nil {
		raw, _ := io.ReadAll(bytes.NewReader(payload))
		if !bytes.Contains(raw, []byte("ok")) {
			t.Fatalf("Invoke payload not JSON and missing ok: %s (%v)", raw, err)
		}
		return
	}
	if body["ok"] != true {
		t.Fatalf("Invoke payload unexpected: %v", body)
	}
}
