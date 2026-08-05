package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDynamoDBIndexQueryPaginationAndFilter(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "idx-query",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "sk", "AttributeType": "S"},
			{"AttributeName": "gsi1", "AttributeType": "S"},
			{"AttributeName": "lsi1", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
			{"AttributeName": "sk", "KeyType": "RANGE"},
		},
		"BillingMode": "PAY_PER_REQUEST",
		"LocalSecondaryIndexes": []map[string]any{{
			"IndexName": "LSI1",
			"KeySchema": []map[string]any{
				{"AttributeName": "pk", "KeyType": "HASH"},
				{"AttributeName": "lsi1", "KeyType": "RANGE"},
			},
			"Projection": map[string]any{"ProjectionType": "ALL"},
		}},
		"GlobalSecondaryIndexes": []map[string]any{{
			"IndexName": "GSI1",
			"KeySchema": []map[string]any{
				{"AttributeName": "gsi1", "KeyType": "HASH"},
			},
			"Projection": map[string]any{"ProjectionType": "ALL"},
		}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable %d %s", create.Code, create.Body.String())
	}

	for i := 0; i < 5; i++ {
		put := mustDynamoJSON(t, handler, "PutItem", map[string]any{
			"TableName": "idx-query",
			"Item": map[string]any{
				"pk":   map[string]any{"S": "p1"},
				"sk":   map[string]any{"S": fmt.Sprintf("s%d", i)},
				"gsi1": map[string]any{"S": "g1"},
				"lsi1": map[string]any{"S": fmt.Sprintf("l%d", i)},
				"n":    map[string]any{"N": fmt.Sprintf("%d", i)},
			},
		}, now)
		if put.Code != http.StatusOK {
			t.Fatalf("PutItem %d %s", put.Code, put.Body.String())
		}
	}

	qGSI := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "idx-query",
		"IndexName":              "GSI1",
		"KeyConditionExpression": "gsi1 = :g",
		"ExpressionAttributeValues": map[string]any{
			":g": map[string]any{"S": "g1"},
		},
		"Limit": 2,
	}, now)
	if qGSI.Code != http.StatusOK {
		t.Fatalf("Query GSI %d %s", qGSI.Code, qGSI.Body.String())
	}
	var qOut map[string]any
	_ = json.Unmarshal(qGSI.Body.Bytes(), &qOut)
	last, _ := qOut["LastEvaluatedKey"].(map[string]any)
	if last != nil {
		page2 := mustDynamoJSON(t, handler, "Query", map[string]any{
			"TableName":              "idx-query",
			"IndexName":              "GSI1",
			"KeyConditionExpression": "gsi1 = :g",
			"ExpressionAttributeValues": map[string]any{
				":g": map[string]any{"S": "g1"},
			},
			"ExclusiveStartKey": last,
			"Limit":             10,
		}, now)
		if page2.Code != http.StatusOK {
			t.Fatalf("Query GSI page2 %d %s", page2.Code, page2.Body.String())
		}
	}

	qLSI := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "idx-query",
		"IndexName":              "LSI1",
		"KeyConditionExpression": "pk = :p AND lsi1 = :l",
		"ExpressionAttributeValues": map[string]any{
			":p": map[string]any{"S": "p1"},
			":l": map[string]any{"S": "l1"},
		},
	}, now)
	if qLSI.Code != http.StatusOK || !strings.Contains(qLSI.Body.String(), `"s1"`) {
		t.Fatalf("Query LSI %d %s", qLSI.Code, qLSI.Body.String())
	}

	qFilter := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "idx-query",
		"KeyConditionExpression": "pk = :p",
		"FilterExpression":       "#n = :n",
		"ExpressionAttributeNames": map[string]any{
			"#n": "n",
		},
		"ExpressionAttributeValues": map[string]any{
			":p": map[string]any{"S": "p1"},
			":n": map[string]any{"N": "3"},
		},
	}, now)
	if qFilter.Code != http.StatusOK || !strings.Contains(qFilter.Body.String(), `"s3"`) {
		t.Fatalf("Query filter %d %s", qFilter.Code, qFilter.Body.String())
	}

	badIdx := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "idx-query",
		"IndexName":              "NOPE",
		"KeyConditionExpression": "pk = :p",
		"ExpressionAttributeValues": map[string]any{
			":p": map[string]any{"S": "p1"},
		},
	}, now)
	if badIdx.Code == http.StatusOK {
		t.Fatalf("missing index should fail")
	}

	upd := mustDynamoJSON(t, handler, "UpdateItem", map[string]any{
		"TableName": "idx-query",
		"Key": map[string]any{
			"pk": map[string]any{"S": "p1"},
			"sk": map[string]any{"S": "s0"},
		},
		"UpdateExpression": "SET #n = :n",
		"ExpressionAttributeNames": map[string]any{
			"#n": "n",
		},
		"ExpressionAttributeValues": map[string]any{
			":n": map[string]any{"N": "99"},
		},
		"ReturnValues": "ALL_NEW",
	}, now)
	if upd.Code != http.StatusOK || !strings.Contains(upd.Body.String(), "99") {
		t.Fatalf("UpdateItem %d %s", upd.Code, upd.Body.String())
	}

	delCond := mustDynamoJSON(t, handler, "DeleteItem", map[string]any{
		"TableName": "idx-query",
		"Key": map[string]any{
			"pk": map[string]any{"S": "p1"},
			"sk": map[string]any{"S": "s4"},
		},
		"ConditionExpression": "attribute_exists(pk)",
		"ReturnValues":        "ALL_OLD",
	}, now)
	if delCond.Code != http.StatusOK {
		t.Fatalf("DeleteItem %d %s", delCond.Code, delCond.Body.String())
	}

	scanFilter := mustDynamoJSON(t, handler, "Scan", map[string]any{
		"TableName":        "idx-query",
		"FilterExpression": "#n = :n",
		"ExpressionAttributeNames": map[string]any{
			"#n": "n",
		},
		"ExpressionAttributeValues": map[string]any{
			":n": map[string]any{"N": "99"},
		},
		"Limit": 10,
	}, now)
	if scanFilter.Code != http.StatusOK || !strings.Contains(scanFilter.Body.String(), "99") {
		t.Fatalf("Scan filter %d %s", scanFilter.Code, scanFilter.Body.String())
	}
}

