package sdk_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/appsync"
	appsynctypes "github.com/aws/aws-sdk-go-v2/service/appsync/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	cttypes "github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lamtypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	sfntypes "github.com/aws/aws-sdk-go-v2/service/sfn/types"
)

func newSFN(t *testing.T, cfg aws.Config) *sfn.Client {
	t.Helper()
	return sfn.NewFromConfig(cfg, func(o *sfn.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newCloudTrail(t *testing.T, cfg aws.Config) *cloudtrail.Client {
	t.Helper()
	return cloudtrail.NewFromConfig(cfg, func(o *cloudtrail.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newAppSync(t *testing.T, cfg aws.Config) *appsync.Client {
	t.Helper()
	return appsync.NewFromConfig(cfg, func(o *appsync.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func TestSFNChoiceStartExecutionSucceeded(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newSFN(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	name := prefix + "-choice"
	if len(name) > 64 {
		name = name[:64]
	}
	definition := `{
  "StartAt":"Pick",
  "States":{
    "Pick":{"Type":"Choice","Choices":[{"Variable":"$.color","StringEquals":"red","Next":"Red"}],"Default":"Other"},
    "Red":{"Type":"Pass","Result":{"branch":"red"},"End":true},
    "Other":{"Type":"Pass","Result":{"branch":"other"},"End":true}
  }
}`
	sm, err := client.CreateStateMachine(ctx, &sfn.CreateStateMachineInput{
		Name: aws.String(name), Definition: aws.String(definition),
	})
	if err != nil {
		t.Fatalf("CreateStateMachine: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteStateMachine(ctx, &sfn.DeleteStateMachineInput{StateMachineArn: sm.StateMachineArn})
	})
	started, err := client.StartExecution(ctx, &sfn.StartExecutionInput{
		StateMachineArn: sm.StateMachineArn,
		Name:            aws.String(prefix + "-run"),
		Input:           aws.String(`{"color":"red"}`),
	})
	if err != nil {
		t.Fatalf("StartExecution: %v", err)
	}
	desc, err := client.DescribeExecution(ctx, &sfn.DescribeExecutionInput{ExecutionArn: started.ExecutionArn})
	if err != nil {
		t.Fatalf("DescribeExecution: %v", err)
	}
	if desc.Status != sfntypes.ExecutionStatusSucceeded {
		t.Fatalf("status=%v want SUCCEEDED", desc.Status)
	}
	if !strings.Contains(aws.ToString(desc.Output), `"branch":"red"`) &&
		!strings.Contains(aws.ToString(desc.Output), `"branch": "red"`) {
		t.Fatalf("output=%q", aws.ToString(desc.Output))
	}
}

func TestCloudTrailCreateTrailStartLoggingS3(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	s3c := newS3(t, cfg)
	ct := newCloudTrail(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	bucket := strings.ToLower(prefix + "-ct")
	trail := prefix + "-trail"
	if len(trail) > 64 {
		trail = trail[:64]
	}

	if _, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		listed, _ := s3c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String("AWSLogs/")})
		if listed != nil {
			for _, obj := range listed.Contents {
				_, _ = s3c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: obj.Key})
			}
		}
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
		_, _ = ct.DeleteTrail(ctx, &cloudtrail.DeleteTrailInput{Name: aws.String(trail)})
	})

	if _, err := ct.CreateTrail(ctx, &cloudtrail.CreateTrailInput{
		Name: aws.String(trail), S3BucketName: aws.String(bucket),
	}); err != nil {
		t.Fatalf("CreateTrail: %v", err)
	}
	if _, err := ct.StartLogging(ctx, &cloudtrail.StartLoggingInput{Name: aws.String(trail)}); err != nil {
		t.Fatalf("StartLogging: %v", err)
	}
	listed, err := s3c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String("AWSLogs/")})
	if err != nil {
		t.Fatalf("ListObjectsV2: %v", err)
	}
	found := false
	needle := "noctaxris-" + trail + "-"
	for _, obj := range listed.Contents {
		if obj.Key != nil && strings.Contains(*obj.Key, "CloudTrail/") && strings.Contains(*obj.Key, needle) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing CloudTrail delivery object: %+v", listed.Contents)
	}
}

func TestCloudTrailPutGetEventSelectors(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	s3c := newS3(t, cfg)
	ct := newCloudTrail(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	bucket := strings.ToLower(prefix + "-ctsel")
	trail := prefix + "-sel"
	if len(trail) > 64 {
		trail = trail[:64]
	}

	if _, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		listed, _ := s3c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
		if listed != nil {
			for _, obj := range listed.Contents {
				_, _ = s3c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: obj.Key})
			}
		}
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
		_, _ = ct.DeleteTrail(ctx, &cloudtrail.DeleteTrailInput{Name: aws.String(trail)})
	})

	if _, err := ct.CreateTrail(ctx, &cloudtrail.CreateTrailInput{
		Name: aws.String(trail), S3BucketName: aws.String(bucket),
	}); err != nil {
		t.Fatalf("CreateTrail: %v", err)
	}

	putStatus, putBody, _ := signedJSONTarget(t, "cloudtrail", "CloudTrail_20131101.PutEventSelectors", map[string]any{
		"TrailName": trail,
		"EventSelectors": []map[string]any{
			{
				"ReadWriteType":           "All",
				"IncludeManagementEvents": true,
				"DataResources": []map[string]any{
					{"Type": "AWS::S3::Object", "Values": []string{"arn:aws:s3:::"}},
				},
			},
		},
	})
	if putStatus != 200 {
		t.Fatalf("PutEventSelectors status=%d body=%s", putStatus, putBody)
	}

	getStatus, getBody, getParsed := signedJSONTarget(t, "cloudtrail", "CloudTrail_20131101.GetEventSelectors", map[string]any{
		"TrailName": trail,
	})
	if getStatus != 200 {
		t.Fatalf("GetEventSelectors status=%d body=%s", getStatus, getBody)
	}
	selectors, _ := getParsed["EventSelectors"].([]any)
	if len(selectors) != 1 {
		t.Fatalf("EventSelectors=%v body=%s", getParsed["EventSelectors"], getBody)
	}
	first, _ := selectors[0].(map[string]any)
	resources, _ := first["DataResources"].([]any)
	if len(resources) == 0 {
		t.Fatalf("expected S3 DataResources in %+v", first)
	}
}

