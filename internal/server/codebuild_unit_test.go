package server

import (
	"testing"
)

func TestAnyToBool(t *testing.T) {
	t.Parallel()
	if !anyToBool("true", false) || anyToBool("false", true) {
		t.Fatal("strings")
	}
	if !anyToBool(float64(1), false) {
		t.Fatal("float")
	}
	if anyToBool(nil, false) {
		t.Fatal("nil default false")
	}
	if !anyToBool(nil, true) {
		t.Fatal("nil default true")
	}
}

func TestParseCodeBuildEnvVars(t *testing.T) {
	t.Parallel()
	vars := parseCodeBuildEnvVars([]any{
		map[string]any{"name": "A", "value": "1", "type": "PLAINTEXT"},
		map[string]any{"name": "", "value": "x"},
		"skip",
	})
	if len(vars) != 1 || vars[0].Name != "A" || vars[0].Value != "1" {
		t.Fatalf("%#v", vars)
	}
	if len(parseCodeBuildEnvVars(nil)) != 0 {
		t.Fatal("nil")
	}
}

func TestParseCodeBuildAllowOverride(t *testing.T) {
	t.Parallel()
	src := map[string]any{"allowOverride": "true"}
	if parseCodeBuildAllowOverride(src, nil) == nil || !*parseCodeBuildAllowOverride(src, nil) {
		t.Fatal("source true")
	}
	params := map[string]any{"overrideAllowed": false}
	if parseCodeBuildAllowOverride(nil, params) == nil || *parseCodeBuildAllowOverride(nil, params) {
		t.Fatal("params false")
	}
	if parseCodeBuildAllowOverride(nil, nil) != nil {
		t.Fatal("nil")
	}
}

func TestValidateCodeBuildConfigStubs(t *testing.T) {
	t.Parallel()
	if err := validateCodeBuildConfigStubs(nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := validateCodeBuildConfigStubs(map[string]any{"vpcId": "vpc-1", "subnets": []any{"subnet-1"}}, nil, nil, nil); err != nil {
		t.Fatalf("valid vpc: %v", err)
	}
	if err := validateCodeBuildConfigStubs(map[string]any{"vpcId": ""}, nil, nil, nil); err == nil {
		t.Fatal("vpc missing id")
	}
	if err := validateCodeBuildConfigStubs(nil, map[string]any{"type": "NOPE"}, nil, nil); err == nil {
		t.Fatal("cache type")
	}
	if err := validateCodeBuildConfigStubs(nil, nil, map[string]any{"fleetArn": "arn:aws:codebuild:us-east-1:1:fleet/x"}, nil); err != nil {
		t.Fatalf("valid fleet: %v", err)
	}
}

func TestDecodeCodeBuildStubHelpers(t *testing.T) {
	t.Parallel()
	if decodeCodeBuildStubObject(`{"a":1}`)["a"] == nil {
		t.Fatal("object")
	}
	if len(decodeCodeBuildStubArray(`[1,2]`)) != 2 {
		t.Fatal("array")
	}
	if len(decodeCodeBuildStubStringSlice(`["x"]`)) != 1 {
		t.Fatal("strings")
	}
}

func TestCodebuildAction(t *testing.T) {
	t.Parallel()
	if codebuildAction("StartBuild") == "" {
		t.Fatal("StartBuild")
	}
	if codebuildAction("codebuild:CreateProject") != "codebuild:CreateProject" {
		t.Fatal("passthrough")
	}
}

func TestDecodeCodeBuildEnvVarsJSON(t *testing.T) {
	t.Parallel()
	if decodeCodeBuildEnvVarsJSON("") != nil || decodeCodeBuildEnvVarsJSON("null") != nil {
		t.Fatal("empty/null")
	}
	got := decodeCodeBuildEnvVarsJSON(`[{"name":"A","value":"1","type":"PLAINTEXT"}]`)
	if len(got) != 1 || got[0].Name != "A" {
		t.Fatalf("%#v", got)
	}
	if decodeCodeBuildEnvVarsJSON("{") != nil {
		t.Fatal("bad json")
	}
}

func TestParseCodeBuildConfigStubs(t *testing.T) {
	t.Parallel()
	vpc, cache, fleet, secondary, reports := parseCodeBuildConfigStubs(map[string]any{
		"vpcConfig":         map[string]any{"vpcId": "vpc-1"},
		"cache":             map[string]any{"type": "NO_CACHE"},
		"fleet":             map[string]any{"fleetArn": "arn:aws:codebuild:us-east-1:1:fleet/x"},
		"secondarySources":  []any{map[string]any{"type": "S3"}},
		"reportGroupArns":   []any{"arn:aws:codebuild:us-east-1:1:report-group/x"},
	})
	if vpc["vpcId"] != "vpc-1" || cache["type"] != "NO_CACHE" || fleet["fleetArn"] == nil {
		t.Fatalf("vpc=%v cache=%v fleet=%v", vpc, cache, fleet)
	}
	if len(secondary) != 1 || len(reports) != 1 {
		t.Fatalf("secondary=%v reports=%v", secondary, reports)
	}
	vpc2, _, fleet2, _, _ := parseCodeBuildConfigStubs(map[string]any{
		"projectFleet": map[string]any{"fleetArn": "arn:x"},
	})
	if vpc2 != nil || fleet2["fleetArn"] != "arn:x" {
		t.Fatalf("projectFleet %v %v", vpc2, fleet2)
	}
	a, b, c, d, e := parseCodeBuildConfigStubs(nil)
	if a != nil || b != nil || c != nil || d != nil || e != nil {
		t.Fatal("nil params")
	}
}
