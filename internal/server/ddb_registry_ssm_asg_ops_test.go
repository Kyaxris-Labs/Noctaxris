package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDynamoDBKMSSealedItemQueryAndTransact(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	kms := mustKMSJSON(t, handler, "CreateKey", map[string]any{"Description": "ddb-sse"}, now)
	if kms.Code != http.StatusOK {
		t.Fatalf("CreateKey %d %s", kms.Code, kms.Body.String())
	}
	var kmsOut map[string]any
	_ = json.Unmarshal(kms.Body.Bytes(), &kmsOut)
	meta, _ := kmsOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "sse-kms-items",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "sk", "AttributeType": "S"},
			{"AttributeName": "gsi1", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
			{"AttributeName": "sk", "KeyType": "RANGE"},
		},
		"BillingMode": "PAY_PER_REQUEST",
		"SSESpecification": map[string]any{
			"Enabled":        true,
			"SSEType":        "KMS",
			"KMSMasterKeyId": keyID,
		},
		"StreamSpecification": map[string]any{
			"StreamEnabled":  true,
			"StreamViewType": "NEW_AND_OLD_IMAGES",
		},
		"GlobalSecondaryIndexes": []map[string]any{{
			"IndexName": "GSI1",
			"KeySchema": []map[string]any{
				{"AttributeName": "gsi1", "KeyType": "HASH"},
			},
			"Projection": map[string]any{"ProjectionType": "ALL"},
		}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable SSE-KMS %d %s", create.Code, create.Body.String())
	}

	for i := 0; i < 4; i++ {
		put := mustDynamoJSON(t, handler, "PutItem", map[string]any{
			"TableName": "sse-kms-items",
			"Item": map[string]any{
				"pk":   map[string]any{"S": "p"},
				"sk":   map[string]any{"S": fmt.Sprintf("s%d", i)},
				"gsi1": map[string]any{"S": "g"},
				"n":    map[string]any{"N": fmt.Sprintf("%d", i)},
			},
			"ReturnValues": "ALL_OLD",
		}, now)
		if put.Code != http.StatusOK {
			t.Fatalf("PutItem sealed %d %s", put.Code, put.Body.String())
		}
	}

	get := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "sse-kms-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "p"},
			"sk": map[string]any{"S": "s1"},
		},
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"s1"`) {
		t.Fatalf("GetItem sealed %d %s", get.Code, get.Body.String())
	}

	upd := mustDynamoJSON(t, handler, "UpdateItem", map[string]any{
		"TableName": "sse-kms-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "p"},
			"sk": map[string]any{"S": "s1"},
		},
		"UpdateExpression": "SET #n = :n",
		"ExpressionAttributeNames": map[string]any{"#n": "n"},
		"ExpressionAttributeValues": map[string]any{
			":n": map[string]any{"N": "42"},
		},
		"ReturnValues": "ALL_NEW",
	}, now)
	if upd.Code != http.StatusOK || !strings.Contains(upd.Body.String(), "42") {
		t.Fatalf("UpdateItem sealed %d %s", upd.Code, upd.Body.String())
	}

	q := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "sse-kms-items",
		"KeyConditionExpression": "pk = :p",
		"ExpressionAttributeValues": map[string]any{
			":p": map[string]any{"S": "p"},
		},
		"Limit": 2,
	}, now)
	if q.Code != http.StatusOK {
		t.Fatalf("Query sealed %d %s", q.Code, q.Body.String())
	}
	var qOut map[string]any
	_ = json.Unmarshal(q.Body.Bytes(), &qOut)
	if last, ok := qOut["LastEvaluatedKey"].(map[string]any); ok && last != nil {
		q2 := mustDynamoJSON(t, handler, "Query", map[string]any{
			"TableName":              "sse-kms-items",
			"IndexName":              "GSI1",
			"KeyConditionExpression": "gsi1 = :g",
			"ExpressionAttributeValues": map[string]any{
				":g": map[string]any{"S": "g"},
			},
			"ExclusiveStartKey": last,
		}, now)
		if q2.Code != http.StatusOK {
			t.Fatalf("Query GSI sealed page %d %s", q2.Code, q2.Body.String())
		}
	}

	scan := mustDynamoJSON(t, handler, "Scan", map[string]any{
		"TableName": "sse-kms-items", "Limit": 10,
	}, now)
	if scan.Code != http.StatusOK {
		t.Fatalf("Scan sealed %d %s", scan.Code, scan.Body.String())
	}

	txGet := mustDynamoJSON(t, handler, "TransactGetItems", map[string]any{
		"TransactItems": []map[string]any{
			{"Get": map[string]any{
				"TableName": "sse-kms-items",
				"Key": map[string]any{
					"pk": map[string]any{"S": "p"},
					"sk": map[string]any{"S": "s0"},
				},
			}},
			{"Get": map[string]any{
				"TableName": "sse-kms-items",
				"Key": map[string]any{
					"pk": map[string]any{"S": "p"},
					"sk": map[string]any{"S": "missing"},
				},
			}},
		},
	}, now)
	if txGet.Code != http.StatusOK {
		t.Fatalf("TransactGetItems sealed %d %s", txGet.Code, txGet.Body.String())
	}

	txWrite := mustDynamoJSON(t, handler, "TransactWriteItems", map[string]any{
		"TransactItems": []map[string]any{
			{"Put": map[string]any{
				"TableName": "sse-kms-items",
				"Item": map[string]any{
					"pk": map[string]any{"S": "p"}, "sk": map[string]any{"S": "tx"},
					"gsi1": map[string]any{"S": "g"}, "n": map[string]any{"N": "7"},
				},
			}},
			{"Delete": map[string]any{
				"TableName": "sse-kms-items",
				"Key": map[string]any{
					"pk": map[string]any{"S": "p"}, "sk": map[string]any{"S": "s3"},
				},
			}},
			{"ConditionCheck": map[string]any{
				"TableName": "sse-kms-items",
				"Key": map[string]any{
					"pk": map[string]any{"S": "p"}, "sk": map[string]any{"S": "s0"},
				},
				"ConditionExpression": "attribute_exists(pk)",
			}},
		},
	}, now)
	if txWrite.Code != http.StatusOK {
		t.Fatalf("TransactWriteItems sealed %d %s", txWrite.Code, txWrite.Body.String())
	}

	batchGet := mustDynamoJSON(t, handler, "BatchGetItem", map[string]any{
		"RequestItems": map[string]any{
			"sse-kms-items": map[string]any{
				"Keys": []map[string]any{
					{"pk": map[string]any{"S": "p"}, "sk": map[string]any{"S": "s0"}},
					{"pk": map[string]any{"S": "p"}, "sk": map[string]any{"S": "tx"}},
				},
			},
		},
	}, now)
	if batchGet.Code != http.StatusOK {
		t.Fatalf("BatchGetItem sealed %d %s", batchGet.Code, batchGet.Body.String())
	}
}

func TestRegistryBlobUploadPatchManifestAndTags(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	repo := "reg-ops"
	create := mustECRJSON(t, handler, "CreateRepository", map[string]any{"repositoryName": repo}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateRepository %d %s", create.Code, create.Body.String())
	}
	pol := mustECRJSON(t, handler, "SetRepositoryPolicy", map[string]any{
		"repositoryName": repo,
		"policyText":     `{"Version":"2008-10-17","Statement":[{"Sid":"Allow","Effect":"Allow","Principal":"*","Action":"ecr:*"}]}`,
	}, now)
	if pol.Code != http.StatusOK {
		t.Fatalf("SetRepositoryPolicy %d %s", pol.Code, pol.Body.String())
	}
	getPol := mustECRJSON(t, handler, "GetRepositoryPolicy", map[string]any{"repositoryName": repo}, now)
	if getPol.Code != http.StatusOK {
		t.Fatalf("GetRepositoryPolicy %d %s", getPol.Code, getPol.Body.String())
	}
	_ = mustECRJSON(t, handler, "PutLifecyclePolicy", map[string]any{
		"repositoryName": repo,
		"lifecyclePolicyText": `{"rules":[{"rulePriority":1,"selection":{"tagStatus":"untagged","countType":"imageCountMoreThan","countNumber":5},"action":{"type":"expire"}}]}`,
	}, now)
	desc := mustECRJSON(t, handler, "DescribeRepositories", map[string]any{
		"repositoryNames": []string{repo},
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), repo) {
		t.Fatalf("DescribeRepositories %d %s", desc.Code, desc.Body.String())
	}
	listImg := mustECRJSON(t, handler, "ListImages", map[string]any{"repositoryName": repo}, now)
	if listImg.Code != http.StatusOK {
		t.Fatalf("ListImages %d %s", listImg.Code, listImg.Body.String())
	}

	token := issueRegistryToken(t, handler, now)
	auth := registryAuthHeader(token)
	account := testAccountID

	root := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/v2/", nil)
	root.Header.Set("Authorization", auth)
	rootRec := httptest.NewRecorder()
	handler.ServeHTTP(rootRec, root)
	if rootRec.Code != http.StatusOK {
		t.Fatalf("GET /v2/ %d", rootRec.Code)
	}
	headRoot := httptest.NewRequest(http.MethodHead, "http://127.0.0.1:4566/v2/", nil)
	headRoot.Header.Set("Authorization", auth)
	headRootRec := httptest.NewRecorder()
	handler.ServeHTTP(headRootRec, headRoot)
	if headRootRec.Code != http.StatusOK {
		t.Fatalf("HEAD /v2/ %d", headRootRec.Code)
	}
	unauth := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/v2/", nil)
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, unauth)
	if unauthRec.Code == http.StatusOK {
		t.Fatalf("unauth /v2/ should fail")
	}

	layer := []byte(strings.Repeat("L", 4096))
	digest := "sha256:" + sha256Hex(layer)
	start := httptest.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/blobs/uploads/", account, repo), nil)
	start.Header.Set("Authorization", auth)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, start)
	if startRec.Code != http.StatusAccepted {
		t.Fatalf("upload start %d %s", startRec.Code, startRec.Body.String())
	}
	location := startRec.Header().Get("Location")

	putBlob := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:4566"+location+"?digest="+digest, bytes.NewReader(layer))
	putBlob.Header.Set("Authorization", auth)
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putBlob)
	if putRec.Code != http.StatusCreated {
		t.Fatalf("blob put %d %s", putRec.Code, putRec.Body.String())
	}
	getBlob := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/blobs/%s", account, repo, digest), nil)
	getBlob.Header.Set("Authorization", auth)
	getBlobRec := httptest.NewRecorder()
	handler.ServeHTTP(getBlobRec, getBlob)
	if getBlobRec.Code != http.StatusOK || len(getBlobRec.Body.Bytes()) == 0 {
		t.Fatalf("GET blob %d", getBlobRec.Code)
	}

	manifest := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"mediaType":"application/vnd.docker.container.image.v1+json","size":1,"digest":"sha256:0000000000000000000000000000000000000000000000000000000000000001"},"layers":[{"mediaType":"application/vnd.docker.image.rootfs.diff.tar.gzip","size":%d,"digest":%q}]}`, len(layer), digest)
	putMan := httptest.NewRequest(http.MethodPut, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/manifests/latest", account, repo), strings.NewReader(manifest))
	putMan.Header.Set("Authorization", auth)
	putMan.Header.Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
	putManRec := httptest.NewRecorder()
	handler.ServeHTTP(putManRec, putMan)
	if putManRec.Code != http.StatusCreated {
		t.Fatalf("manifest put %d %s", putManRec.Code, putManRec.Body.String())
	}
	getMan := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/manifests/latest", account, repo), nil)
	getMan.Header.Set("Authorization", auth)
	getManRec := httptest.NewRecorder()
	handler.ServeHTTP(getManRec, getMan)
	if getManRec.Code != http.StatusOK {
		t.Fatalf("GET manifest %d", getManRec.Code)
	}
	tags := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/tags/list", account, repo), nil)
	tags.Header.Set("Authorization", auth)
	tagsRec := httptest.NewRecorder()
	handler.ServeHTTP(tagsRec, tags)
	if tagsRec.Code != http.StatusOK || !strings.Contains(tagsRec.Body.String(), "latest") {
		t.Fatalf("tags/list %d %s", tagsRec.Code, tagsRec.Body.String())
	}

	delPol := mustECRJSON(t, handler, "DeleteRepositoryPolicy", map[string]any{"repositoryName": repo}, now)
	if delPol.Code != http.StatusOK {
		t.Fatalf("DeleteRepositoryPolicy %d %s", delPol.Code, delPol.Body.String())
	}
}

