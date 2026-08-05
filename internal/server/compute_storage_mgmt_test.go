package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLambdaListDeleteAliasEventInvokeConfig(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-mgmt-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-mgmt-role"
	fnName := "mgmt-fn"
	create := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": fnName,
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", create.Code, create.Body.String())
	}

	list := mustLambdaJSON(t, handler, "ListFunctions", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), fnName) {
		t.Fatalf("ListFunctions status=%d body=%q", list.Code, list.Body.String())
	}

	pub := mustLambdaJSON(t, handler, "PublishVersion", map[string]any{"FunctionName": fnName}, now)
	if pub.Code != http.StatusOK {
		t.Fatalf("PublishVersion status=%d body=%q", pub.Code, pub.Body.String())
	}
	alias := mustLambdaJSON(t, handler, "CreateAlias", map[string]any{
		"FunctionName": fnName, "Name": "live", "FunctionVersion": "1",
	}, now)
	if alias.Code != http.StatusOK {
		t.Fatalf("CreateAlias status=%d body=%q", alias.Code, alias.Body.String())
	}
	getAlias := mustLambdaJSON(t, handler, "GetAlias", map[string]any{
		"FunctionName": fnName, "Name": "live",
	}, now)
	if getAlias.Code != http.StatusOK || !strings.Contains(getAlias.Body.String(), "live") {
		t.Fatalf("GetAlias status=%d body=%q", getAlias.Code, getAlias.Body.String())
	}
	listAliases := mustLambdaJSON(t, handler, "ListAliases", map[string]any{"FunctionName": fnName}, now)
	if listAliases.Code != http.StatusOK || !strings.Contains(listAliases.Body.String(), "live") {
		t.Fatalf("ListAliases status=%d body=%q", listAliases.Code, listAliases.Body.String())
	}
	updAlias := mustLambdaJSON(t, handler, "UpdateAlias", map[string]any{
		"FunctionName": fnName, "Name": "live", "FunctionVersion": "1", "Description": "updated",
	}, now)
	if updAlias.Code != http.StatusOK {
		t.Fatalf("UpdateAlias status=%d body=%q", updAlias.Code, updAlias.Body.String())
	}

	putEIC := mustLambdaJSON(t, handler, "PutFunctionEventInvokeConfig", map[string]any{
		"FunctionName": fnName,
		"DestinationConfig": map[string]any{
			"OnFailure": map[string]any{"Destination": "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":dlq"},
		},
	}, now)
	if putEIC.Code != http.StatusOK {
		t.Fatalf("PutFunctionEventInvokeConfig status=%d body=%q", putEIC.Code, putEIC.Body.String())
	}
	getEIC := mustLambdaJSON(t, handler, "GetFunctionEventInvokeConfig", map[string]any{"FunctionName": fnName}, now)
	if getEIC.Code != http.StatusOK {
		t.Fatalf("GetFunctionEventInvokeConfig status=%d body=%q", getEIC.Code, getEIC.Body.String())
	}
	delEIC := mustLambdaJSON(t, handler, "DeleteFunctionEventInvokeConfig", map[string]any{"FunctionName": fnName}, now)
	if delEIC.Code != http.StatusOK {
		t.Fatalf("DeleteFunctionEventInvokeConfig status=%d body=%q", delEIC.Code, delEIC.Body.String())
	}

	layer := mustLambdaJSON(t, handler, "PublishLayerVersion", map[string]any{
		"LayerName":          "mgmt-layer",
		"Content":            map[string]any{"ZipFile": testLambdaLayerZipB64(t)},
		"CompatibleRuntimes": []string{"python3.12"},
	}, now)
	if layer.Code != http.StatusOK {
		t.Fatalf("PublishLayerVersion status=%d body=%q", layer.Code, layer.Body.String())
	}
	var layerOut map[string]any
	_ = json.Unmarshal(layer.Body.Bytes(), &layerOut)
	ver, _ := layerOut["Version"].(float64)
	delLayer := mustLambdaJSON(t, handler, "DeleteLayerVersion", map[string]any{
		"LayerName": "mgmt-layer", "VersionNumber": int(ver),
	}, now)
	if delLayer.Code != http.StatusOK {
		t.Fatalf("DeleteLayerVersion status=%d body=%q", delLayer.Code, delLayer.Body.String())
	}

	delAlias := mustLambdaJSON(t, handler, "DeleteAlias", map[string]any{
		"FunctionName": fnName, "Name": "live",
	}, now)
	if delAlias.Code != http.StatusOK {
		t.Fatalf("DeleteAlias status=%d body=%q", delAlias.Code, delAlias.Body.String())
	}
	delFn := mustLambdaJSON(t, handler, "DeleteFunction", map[string]any{"FunctionName": fnName}, now)
	if delFn.Code != http.StatusOK {
		t.Fatalf("DeleteFunction status=%d body=%q", delFn.Code, delFn.Body.String())
	}
	gone := mustLambdaJSON(t, handler, "DeleteFunction", map[string]any{"FunctionName": fnName}, now)
	if gone.Code == http.StatusOK {
		t.Fatalf("delete missing function should fail: %q", gone.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "lambda-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "lambda-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"lambda:ListFunctions","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSLambda.ListFunctions")
	signHeader(t, req, raw, denyAK, denySecret, testRegion, "lambda", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}

func TestECRImagePolicyTagMgmt(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustECRJSON(t, handler, "CreateRepository", map[string]any{"repositoryName": "mgmt-repo"}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", create.Code, create.Body.String())
	}

	policy := `{"Version":"2012-10-17","Statement":[{"Sid":"allow","Effect":"Allow","Principal":"*","Action":"ecr:GetDownloadUrlForLayer"}]}`
	setPol := mustECRJSON(t, handler, "SetRepositoryPolicy", map[string]any{
		"repositoryName": "mgmt-repo", "policyText": policy,
	}, now)
	if setPol.Code != http.StatusOK {
		t.Fatalf("SetRepositoryPolicy status=%d body=%q", setPol.Code, setPol.Body.String())
	}
	getPol := mustECRJSON(t, handler, "GetRepositoryPolicy", map[string]any{"repositoryName": "mgmt-repo"}, now)
	if getPol.Code != http.StatusOK || !strings.Contains(getPol.Body.String(), "allow") {
		t.Fatalf("GetRepositoryPolicy status=%d body=%q", getPol.Code, getPol.Body.String())
	}

	digest := "sha256:" + strings.Repeat("ab", 32)
	putImg := mustECRJSON(t, handler, "PutImage", map[string]any{
		"repositoryName": "mgmt-repo",
		"imageManifest":  `{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json"}`,
		"imageId":        map[string]any{"imageDigest": digest, "imageTag": "v1"},
	}, now)
	if putImg.Code != http.StatusOK {
		t.Fatalf("PutImage status=%d body=%q", putImg.Code, putImg.Body.String())
	}
	listImg := mustECRJSON(t, handler, "ListImages", map[string]any{"repositoryName": "mgmt-repo"}, now)
	if listImg.Code != http.StatusOK || !strings.Contains(listImg.Body.String(), digest) && !strings.Contains(listImg.Body.String(), "v1") {
		t.Fatalf("ListImages status=%d body=%q", listImg.Code, listImg.Body.String())
	}
	batchGet := mustECRJSON(t, handler, "BatchGetImage", map[string]any{
		"repositoryName": "mgmt-repo",
		"imageIds":       []map[string]any{{"imageTag": "v1"}},
	}, now)
	if batchGet.Code != http.StatusOK {
		t.Fatalf("BatchGetImage status=%d body=%q", batchGet.Code, batchGet.Body.String())
	}

	repoARN := "arn:aws:ecr:" + testRegion + ":" + testAccountID + ":repository/mgmt-repo"
	tag := mustECRJSON(t, handler, "TagResource", map[string]any{
		"resourceArn": repoARN,
		"tags":        []map[string]any{{"Key": "env", "Value": "lab"}},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagResource status=%d body=%q", tag.Code, tag.Body.String())
	}
	listTags := mustECRJSON(t, handler, "ListTagsForResource", map[string]any{"resourceArn": repoARN}, now)
	if listTags.Code != http.StatusOK {
		t.Fatalf("ListTagsForResource status=%d body=%q", listTags.Code, listTags.Body.String())
	}
	untag := mustECRJSON(t, handler, "UntagResource", map[string]any{
		"resourceArn": repoARN, "tagKeys": []string{"env"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource status=%d body=%q", untag.Code, untag.Body.String())
	}

	batchDel := mustECRJSON(t, handler, "BatchDeleteImage", map[string]any{
		"repositoryName": "mgmt-repo",
		"imageIds":       []map[string]any{{"imageTag": "v1"}},
	}, now)
	if batchDel.Code != http.StatusOK {
		t.Fatalf("BatchDeleteImage status=%d body=%q", batchDel.Code, batchDel.Body.String())
	}
	delPol := mustECRJSON(t, handler, "DeleteRepositoryPolicy", map[string]any{"repositoryName": "mgmt-repo"}, now)
	if delPol.Code != http.StatusOK {
		t.Fatalf("DeleteRepositoryPolicy status=%d body=%q", delPol.Code, delPol.Body.String())
	}
	delRepo := mustECRJSON(t, handler, "DeleteRepository", map[string]any{"repositoryName": "mgmt-repo"}, now)
	if delRepo.Code != http.StatusOK {
		t.Fatalf("DeleteRepository status=%d body=%q", delRepo.Code, delRepo.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "ecr-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "ecr-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"ecr:DeleteRepository","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	mustECRJSON(t, handler, "CreateRepository", map[string]any{"repositoryName": "deny-repo"}, now)
	denyRec := mustECRJSONWithCreds(t, handler, "DeleteRepository", map[string]any{"repositoryName": "deny-repo"}, denyAK, denySecret, now)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}

func TestS3BucketListHeadDeletePolicyEncryptionVersioning(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mgmt-bucket", nil, "s3", now, nil)
	list := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/", nil, "s3", now, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "mgmt-bucket") {
		t.Fatalf("ListBuckets status=%d body=%q", list.Code, list.Body.String())
	}
	head := mustS3(t, handler, http.MethodHead, "http://127.0.0.1:4566/mgmt-bucket", nil, "s3", now, nil)
	if head.Code != http.StatusOK {
		t.Fatalf("HeadBucket status=%d body=%q", head.Code, head.Body.String())
	}

	policy := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"s3:GetObject","Resource":"*"}]}`)
	putPol := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mgmt-bucket?policy", policy, "s3", now, map[string]string{
		"Content-Type": "application/json",
	})
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutBucketPolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}
	getPol := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/mgmt-bucket?policy", nil, "s3", now, nil)
	if getPol.Code != http.StatusOK || !strings.Contains(getPol.Body.String(), "GetObject") {
		t.Fatalf("GetBucketPolicy status=%d body=%q", getPol.Code, getPol.Body.String())
	}
	delPol := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/mgmt-bucket?policy", nil, "s3", now, nil)
	if delPol.Code != http.StatusOK && delPol.Code != http.StatusNoContent {
		t.Fatalf("DeleteBucketPolicy status=%d body=%q", delPol.Code, delPol.Body.String())
	}

	encBody := []byte(`<ServerSideEncryptionConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>AES256</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`)
	putEnc := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mgmt-bucket?encryption", encBody, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if putEnc.Code != http.StatusOK {
		t.Fatalf("PutBucketEncryption status=%d body=%q", putEnc.Code, putEnc.Body.String())
	}
	getEnc := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/mgmt-bucket?encryption", nil, "s3", now, nil)
	if getEnc.Code != http.StatusOK || !strings.Contains(getEnc.Body.String(), "AES256") {
		t.Fatalf("GetBucketEncryption status=%d body=%q", getEnc.Code, getEnc.Body.String())
	}
	delEnc := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/mgmt-bucket?encryption", nil, "s3", now, nil)
	if delEnc.Code != http.StatusOK && delEnc.Code != http.StatusNoContent {
		t.Fatalf("DeleteBucketEncryption status=%d body=%q", delEnc.Code, delEnc.Body.String())
	}

	verBody := []byte(`<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>`)
	putVer := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mgmt-bucket?versioning", verBody, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if putVer.Code != http.StatusOK {
		t.Fatalf("PutBucketVersioning status=%d body=%q", putVer.Code, putVer.Body.String())
	}
	getVer := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/mgmt-bucket?versioning", nil, "s3", now, nil)
	if getVer.Code != http.StatusOK || !strings.Contains(getVer.Body.String(), "Enabled") {
		t.Fatalf("GetBucketVersioning status=%d body=%q", getVer.Code, getVer.Body.String())
	}

	obj := []byte("v1")
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mgmt-bucket/file.txt", obj, "s3", now, map[string]string{"Content-Type": "text/plain"})
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mgmt-bucket/file.txt", []byte("v2"), "s3", now, map[string]string{"Content-Type": "text/plain"})
	listVer := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/mgmt-bucket?versions", nil, "s3", now, nil)
	if listVer.Code != http.StatusOK {
		t.Fatalf("ListObjectVersions status=%d body=%q", listVer.Code, listVer.Body.String())
	}
	listObj := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/mgmt-bucket?list-type=2", nil, "s3", now, nil)
	if listObj.Code != http.StatusOK || !strings.Contains(listObj.Body.String(), "file.txt") {
		t.Fatalf("ListObjectsV2 status=%d body=%q", listObj.Code, listObj.Body.String())
	}
	headObj := mustS3(t, handler, http.MethodHead, "http://127.0.0.1:4566/mgmt-bucket/file.txt", nil, "s3", now, nil)
	if headObj.Code != http.StatusOK {
		t.Fatalf("HeadObject status=%d", headObj.Code)
	}

	// empty bucket for delete: remove object first (versioned may need delete markers; force via store)
	_ = st
	mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/mgmt-bucket/file.txt", nil, "s3", now, nil)
	// versioned buckets may remain non-empty; create a separate empty bucket for DeleteBucket
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/empty-mgmt", nil, "s3", now, nil)
	delBucket := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/empty-mgmt", nil, "s3", now, nil)
	if delBucket.Code != http.StatusOK && delBucket.Code != http.StatusNoContent {
		t.Fatalf("DeleteBucket status=%d body=%q", delBucket.Code, delBucket.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "s3-deny-list")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "s3-deny-list")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"s3:ListAllMyBuckets","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/", nil)
	signS3Header(t, req, nil, denyAK, denySecret, testRegion, "s3", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDenied") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}

func TestECSListDescribeDeregisterTags(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "ecs-mgmt-task", ecsTrustOK, now)
	mustCreateIAMRole(t, handler, "ecs-mgmt-exec", ecsTrustOK, now)
	taskRole := "arn:aws:iam::" + testAccountID + ":role/ecs-mgmt-task"
	execRole := "arn:aws:iam::" + testAccountID + ":role/ecs-mgmt-exec"

	descClusters := mustECSJSON(t, handler, "DescribeClusters", map[string]any{
		"clusters": []string{"default"},
	}, now)
	if descClusters.Code != http.StatusOK || !strings.Contains(descClusters.Body.String(), "default") {
		t.Fatalf("DescribeClusters status=%d body=%q", descClusters.Code, descClusters.Body.String())
	}

	reg := registerECSTaskDefinition(t, handler, "mgmt-family", taskRole, execRole, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterTaskDefinition status=%d body=%q", reg.Code, reg.Body.String())
	}
	var regOut map[string]any
	_ = json.Unmarshal(reg.Body.Bytes(), &regOut)
	td, _ := regOut["taskDefinition"].(map[string]any)
	tdARN, _ := td["taskDefinitionArn"].(string)

	listTD := mustECSJSON(t, handler, "ListTaskDefinitions", map[string]any{"familyPrefix": "mgmt-family"}, now)
	if listTD.Code != http.StatusOK || !strings.Contains(listTD.Body.String(), "mgmt-family") {
		t.Fatalf("ListTaskDefinitions status=%d body=%q", listTD.Code, listTD.Body.String())
	}

	tag := mustECSJSON(t, handler, "TagResource", map[string]any{
		"resourceArn": tdARN,
		"tags":        []map[string]any{{"key": "team", "value": "lab"}},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagResource status=%d body=%q", tag.Code, tag.Body.String())
	}
	listTags := mustECSJSON(t, handler, "ListTagsForResource", map[string]any{"resourceArn": tdARN}, now)
	if listTags.Code != http.StatusOK {
		t.Fatalf("ListTagsForResource status=%d body=%q", listTags.Code, listTags.Body.String())
	}
	untag := mustECSJSON(t, handler, "UntagResource", map[string]any{
		"resourceArn": tdARN, "tagKeys": []string{"team"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource status=%d body=%q", untag.Code, untag.Body.String())
	}

	dereg := mustECSJSON(t, handler, "DeregisterTaskDefinition", map[string]any{
		"taskDefinition": tdARN,
	}, now)
	if dereg.Code != http.StatusOK {
		t.Fatalf("DeregisterTaskDefinition status=%d body=%q", dereg.Code, dereg.Body.String())
	}
}

func TestEC2DescribeSubnets(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createVpc := mustEC2Query(t, handler, "Action=CreateVpc&Version=2016-11-15&CidrBlock=10.8.0.0/16", now)
	if createVpc.Code != http.StatusOK {
		t.Fatalf("CreateVpc status=%d body=%q", createVpc.Code, createVpc.Body.String())
	}
	vpcID := xmlTag(t, createVpc.Body.String(), "vpcId")
	createSubnet := mustEC2Query(t, handler, strings.Join([]string{
		"Action=CreateSubnet", "Version=2016-11-15", "VpcId=" + vpcID,
		"CidrBlock=10.8.1.0/24", "AvailabilityZone=us-east-1a",
	}, "&"), now)
	if createSubnet.Code != http.StatusOK {
		t.Fatalf("CreateSubnet status=%d body=%q", createSubnet.Code, createSubnet.Body.String())
	}
	subnetID := xmlTag(t, createSubnet.Body.String(), "subnetId")
	desc := mustEC2Query(t, handler,
		"Action=DescribeSubnets&Version=2016-11-15&SubnetId.1="+url.QueryEscape(subnetID), now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), subnetID) {
		t.Fatalf("DescribeSubnets status=%d body=%q", desc.Code, desc.Body.String())
	}
}

func TestSFNDescribeAndResourcePolicy(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	def := `{"StartAt":"P","States":{"P":{"Type":"Pass","End":true}}}`
	create := mustSFNJSON(t, handler, "CreateStateMachine", map[string]any{
		"name": "mgmt-sm", "definition": def,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateStateMachine status=%d body=%q", create.Code, create.Body.String())
	}
	var createOut map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &createOut)
	smARN, _ := createOut["stateMachineArn"].(string)

	desc := mustSFNJSON(t, handler, "DescribeStateMachine", map[string]any{"stateMachineArn": smARN}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "mgmt-sm") {
		t.Fatalf("DescribeStateMachine status=%d body=%q", desc.Code, desc.Body.String())
	}
	missing := mustSFNJSON(t, handler, "DescribeStateMachine", map[string]any{}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("empty arn should fail: %q", missing.Body.String())
	}

	policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"states:StartExecution","Resource":"%s"}]}`, smARN)
	putPol := mustSFNJSON(t, handler, "PutResourcePolicy", map[string]any{
		"stateMachineArn": smARN, "policy": policy,
	}, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}
	getPol := mustSFNJSON(t, handler, "GetResourcePolicy", map[string]any{"stateMachineArn": smARN}, now)
	if getPol.Code != http.StatusOK || !strings.Contains(getPol.Body.String(), "StartExecution") {
		t.Fatalf("GetResourcePolicy status=%d body=%q", getPol.Code, getPol.Body.String())
	}
	delPol := mustSFNJSON(t, handler, "DeleteResourcePolicy", map[string]any{"stateMachineArn": smARN}, now)
	if delPol.Code != http.StatusOK {
		t.Fatalf("DeleteResourcePolicy status=%d body=%q", delPol.Code, delPol.Body.String())
	}
	_ = mustSFNJSON(t, handler, "DeleteStateMachine", map[string]any{"stateMachineArn": smARN}, now)
}