func TestCloudTrailInjectInsightsEventsLookup(t *testing.T) {
	requireReady(t)
	if os.Getenv("NOCTAXRIS_CLOUDTRAIL_INJECT") != "1" {
		t.Skip("set NOCTAXRIS_CLOUDTRAIL_INJECT=1 on the API process for lab InjectInsightsEvents")
	}
	prefix := uniquePrefix(t)
	eventID := "sdk-insight-" + prefix

	injStatus, injBody, _ := signedJSONTarget(t, "cloudtrail", "NoctaxrisCloudTrail.InjectInsightsEvents", map[string]any{
		"Event": map[string]any{
			"eventTime": "2026-07-20T12:05:00Z",
			"eventID":   eventID,
			"insightDetails": map[string]any{
				"state":               "Start",
				"eventSource":         "sts.amazonaws.com",
				"eventName":           "AssumeRole",
				"insightType":         "ApiCallRateInsight",
				"sourceEventCategory": "Management",
				"insightContext": map[string]any{
					"statistics": map[string]any{
						"baseline":         map[string]any{"average": 0.1},
						"insight":          map[string]any{"average": 12.0},
						"insightDuration":  5,
						"baselineDuration": 1000,
					},
				},
			},
		},
	})
	if injStatus != 200 {
		t.Fatalf("InjectInsightsEvents status=%d body=%s", injStatus, injBody)
	}

	lookupStatus, lookupBody, lookupParsed := signedJSONTarget(t, "cloudtrail", "CloudTrail_20131101.LookupEvents", map[string]any{
		"EventCategory": "insight",
		"MaxResults":    50,
	})
	if lookupStatus != 200 {
		t.Fatalf("LookupEvents insight status=%d body=%s", lookupStatus, lookupBody)
	}
	raw := string(lookupBody)
	if !strings.Contains(raw, eventID) {
		t.Fatalf("insight LookupEvents missing %s: %s parsed=%v", eventID, lookupBody, lookupParsed)
	}
}