func TestSSMSecureHistoryLabelsAndDeleteOps(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	kms := mustKMSJSON(t, handler, "CreateKey", map[string]any{"Description": "ssm-ops"}, now)
	var kmsOut map[string]any
	_ = json.Unmarshal(kms.Body.Bytes(), &kmsOut)
	meta, _ := kmsOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	put := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name": "/ops/secure", "Value": "secret-1", "Type": "SecureString", "KeyId": keyID,
		"Tags": []map[string]any{{"Key": "env", "Value": "lab"}},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutParameter secure %d %s", put.Code, put.Body.String())
	}
	put2 := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name": "/ops/secure", "Value": "secret-2", "Type": "SecureString", "KeyId": keyID, "Overwrite": true,
	}, now)
	if put2.Code != http.StatusOK {
		t.Fatalf("PutParameter overwrite %d %s", put2.Code, put2.Body.String())
	}
	get := mustSSMJSON(t, handler, "GetParameter", map[string]any{
		"Name": "/ops/secure", "WithDecryption": true,
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "secret-2") {
		t.Fatalf("GetParameter %d %s", get.Code, get.Body.String())
	}
	hist := mustSSMJSON(t, handler, "GetParameterHistory", map[string]any{"Name": "/ops/secure"}, now)
	if hist.Code != http.StatusOK {
		t.Fatalf("GetParameterHistory %d %s", hist.Code, hist.Body.String())
	}
	label := mustSSMJSON(t, handler, "LabelParameterVersion", map[string]any{
		"Name": "/ops/secure", "ParameterVersion": 1, "Labels": []string{"stable"},
	}, now)
	if label.Code != http.StatusOK {
		t.Fatalf("LabelParameterVersion %d %s", label.Code, label.Body.String())
	}
	getLabel := mustSSMJSON(t, handler, "GetParameter", map[string]any{
		"Name": "/ops/secure:stable", "WithDecryption": true,
	}, now)
	if getLabel.Code != http.StatusOK {
		t.Fatalf("GetParameter by label %d %s", getLabel.Code, getLabel.Body.String())
	}

	_ = mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name": "/ops/plain", "Value": "v", "Type": "String",
	}, now)
	byPath := mustSSMJSON(t, handler, "GetParametersByPath", map[string]any{
		"Path": "/ops", "Recursive": true, "WithDecryption": true,
	}, now)
	if byPath.Code != http.StatusOK || !strings.Contains(byPath.Body.String(), "/ops/") {
		t.Fatalf("GetParametersByPath %d %s", byPath.Code, byPath.Body.String())
	}
	multi := mustSSMJSON(t, handler, "GetParameters", map[string]any{
		"Names": []string{"/ops/secure", "/ops/plain", "/ops/missing"}, "WithDecryption": true,
	}, now)
	if multi.Code != http.StatusOK {
		t.Fatalf("GetParameters %d %s", multi.Code, multi.Body.String())
	}
	desc := mustSSMJSON(t, handler, "DescribeParameters", map[string]any{}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeParameters %d %s", desc.Code, desc.Body.String())
	}
	addTags := mustSSMJSON(t, handler, "AddTagsToResource", map[string]any{
		"ResourceType": "Parameter", "ResourceId": "/ops/plain",
		"Tags": []map[string]any{{"Key": "team", "Value": "core"}},
	}, now)
	if addTags.Code != http.StatusOK {
		t.Fatalf("AddTagsToResource %d %s", addTags.Code, addTags.Body.String())
	}
	listTags := mustSSMJSON(t, handler, "ListTagsForResource", map[string]any{
		"ResourceType": "Parameter", "ResourceId": "/ops/plain",
	}, now)
	if listTags.Code != http.StatusOK {
		t.Fatalf("ListTagsForResource %d %s", listTags.Code, listTags.Body.String())
	}
	remTags := mustSSMJSON(t, handler, "RemoveTagsFromResource", map[string]any{
		"ResourceType": "Parameter", "ResourceId": "/ops/plain", "TagKeys": []string{"team"},
	}, now)
	if remTags.Code != http.StatusOK {
		t.Fatalf("RemoveTagsFromResource %d %s", remTags.Code, remTags.Body.String())
	}

	delOne := mustSSMJSON(t, handler, "DeleteParameter", map[string]any{"Name": "/ops/plain"}, now)
	if delOne.Code != http.StatusOK {
		t.Fatalf("DeleteParameter %d %s", delOne.Code, delOne.Body.String())
	}
	delSecure := mustSSMJSON(t, handler, "DeleteParameter", map[string]any{"Name": "/ops/secure"}, now)
	if delSecure.Code != http.StatusOK {
		t.Fatalf("DeleteParameter secure %d %s", delSecure.Code, delSecure.Body.String())
	}
	_ = mustSSMJSON(t, handler, "DeleteParameters", map[string]any{
		"Names": []string{"/ops/secure", "/ops/missing"},
	}, now)
	miss := mustSSMJSON(t, handler, "GetParameter", map[string]any{"Name": "/ops/secure"}, now)
	if miss.Code == http.StatusOK {
		t.Fatalf("deleted param should fail")
	}
}