func TestIAMPolicyVersionAndOIDCProviderOps(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	doc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`)
	create := iamForm(t, handler, "Action=CreatePolicy&Version=2010-05-08&PolicyName=ver-pol&PolicyDocument="+doc, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreatePolicy %d %s", create.Code, create.Body.String())
	}
	arn := xmlTag(t, create.Body.String(), "Arn")

	v2doc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:ListBucket","Resource":"*"}]}`)
	v2 := iamForm(t, handler, "Action=CreatePolicyVersion&Version=2010-05-08&PolicyArn="+url.QueryEscape(arn)+
		"&PolicyDocument="+v2doc+"&SetAsDefault=true", now)
	if v2.Code != http.StatusOK {
		t.Fatalf("CreatePolicyVersion %d %s", v2.Code, v2.Body.String())
	}
	vid := xmlTag(t, v2.Body.String(), "VersionId")

	getV := iamForm(t, handler, "Action=GetPolicyVersion&Version=2010-05-08&PolicyArn="+url.QueryEscape(arn)+"&VersionId="+vid, now)
	if getV.Code != http.StatusOK {
		t.Fatalf("GetPolicyVersion %d %s", getV.Code, getV.Body.String())
	}
	listV := iamForm(t, handler, "Action=ListPolicyVersions&Version=2010-05-08&PolicyArn="+url.QueryEscape(arn), now)
	if listV.Code != http.StatusOK || !strings.Contains(listV.Body.String(), "v1") {
		t.Fatalf("ListPolicyVersions %d %s", listV.Code, listV.Body.String())
	}
	setDef := iamForm(t, handler, "Action=SetDefaultPolicyVersion&Version=2010-05-08&PolicyArn="+url.QueryEscape(arn)+"&VersionId=v1", now)
	if setDef.Code != http.StatusOK {
		t.Fatalf("SetDefaultPolicyVersion %d %s", setDef.Code, setDef.Body.String())
	}
	delDef := iamForm(t, handler, "Action=DeletePolicyVersion&Version=2010-05-08&PolicyArn="+url.QueryEscape(arn)+"&VersionId=v1", now)
	if delDef.Code == http.StatusOK {
		t.Fatalf("DeletePolicyVersion default should fail: %s", delDef.Body.String())
	}
	delV2 := iamForm(t, handler, "Action=DeletePolicyVersion&Version=2010-05-08&PolicyArn="+url.QueryEscape(arn)+"&VersionId="+vid, now)
	if delV2.Code != http.StatusOK {
		t.Fatalf("DeletePolicyVersion non-default %d %s", delV2.Code, delV2.Body.String())
	}
	missV := iamForm(t, handler, "Action=GetPolicyVersion&Version=2010-05-08&PolicyArn="+url.QueryEscape(arn)+"&VersionId=v9", now)
	if missV.Code == http.StatusOK {
		t.Fatalf("missing version should fail")
	}
	badDoc := iamForm(t, handler, "Action=CreatePolicyVersion&Version=2010-05-08&PolicyArn="+url.QueryEscape(arn)+
		"&PolicyDocument="+url.QueryEscape("not-json"), now)
	if badDoc.Code == http.StatusOK {
		t.Fatalf("bad policy doc should fail")
	}

	oidc := iamForm(t, handler, "Action=CreateOpenIDConnectProvider&Version=2010-05-08&Url="+url.QueryEscape("https://oidc.example.com/lab")+
		"&ClientIDList.member.1=aud1&ThumbprintList.member.1=0123456789012345678901234567890123456789", now)
	if oidc.Code != http.StatusOK {
		t.Fatalf("CreateOpenIDConnectProvider %d %s", oidc.Code, oidc.Body.String())
	}
	oidcARN := xmlTag(t, oidc.Body.String(), "OpenIDConnectProviderArn")
	getOIDC := iamForm(t, handler, "Action=GetOpenIDConnectProvider&Version=2010-05-08&OpenIDConnectProviderArn="+url.QueryEscape(oidcARN), now)
	if getOIDC.Code != http.StatusOK {
		t.Fatalf("GetOpenIDConnectProvider %d %s", getOIDC.Code, getOIDC.Body.String())
	}
	listOIDC := iamForm(t, handler, "Action=ListOpenIDConnectProviders&Version=2010-05-08", now)
	if listOIDC.Code != http.StatusOK || !strings.Contains(listOIDC.Body.String(), oidcARN) {
		t.Fatalf("ListOpenIDConnectProviders %d %s", listOIDC.Code, listOIDC.Body.String())
	}
	delOIDC := iamForm(t, handler, "Action=DeleteOpenIDConnectProvider&Version=2010-05-08&OpenIDConnectProviderArn="+url.QueryEscape(oidcARN), now)
	if delOIDC.Code != http.StatusOK {
		t.Fatalf("DeleteOpenIDConnectProvider %d %s", delOIDC.Code, delOIDC.Body.String())
	}

	samlMeta := url.QueryEscape(`<?xml version="1.0"?><EntityDescriptor entityID="https://idp.example/lab"></EntityDescriptor>`)
	saml := iamForm(t, handler, "Action=CreateSAMLProvider&Version=2010-05-08&Name=lab-saml&SAMLMetadataDocument="+samlMeta, now)
	if saml.Code != http.StatusOK {
		t.Fatalf("CreateSAMLProvider %d %s", saml.Code, saml.Body.String())
	}
	samlARN := xmlTag(t, saml.Body.String(), "SAMLProviderArn")
	getSAML := iamForm(t, handler, "Action=GetSAMLProvider&Version=2010-05-08&SAMLProviderArn="+url.QueryEscape(samlARN), now)
	if getSAML.Code != http.StatusOK {
		t.Fatalf("GetSAMLProvider %d %s", getSAML.Code, getSAML.Body.String())
	}
	delSAML := iamForm(t, handler, "Action=DeleteSAMLProvider&Version=2010-05-08&SAMLProviderArn="+url.QueryEscape(samlARN), now)
	if delSAML.Code != http.StatusOK {
		t.Fatalf("DeleteSAMLProvider %d %s", delSAML.Code, delSAML.Body.String())
	}
}