func TestCloudTrailInjectEventsLookupAndValidateLogs(t *testing.T) {
	requireReady(t)
	if os.Getenv("NOCTAXRIS_CLOUDTRAIL_INJECT") != "1" {
		t.Skip("set NOCTAXRIS_CLOUDTRAIL_INJECT=1 on the API process for lab InjectEvents")
	}
	cfg := loadAWSConfig(t)
	s3c := newS3(t, cfg)
	ct := newCloudTrail(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	bucket := strings.ToLower(prefix + "-ctinj")
	trail := prefix + "-inj"
	if len(trail) > 64 {
		trail = trail[:64]
	}
	eventID := "sdk-inj-" + prefix
	sourceIP := "198.51.100.44"

	injStatus, injBody, _ := signedJSONTarget(t, "cloudtrail", "NoctaxrisCloudTrail.InjectEvents", map[string]any{
		"Events": []map[string]any{{
			"eventTime":       "2026-07-20T15:00:00Z",
			"sourceIPAddress": sourceIP,
			"userIdentity":    map[string]any{"type": "IAMUser", "userName": "sdk-forensic"},
			"eventSource":     "signin.amazonaws.com",
			"eventName":       "ConsoleLogin",
			"eventID":         eventID,
			"readOnly":        false,
		}},
	})
	if injStatus != 200 {
		t.Fatalf("InjectEvents status=%d body=%s", injStatus, injBody)
	}

	byIP, err := ct.LookupEvents(ctx, &cloudtrail.LookupEventsInput{
		LookupAttributes: []cttypes.LookupAttribute{{
			AttributeKey:   cttypes.LookupAttributeKey("SourceIPAddress"),
			AttributeValue: aws.String(sourceIP),
		}},
		MaxResults: aws.Int32(10),
	})
	if err != nil {
		t.Fatalf("LookupEvents SourceIPAddress: %v", err)
	}
	if !cloudTrailLookupContains(byIP, eventID) {
		t.Fatalf("LookupEvents SourceIPAddress missing %s: %+v", eventID, byIP.Events)
	}

	byName, err := ct.LookupEvents(ctx, &cloudtrail.LookupEventsInput{
		LookupAttributes: []cttypes.LookupAttribute{{
			AttributeKey:   cttypes.LookupAttributeKeyEventName,
			AttributeValue: aws.String("ConsoleLogin"),
		}},
		MaxResults: aws.Int32(10),
	})
	if err != nil {
		t.Fatalf("LookupEvents EventName: %v", err)
	}
	if !cloudTrailLookupContains(byName, eventID) {
		t.Fatalf("LookupEvents EventName missing %s: %+v", eventID, byName.Events)
	}

	if _, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		listed, _ := s3c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String("AWSLogs/")})
		if listed != nil {
			for _, obj := range listed.Contents {
				_, _ = s3c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: obj.Key})
			}
		}
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
		_, _ = ct.DeleteTrail(ctx, &cloudtrail.DeleteTrailInput{Name: aws.String(trail)})
	})

	if _, err := ct.CreateTrail(ctx, &cloudtrail.CreateTrailInput{
		Name: aws.String(trail), S3BucketName: aws.String(bucket),
	}); err != nil {
		t.Fatalf("CreateTrail: %v", err)
	}
	if _, err := ct.StartLogging(ctx, &cloudtrail.StartLoggingInput{Name: aws.String(trail)}); err != nil {
		t.Fatalf("StartLogging: %v", err)
	}
	listed, err := s3c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String("AWSLogs/")})
	if err != nil {
		t.Fatalf("ListObjectsV2: %v", err)
	}
	var logKey string
	for _, obj := range listed.Contents {
		if obj.Key == nil {
			continue
		}
		k := *obj.Key
		if strings.Contains(k, "/CloudTrail/") && !strings.Contains(k, "CloudTrail-Digest") {
			logKey = k
			break
		}
	}
	if logKey == "" {
		t.Fatalf("missing CloudTrail log object: %+v", listed.Contents)
	}

	valStatus, valBody, valParsed := signedJSONTarget(t, "cloudtrail", "CloudTrail_20131101.ValidateLogs", map[string]any{
		"S3BucketName": bucket,
		"S3ObjectKey":  logKey,
	})
	if valStatus != 200 {
		t.Fatalf("ValidateLogs status=%d body=%s", valStatus, valBody)
	}
	if valid, _ := valParsed["Valid"].(bool); !valid {
		t.Fatalf("ValidateLogs Valid=%v body=%s", valParsed["Valid"], valBody)
	}
}

func cloudTrailLookupContains(out *cloudtrail.LookupEventsOutput, needle string) bool {
	if out == nil {
		return false
	}
	for _, ev := range out.Events {
		if ev.EventId != nil && *ev.EventId == needle {
			return true
		}
		if ev.CloudTrailEvent != nil && strings.Contains(*ev.CloudTrailEvent, needle) {
			return true
		}
		raw, _ := json.Marshal(ev)
		if strings.Contains(string(raw), needle) {
			return true
		}
	}
	return false
}

