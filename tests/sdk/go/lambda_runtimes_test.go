package sdk_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

// Newer LTS ids may not yet have named constants in the pinned SDK module;
// cast keeps CreateFunction CRUD coverage without forcing a module bump.
const (
	runtimePython314 = types.Runtime("python3.14")
	runtimeNodejs24x = types.Runtime("nodejs24.x")
	runtimeJava25    = types.Runtime("java25")
)

func TestLambdaCreateGetDeleteRuntimes(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	iamClient := newIAM(t, cfg)
	lam := newLambda(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	roleName := prefix + "-lambda-rt-role"
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleOut, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trust),
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	roleARN := *roleOut.Role.Arn
	t.Cleanup(func() {
		_, _ = iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	})

	cases := []struct {
		suffix  string
		runtime types.Runtime
		handler string
		zip     []byte
	}{
		{"py311", types.RuntimePython311, "index.handler", minimalPythonZip(t)},
		{"py312", types.RuntimePython312, "index.handler", minimalPythonZip(t)},
		{"py313", types.RuntimePython313, "index.handler", minimalPythonZip(t)},
		{"py314", runtimePython314, "index.handler", minimalPythonZip(t)},
		{"node20", types.RuntimeNodejs20x, "index.handler", minimalNodeZip(t)},
		{"node22", types.RuntimeNodejs22x, "index.handler", minimalNodeZip(t)},
		{"node24", runtimeNodejs24x, "index.handler", minimalNodeZip(t)},
		{"java21", types.RuntimeJava21, "example.Handler::handleRequest", minimalJavaZip(t)},
		{"java25", runtimeJava25, "example.Handler::handleRequest", minimalJavaZip(t)},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.runtime), func(t *testing.T) {
			fnName := fmt.Sprintf("%s-%s", prefix, tc.suffix)
			_, err := lam.CreateFunction(ctx, &lambda.CreateFunctionInput{
				FunctionName: aws.String(fnName),
				Runtime:      tc.runtime,
				Role:         aws.String(roleARN),
				Handler:      aws.String(tc.handler),
				Code:         &types.FunctionCode{ZipFile: tc.zip},
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
			if got.Configuration == nil || got.Configuration.Runtime != tc.runtime {
				t.Fatalf("GetFunction Runtime=%v want %s", got.Configuration, tc.runtime)
			}
			if got.Configuration.Handler == nil || *got.Configuration.Handler != tc.handler {
				t.Fatalf("GetFunction Handler=%v want %s", got.Configuration.Handler, tc.handler)
			}

			_, err = lam.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})
			if err != nil {
				t.Fatalf("DeleteFunction: %v", err)
			}
		})
	}
}

func TestLambdaCreateFunctionRejectsUnsupportedRuntime(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	iamClient := newIAM(t, cfg)
	lam := newLambda(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	roleName := prefix + "-lambda-bad-rt"
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleOut, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trust),
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	roleARN := *roleOut.Role.Arn
	t.Cleanup(func() {
		_, _ = iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	})

	_, err = lam.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(prefix + "-bad"),
		Runtime:      types.Runtime("ruby3.4"),
		Role:         aws.String(roleARN),
		Handler:      aws.String("index.handler"),
		Code:         &types.FunctionCode{ZipFile: minimalPythonZip(t)},
	})
	if err == nil {
		t.Fatal("CreateFunction with ruby3.4: want error")
	}
}

func minimalNodeZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("index.js")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write([]byte("exports.handler = async () => ({ ok: true });\n")); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func minimalJavaZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("example/Handler.java")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	src := `package example;
public class Handler {
  public static String handleRequest(String in) { return "{\"ok\":true}"; }
}
`
	if _, err := w.Write([]byte(src)); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
