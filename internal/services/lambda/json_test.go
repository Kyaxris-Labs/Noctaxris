package lambda_test

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	lambdasvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/lambda"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func sampleFunction() store.LambdaFunction {
	layerARN := "arn:aws:lambda:us-east-1:000000000001:layer:ly:1"
	return store.LambdaFunction{
		AccountID:               "000000000001",
		FunctionName:            "fn",
		FunctionARN:             "arn:aws:lambda:us-east-1:000000000001:function:fn",
		RoleARN:                 "arn:aws:iam::000000000001:role/lambda",
		Runtime:                 "nodejs22.x",
		Handler:                 "index.handler",
		Timeout:                 30,
		Memory:                  128,
		CodeSHA256:              hex.EncodeToString([]byte{1, 2, 3, 4}),
		PackageType:             store.LambdaPackageTypeZip,
		State:                   "Active",
		Description:             "lab",
		LastModified:            "2024-01-01T00:00:00Z",
		Env:                     map[string]string{"K": "V"},
		Layers:                  []string{layerARN},
		LayerCodeSizes:          map[string]int64{layerARN: 42},
		DeadLetterTargetArn:     "arn:aws:sns:us-east-1:000000000001:dlq",
		DestinationOnFailureArn: "arn:aws:sns:us-east-1:000000000001:fail",
		DestinationOnSuccessArn: "arn:aws:sns:us-east-1:000000000001:ok",
	}
}