func TestELBv2CreateRulePathPattern(t *testing.T) {
	requireReady(t)
	prefix := uniquePrefix(t)
	lbName := "lb" + strings.ReplaceAll(prefix, "-", "")
	if len(lbName) > 32 {
		lbName = lbName[:32]
	}

	lbStatus, lbBody, lbParsed := signedJSONTarget(t, "elasticloadbalancing", "ElasticLoadBalancing_v2.CreateLoadBalancer", map[string]any{
		"Name": lbName,
	})
	if lbStatus != 200 {
		t.Fatalf("CreateLoadBalancer status=%d body=%s", lbStatus, lbBody)
	}
	lbs, _ := lbParsed["LoadBalancers"].([]any)
	if len(lbs) == 0 {
		if arn, ok := lbParsed["LoadBalancerArn"].(string); ok {
			lbs = []any{map[string]any{"LoadBalancerArn": arn}}
		}
	}
	if len(lbs) == 0 {
		t.Fatalf("missing LB: %s", lbBody)
	}
	lb0, _ := lbs[0].(map[string]any)
	lbARN, _ := lb0["LoadBalancerArn"].(string)
	if lbARN == "" {
		t.Fatalf("missing LoadBalancerArn: %s", lbBody)
	}

	tgAStatus, tgABody, tgAParsed := signedJSONTarget(t, "elasticloadbalancing", "ElasticLoadBalancing_v2.CreateTargetGroup", map[string]any{
		"Name": lbName + "a", "TargetType": "lambda",
	})
	if tgAStatus != 200 {
		t.Fatalf("CreateTargetGroup A status=%d body=%s", tgAStatus, tgABody)
	}
	tgBStatus, tgBBody, tgBParsed := signedJSONTarget(t, "elasticloadbalancing", "ElasticLoadBalancing_v2.CreateTargetGroup", map[string]any{
		"Name": lbName + "b", "TargetType": "lambda",
	})
	if tgBStatus != 200 {
		t.Fatalf("CreateTargetGroup B status=%d body=%s", tgBStatus, tgBBody)
	}
	tgARN := func(parsed map[string]any, raw []byte) string {
		if tgs, ok := parsed["TargetGroups"].([]any); ok && len(tgs) > 0 {
			if m, ok := tgs[0].(map[string]any); ok {
				if a, _ := m["TargetGroupArn"].(string); a != "" {
					return a
				}
			}
		}
		if a, _ := parsed["TargetGroupArn"].(string); a != "" {
			return a
		}
		t.Fatalf("missing TargetGroupArn: %s", raw)
		return ""
	}
	tgA := tgARN(tgAParsed, tgABody)
	tgB := tgARN(tgBParsed, tgBBody)

	lisStatus, lisBody, lisParsed := signedJSONTarget(t, "elasticloadbalancing", "ElasticLoadBalancing_v2.CreateListener", map[string]any{
		"LoadBalancerArn": lbARN,
		"Protocol":        "HTTP",
		"Port":            80,
		"DefaultActions":  []map[string]any{{"Type": "forward", "TargetGroupArn": tgB}},
	})
	if lisStatus != 200 {
		t.Fatalf("CreateListener status=%d body=%s", lisStatus, lisBody)
	}
	listenerARN := ""
	if ls, ok := lisParsed["Listeners"].([]any); ok && len(ls) > 0 {
		if m, ok := ls[0].(map[string]any); ok {
			listenerARN, _ = m["ListenerArn"].(string)
		}
	}
	if listenerARN == "" {
		listenerARN, _ = lisParsed["ListenerArn"].(string)
	}
	if listenerARN == "" {
		t.Fatalf("missing ListenerArn: %s", lisBody)
	}

	ruleStatus, ruleBody, _ := signedJSONTarget(t, "elasticloadbalancing", "ElasticLoadBalancing_v2.CreateRule", map[string]any{
		"ListenerArn": listenerARN,
		"Priority":    5,
		"Conditions":  []map[string]any{{"Field": "path-pattern", "Values": []string{"/api*"}}},
		"Actions":     []map[string]any{{"Type": "forward", "TargetGroupArn": tgA}},
	})
	if ruleStatus != 200 {
		t.Fatalf("CreateRule status=%d body=%s", ruleStatus, ruleBody)
	}
	descStatus, descBody, descParsed := signedJSONTarget(t, "elasticloadbalancing", "ElasticLoadBalancing_v2.DescribeRules", map[string]any{
		"ListenerArn": listenerARN,
	})
	if descStatus != 200 {
		t.Fatalf("DescribeRules status=%d body=%s", descStatus, descBody)
	}
	if !strings.Contains(string(descBody), "/api*") && !strings.Contains(string(descBody), "path-pattern") {
		t.Fatalf("DescribeRules missing path rule: %s parsed=%v", descBody, descParsed)
	}
}

