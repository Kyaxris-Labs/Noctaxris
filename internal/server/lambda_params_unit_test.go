package server

import (
	"strings"
	"testing"
)

func TestFunctionQualifierParam(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{"empty defaults latest", map[string]any{}, "$LATEST"},
		{"qualifier only", map[string]any{"Qualifier": "live"}, "live"},
		{"name with version", map[string]any{"FunctionName": "fn:1"}, "1"},
		{"explicit qualifier wins", map[string]any{"FunctionName": "fn:1", "Qualifier": "live"}, "live"},
		{"bare name latest", map[string]any{"FunctionName": "fn"}, "$LATEST"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := functionQualifierParam(tc.in); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestFunctionNameAndQualifier(t *testing.T) {
	t.Parallel()
	name, qual := functionNameAndQualifier(map[string]any{})
	if name != "" || qual != "$LATEST" {
		t.Fatalf("empty -> %q %q", name, qual)
	}
	name, qual = functionNameAndQualifier(map[string]any{"FunctionName": "fn:alias", "Qualifier": "v2"})
	if name != "fn" || qual != "v2" {
		t.Fatalf("got %q %q", name, qual)
	}
}

func TestLambdaEnvFromParams(t *testing.T) {
	t.Parallel()
	if got := lambdaEnvFromParams(nil); len(got) != 0 {
		t.Fatalf("nil -> %v", got)
	}
	got := lambdaEnvFromParams(map[string]any{
		"Environment": map[string]any{
			"Variables": map[string]any{"A": "1", "B": 2},
		},
	})
	if got["A"] != "1" || got["B"] != "" {
		t.Fatalf("got %#v", got)
	}
}

func TestLambdaDestinationsFromParams(t *testing.T) {
	t.Parallel()
	dlq, onFail, onOK := lambdaDestinationsFromParams(map[string]any{
		"DeadLetterConfig": map[string]any{"TargetArn": " arn:aws:sqs:us-east-1:1:q "},
		"DestinationConfig": map[string]any{
			"OnFailure": map[string]any{"Destination": "arn:aws:sns:us-east-1:1:fail"},
			"OnSuccess": " arn:aws:sns:us-east-1:1:ok ",
		},
	})
	if !strings.HasPrefix(dlq, "arn:aws:sqs:") || onFail == "" || onOK == "" {
		t.Fatalf("dlq=%q fail=%q ok=%q", dlq, onFail, onOK)
	}
	dlq2, fail2 := lambdaDLQFromParams(map[string]any{
		"DeadLetterConfig": map[string]any{"TargetArn": "arn:aws:sqs:us-east-1:1:q"},
	})
	if dlq2 == "" || fail2 != "" {
		t.Fatalf("dlq helper %q %q", dlq2, fail2)
	}
	if destinationARNFromConfig(nil) != "" {
		t.Fatal("nil dest")
	}
	if destinationARNFromConfig(42) != "" {
		t.Fatal("bad type dest")
	}
}

func TestLambdaLayersFromParams(t *testing.T) {
	t.Parallel()
	got := lambdaLayersFromParams(map[string]any{
		"Layers": []any{"arn:aws:lambda:us-east-1:1:layer:a:1", 9, "  "},
	})
	if len(got) != 1 || !strings.Contains(got[0], ":layer:a:1") {
		t.Fatalf("got %#v", got)
	}
}

func TestLambdaVpcConfigHelpers(t *testing.T) {
	t.Parallel()
	msg := lambdaVpcConfigRejectMessage()
	if !strings.Contains(msg, "VpcConfig") {
		t.Fatalf("msg=%q", msg)
	}
	if lambdaHasNonEmptyVpcConfig(nil) {
		t.Fatal("nil")
	}
	if lambdaHasNonEmptyVpcConfig(map[string]any{"VpcConfig": map[string]any{}}) {
		t.Fatal("empty object still non-empty per len>0? empty map len 0 should be false")
	}
	if !lambdaHasNonEmptyVpcConfig(map[string]any{
		"VpcConfig": map[string]any{"SubnetIds": []any{"subnet-1"}},
	}) {
		t.Fatal("subnet")
	}
	if !lambdaHasNonEmptyVpcConfig(map[string]any{
		"VpcConfig": map[string]any{"SecurityGroupIds": []any{"sg-1"}},
	}) {
		t.Fatal("sg")
	}
	if !lambdaHasNonEmptyVpcConfig(map[string]any{
		"VpcConfig": map[string]any{"Ipv6AllowedForDualStack": true},
	}) {
		t.Fatal("other keys")
	}
}