func TestLambdaFunctionJSON(t *testing.T) {
	fn := sampleFunction()
	createRaw, err := lambdasvc.CreateFunctionJSON(fn)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(createRaw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["PackageType"] != store.LambdaPackageTypeZip {
		t.Fatalf("cfg=%v", cfg)
	}
	wantB64 := base64.StdEncoding.EncodeToString([]byte{1, 2, 3, 4})
	if cfg["CodeSha256"] != wantB64 {
		t.Fatalf("sha=%v", cfg["CodeSha256"])
	}

	getZip, err := lambdasvc.GetFunctionJSON(fn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(getZip), "Configuration") {
		t.Fatalf("get=%s", getZip)
	}

	imgFn := fn
	imgFn.PackageType = store.LambdaPackageTypeImage
	imgFn.ImageURI = "000000000001.dkr.ecr.us-east-1.amazonaws.com/img:latest"
	getImg, err := lambdasvc.GetFunctionJSON(imgFn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(getImg), "ECR") {
		t.Fatalf("image get=%s", getImg)
	}

	listRaw, err := lambdasvc.ListFunctionsJSON([]store.LambdaFunction{fn})
	if err != nil {
		t.Fatal(err)
	}
	invokeRaw, err := lambdasvc.InvokeJSON([]byte("ok"), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	asyncRaw, err := lambdasvc.InvokeAsyncAcceptedJSON("")
	if err != nil {
		t.Fatal(err)
	}
	var invoke map[string]any
	if err := json.Unmarshal(invokeRaw, &invoke); err != nil {
		t.Fatal(err)
	}
	if invoke["StatusCode"] != float64(200) {
		t.Fatalf("invoke=%v", invoke)
	}
	if !strings.Contains(string(asyncRaw), "202") {
		t.Fatalf("async=%s", asyncRaw)
	}

	ver := store.LambdaFunctionVersion{Version: 3, LambdaFunction: fn}
	pubRaw, err := lambdasvc.PublishVersionJSON(ver)
	if err != nil {
		t.Fatal(err)
	}
	listVerRaw, err := lambdasvc.ListVersionsByFunctionJSON(fn, []store.LambdaFunctionVersion{ver})
	if err != nil {
		t.Fatal(err)
	}
	qFn := sampleFunction()
	qFn.PackageType = store.LambdaPackageTypeImage
	qFn.ImageURI = "000000000001.dkr.ecr.us-east-1.amazonaws.com/img:latest"
	q := store.QualifiedFunction{LambdaFunction: qFn, Version: "3"}
	getQ, err := lambdasvc.GetFunctionQualifiedJSON(q)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pubRaw), `"Version":"3"`) && !strings.Contains(string(pubRaw), "3") {
		t.Fatalf("publish=%s", pubRaw)
	}
	if !strings.Contains(string(listVerRaw), "$LATEST") || !strings.Contains(string(getQ), "3") {
		t.Fatalf("versions=%s qualified=%s", listVerRaw, getQ)
	}

	alias := store.LambdaAlias{
		AliasARN:        "arn:aws:lambda:us-east-1:000000000001:function:fn:live",
		AliasName:       "live",
		FunctionVersion: 3,
		Description:     "prod",
		RevisionID:      "rev-1",
	}
	aliasCreate, err := lambdasvc.CreateAliasJSON(alias)
	if err != nil {
		t.Fatal(err)
	}
	aliasGet, err := lambdasvc.GetAliasJSON(alias)
	if err != nil {
		t.Fatal(err)
	}
	aliasList, err := lambdasvc.ListAliasesJSON([]store.LambdaAlias{alias})
	if err != nil {
		t.Fatal(err)
	}
	if string(aliasCreate) != string(aliasGet) || !strings.Contains(string(aliasList), "live") {
		t.Fatalf("alias create=%s list=%s", aliasCreate, aliasList)
	}

	empty, err := lambdasvc.EmptyOKJSON()
	if err != nil || string(empty) != "{}" {
		t.Fatal(err)
	}
	tagsRaw, err := lambdasvc.ListTagsJSON(map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	tagsNil, err := lambdasvc.ListTagsJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tagsRaw), "v") || !strings.Contains(string(tagsNil), "Tags") {
		t.Fatalf("tags=%s nil=%s", tagsRaw, tagsNil)
	}

	perm, err := lambdasvc.AddPermissionJSON(`{"Sid":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	pol, err := lambdasvc.GetPolicyJSON(`{"Version":"2012-10-17"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(perm), "Statement") || !strings.Contains(string(pol), "Policy") {
		t.Fatalf("perm=%s pol=%s", perm, pol)
	}

	layer := store.LambdaLayer{
		LayerARN:    "arn:aws:lambda:us-east-1:000000000001:layer:ly:2",
		Description: "layer",
		PublishedAt: "2024-01-01T00:00:00Z",
		Version:     2,
		CodeSHA256:  hex.EncodeToString([]byte{9}),
	}
	pubLayer, err := lambdasvc.PublishLayerVersionJSON(layer, 100)
	if err != nil {
		t.Fatal(err)
	}
	getLayer, err := lambdasvc.GetLayerVersionJSON(layer, 100)
	if err != nil {
		t.Fatal(err)
	}
	listLayers, err := lambdasvc.ListLayerVersionsJSON([]store.LambdaLayer{layer})
	if err != nil {
		t.Fatal(err)
	}
	if string(pubLayer) != string(getLayer) || !strings.Contains(string(listLayers), "LayerVersions") {
		t.Fatalf("layer pub=%s list=%s", pubLayer, listLayers)
	}

	zipRaw, err := lambdasvc.DecodeZipFile(base64.StdEncoding.EncodeToString([]byte("zip")))
	if err != nil || string(zipRaw) != "zip" {
		t.Fatalf("zip decode=%q err=%v", zipRaw, err)
	}
	zipBytes, err := lambdasvc.DecodeZipFile([]byte("raw"))
	if err != nil || string(zipBytes) != "raw" {
		t.Fatalf("zip bytes=%q err=%v", zipBytes, err)
	}
	if _, err := lambdasvc.DecodeZipFile(""); err == nil {
		t.Fatal("expected empty zip error")
	}
	if _, err := lambdasvc.DecodeZipFile("!!!"); err == nil {
		t.Fatal("expected bad base64 error")
	}
	if _, err := lambdasvc.DecodeZipFile(123); err == nil {
		t.Fatal("expected type error")
	}

	esm := store.LambdaEventSourceMapping{
		UUID:                      "uuid-1",
		FunctionARN:               fn.FunctionARN,
		EventSourceARN:            "arn:aws:sqs:us-east-1:000000000001:q",
		BatchSize:                 10,
		State:                     "Enabled",
		LastModified:              "2024-01-01T00:00:00Z",
		Qualifier:                 "live",
		FilterCriteriaJSON:        `{"Filters":[{"Pattern":"{}"}]}`,
		FunctionResponseTypesJSON: `["ReportBatchItemFailures"]`,
	}
	esmRaw, err := lambdasvc.EventSourceMappingJSON(esm)
	if err != nil {
		t.Fatal(err)
	}
	esmList, err := lambdasvc.ListEventSourceMappingsJSON([]store.LambdaEventSourceMapping{esm})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(esmRaw), "FilterCriteria") || !strings.Contains(string(esmList), "EventSourceMappings") {
		t.Fatalf("esm=%s list=%s", esmRaw, esmList)
	}

	evCfg, err := lambdasvc.EventInvokeConfigJSON(fn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(evCfg), "DestinationConfig") {
		t.Fatalf("ev=%s", evCfg)
	}

	urlCfg := store.LambdaFunctionURL{
		FunctionURL:      "https://abc.lambda-url.us-east-1.on.aws/",
		FunctionARN:      fn.FunctionARN,
		AuthType:         "AWS_IAM",
		CreationTime:     "2024-01-01T00:00:00Z",
		CorsAllowOrigins: []string{"https://example.com"},
	}
	urlRaw, err := lambdasvc.FunctionURLConfigJSON(urlCfg)
	if err != nil {
		t.Fatal(err)
	}
	urlList, err := lambdasvc.ListFunctionURLConfigsJSON([]store.LambdaFunctionURL{urlCfg})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(urlRaw), "Cors") || !strings.Contains(string(urlList), "FunctionUrlConfigs") {
		t.Fatalf("url=%s list=%s", urlRaw, urlList)
	}

	// Default package type and invalid hex CodeSha256 passthrough.
	minFn := store.LambdaFunction{FunctionName: "min", CodeSHA256: "not-hex"}
	minRaw, err := lambdasvc.CreateFunctionJSON(minFn)
	if err != nil {
		t.Fatal(err)
	}
	var minOut map[string]any
	if err := json.Unmarshal(minRaw, &minOut); err != nil {
		t.Fatal(err)
	}
	if minOut["PackageType"] != store.LambdaPackageTypeZip || minOut["CodeSha256"] != "not-hex" {
		t.Fatalf("min=%v", minOut)
	}
	_ = listRaw
}