func TestAppSyncServiceRoleArnMultiResolver(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	iamc := newIAM(t, cfg)
	lam := newLambda(t, cfg)
	as := newAppSync(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	roleName := prefix + "-appsync"
	if len(roleName) > 64 {
		roleName = roleName[:64]
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"appsync.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	role, err := iamc.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName: aws.String(roleName), AssumeRolePolicyDocument: aws.String(trust),
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	t.Cleanup(func() {
		_, _ = iamc.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	})
	if _, err := iamc.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName: aws.String(roleName), PolicyName: aws.String("invoke"),
		PolicyDocument: aws.String(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"lambda:InvokeFunction","Resource":"*"}]}`),
	}); err != nil {
		t.Fatalf("PutRolePolicy: %v", err)
	}

	lambdaTrust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	fnRoleName := prefix + "-fn"
	if len(fnRoleName) > 64 {
		fnRoleName = fnRoleName[:64]
	}
	fnRole, err := iamc.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName: aws.String(fnRoleName), AssumeRolePolicyDocument: aws.String(lambdaTrust),
	})
	if err != nil {
		t.Fatalf("CreateFunction role: %v", err)
	}
	t.Cleanup(func() {
		_, _ = iamc.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(fnRoleName)})
	})

	fnName := prefix + "-hello"
	zip := minimalPythonZip(t)
	_, err = lam.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Runtime:      lamtypes.RuntimePython312,
		Role:         fnRole.Role.Arn,
		Handler:      aws.String("index.handler"),
		Code:         &lamtypes.FunctionCode{ZipFile: zip},
	})
	if err != nil {
		t.Fatalf("CreateFunction: %v", err)
	}
	t.Cleanup(func() {
		_, _ = lam.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})
	})
	fn, err := lam.GetFunction(ctx, &lambda.GetFunctionInput{FunctionName: aws.String(fnName)})
	if err != nil {
		t.Fatalf("GetFunction: %v", err)
	}
	fnARN := aws.ToString(fn.Configuration.FunctionArn)

	api, err := as.CreateGraphqlApi(ctx, &appsync.CreateGraphqlApiInput{
		Name:                     aws.String(prefix + "-api"),
		AuthenticationType:       appsynctypes.AuthenticationTypeApiKey,
	})
	if err != nil {
		t.Fatalf("CreateGraphqlApi: %v", err)
	}
	apiID := aws.ToString(api.GraphqlApi.ApiId)
	t.Cleanup(func() {
		_, _ = as.DeleteGraphqlApi(ctx, &appsync.DeleteGraphqlApiInput{ApiId: aws.String(apiID)})
	})
	if _, err := as.StartSchemaCreation(ctx, &appsync.StartSchemaCreationInput{
		ApiId:      aws.String(apiID),
		Definition: []byte("type Query { hello: String world: String }"),
	}); err != nil {
		t.Fatalf("StartSchemaCreation: %v", err)
	}
	if _, err := as.CreateApiKey(ctx, &appsync.CreateApiKeyInput{ApiId: aws.String(apiID)}); err != nil {
		t.Fatalf("CreateApiKey: %v", err)
	}
	if _, err := as.CreateDataSource(ctx, &appsync.CreateDataSourceInput{
		ApiId:           aws.String(apiID),
		Name:            aws.String("HelloDS"),
		Type:            appsynctypes.DataSourceTypeAwsLambda,
		ServiceRoleArn:  role.Role.Arn,
		LambdaConfig:    &appsynctypes.LambdaDataSourceConfig{LambdaFunctionArn: aws.String(fnARN)},
	}); err != nil {
		t.Fatalf("CreateDataSource: %v", err)
	}
	for _, field := range []string{"hello", "world"} {
		if _, err := as.CreateResolver(ctx, &appsync.CreateResolverInput{
			ApiId: aws.String(apiID), TypeName: aws.String("Query"), FieldName: aws.String(field),
			DataSourceName: aws.String("HelloDS"),
		}); err != nil {
			t.Fatalf("CreateResolver %s: %v", field, err)
		}
	}
}