func TestASGLifecycleHooksAndTagsOps(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createLC := mustASGQuery(t, handler, strings.Join([]string{
		"Action=CreateLaunchConfiguration",
		"Version=2011-01-01",
		"LaunchConfigurationName=lc-hooks",
		"ImageId=ami-hooks",
		"InstanceType=t3.micro",
	}, "&"), now)
	if createLC.Code != 200 {
		t.Fatalf("CreateLaunchConfiguration %d %s", createLC.Code, createLC.Body.String())
	}
	createASG := mustASGQuery(t, handler, strings.Join([]string{
		"Action=CreateAutoScalingGroup",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-hooks",
		"LaunchConfigurationName=lc-hooks",
		"MinSize=0",
		"MaxSize=2",
		"DesiredCapacity=0",
		"AvailabilityZones.member.1=us-east-1a",
		"Tags.member.1.Key=env",
		"Tags.member.1.Value=lab",
		"Tags.member.1.PropagateAtLaunch=true",
	}, "&"), now)
	if createASG.Code != 200 {
		t.Fatalf("CreateAutoScalingGroup %d %s", createASG.Code, createASG.Body.String())
	}

	putHook := mustASGQuery(t, handler, strings.Join([]string{
		"Action=PutLifecycleHook",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-hooks",
		"LifecycleHookName=hook1",
		"LifecycleTransition=autoscaling:EC2_INSTANCE_LAUNCHING",
		"HeartbeatTimeout=60",
		"DefaultResult=CONTINUE",
	}, "&"), now)
	if putHook.Code != 200 {
		t.Fatalf("PutLifecycleHook %d %s", putHook.Code, putHook.Body.String())
	}
	descHooks := mustASGQuery(t, handler,
		"Action=DescribeLifecycleHooks&Version=2011-01-01&AutoScalingGroupName=asg-hooks", now)
	if descHooks.Code != 200 || !strings.Contains(descHooks.Body.String(), "hook1") {
		t.Fatalf("DescribeLifecycleHooks %d %s", descHooks.Code, descHooks.Body.String())
	}
	delHook := mustASGQuery(t, handler, strings.Join([]string{
		"Action=DeleteLifecycleHook",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-hooks",
		"LifecycleHookName=hook1",
	}, "&"), now)
	if delHook.Code != 200 {
		t.Fatalf("DeleteLifecycleHook %d %s", delHook.Code, delHook.Body.String())
	}

	createTG := mustJSONTarget(t, handler, "ElasticLoadBalancingV2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "asg-tg", "Protocol": "HTTP", "Port": 80, "VpcId": "vpc-lab",
	}, now)
	tgARN := ""
	if createTG.Code == http.StatusOK {
		var tgOut map[string]any
		_ = json.Unmarshal(createTG.Body.Bytes(), &tgOut)
		if tgs, ok := tgOut["TargetGroups"].([]any); ok && len(tgs) > 0 {
			tgARN, _ = tgs[0].(map[string]any)["TargetGroupArn"].(string)
		}
	}
	if tgARN != "" {
		attach := mustASGQuery(t, handler, "Action=AttachLoadBalancerTargetGroups&Version=2011-01-01&AutoScalingGroupName=asg-hooks&TargetGroupARNs.member.1="+tgARN, now)
		if attach.Code != 200 {
			t.Fatalf("AttachLoadBalancerTargetGroups %d %s", attach.Code, attach.Body.String())
		}
		descTG := mustASGQuery(t, handler, "Action=DescribeLoadBalancerTargetGroups&Version=2011-01-01&AutoScalingGroupName=asg-hooks", now)
		if descTG.Code != 200 {
			t.Fatalf("DescribeLoadBalancerTargetGroups %d %s", descTG.Code, descTG.Body.String())
		}
		detach := mustASGQuery(t, handler, "Action=DetachLoadBalancerTargetGroups&Version=2011-01-01&AutoScalingGroupName=asg-hooks&TargetGroupARNs.member.1="+tgARN, now)
		if detach.Code != 200 {
			t.Fatalf("DetachLoadBalancerTargetGroups %d %s", detach.Code, detach.Body.String())
		}
	}

	setCap := mustASGQuery(t, handler, strings.Join([]string{
		"Action=SetDesiredCapacity",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-hooks",
		"DesiredCapacity=1",
	}, "&"), now)
	if setCap.Code != 200 {
		t.Fatalf("SetDesiredCapacity %d %s", setCap.Code, setCap.Body.String())
	}
	descInst := mustASGQuery(t, handler, "Action=DescribeAutoScalingInstances&Version=2011-01-01", now)
	if descInst.Code != 200 {
		t.Fatalf("DescribeAutoScalingInstances %d %s", descInst.Code, descInst.Body.String())
	}

	delASG := mustASGQuery(t, handler, "Action=DeleteAutoScalingGroup&Version=2011-01-01&AutoScalingGroupName=asg-hooks&ForceDelete=true", now)
	if delASG.Code != 200 {
		t.Fatalf("DeleteAutoScalingGroup %d %s", delASG.Code, delASG.Body.String())
	}
	delLC := mustASGQuery(t, handler, "Action=DeleteLaunchConfiguration&Version=2011-01-01&LaunchConfigurationName=lc-hooks", now)
	if delLC.Code != 200 {
		t.Fatalf("DeleteLaunchConfiguration %d %s", delLC.Code, delLC.Body.String())
	}
}
