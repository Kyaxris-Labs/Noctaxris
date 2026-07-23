package sdk_test

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

func TestLambdaCreateGetDelete(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	iamClient := newIAM(t, cfg)
	lam := newLambda(t, cfg)
	ctx := context.Background()
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
		_, _ = iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	})

	zipBytes := minimalPythonZip(t)
	fnName := prefix + "-fn"
	_, err = lam.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Runtime:      types.RuntimePython312,
		Role:         aws.String(roleARN),
		Handler:      aws.String("index.handler"),
		Code:         &types.FunctionCode{ZipFile: zipBytes},
	})
	if err != nil {
		t.Fatalf("CreateFunction: %v", err)
	}
	t.Cleanup(func() {
		_, _ = lam.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})
	})

	got, err := lam.GetFunction(ctx, &lambda.GetFunctionInput{FunctionName: aws.String(fnName)})
	if err != nil {
		t.Fatalf("GetFunction: %v", err)
	}
	if got.Configuration == nil || got.Configuration.FunctionName == nil || *got.Configuration.FunctionName != fnName {
		t.Fatalf("GetFunction unexpected: %+v", got.Configuration)
	}

	list, err := lam.ListFunctions(ctx, &lambda.ListFunctionsInput{})
	if err != nil {
		t.Fatalf("ListFunctions: %v", err)
	}
	found := false
	for _, fn := range list.Functions {
		if fn.FunctionName != nil && *fn.FunctionName == fnName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListFunctions missing %s", fnName)
	}

	// Invoke requires nested DinD; CRUD is enough for this suite.
	_, err = lam.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})
	if err != nil {
		t.Fatalf("DeleteFunction: %v", err)
	}
}

func minimalPythonZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("index.py")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write([]byte("def handler(event, context):\n    return {'ok': True}\n")); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
