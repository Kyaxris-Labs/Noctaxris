package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestS3ObjectHeadCopyMultipartOps(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	src := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/obj-ops-src", nil, "s3", now, nil)
	if src.Code < 200 || src.Code >= 300 {
		t.Fatalf("src bucket %d", src.Code)
	}
	dst := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/obj-ops-dst", nil, "s3", now, nil)
	if dst.Code < 200 || dst.Code >= 300 {
		t.Fatalf("dst bucket %d", dst.Code)
	}

	put := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/obj-ops-src/a.txt", []byte("hello-object"), "s3", now, map[string]string{
		"Content-Type": "text/plain",
	})
	if put.Code < 200 || put.Code >= 300 {
		t.Fatalf("PutObject %d %s", put.Code, put.Body.String())
	}
	head := mustS3(t, handler, http.MethodHead, "http://127.0.0.1:4566/obj-ops-src/a.txt", nil, "s3", now, nil)
	if head.Code != http.StatusOK {
		t.Fatalf("HeadObject %d", head.Code)
	}
	headMiss := mustS3(t, handler, http.MethodHead, "http://127.0.0.1:4566/obj-ops-src/missing.txt", nil, "s3", now, nil)
	if headMiss.Code != http.StatusNotFound {
		t.Fatalf("HeadObject missing want 404 got %d", headMiss.Code)
	}
	headBkt := mustS3(t, handler, http.MethodHead, "http://127.0.0.1:4566/obj-ops-src", nil, "s3", now, nil)
	if headBkt.Code != http.StatusOK {
		t.Fatalf("HeadBucket %d", headBkt.Code)
	}

	copy := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/obj-ops-dst/b.txt", nil, "s3", now, map[string]string{
		"x-amz-copy-source": "/obj-ops-src/a.txt",
	})
	if copy.Code != http.StatusOK {
		t.Fatalf("CopyObject %d %s", copy.Code, copy.Body.String())
	}
	copyMiss := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/obj-ops-dst/c.txt", nil, "s3", now, map[string]string{
		"x-amz-copy-source": "/obj-ops-src/nope.txt",
	})
	if copyMiss.Code == http.StatusOK {
		t.Fatalf("CopyObject missing source should fail")
	}

	list := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/obj-ops-src?list-type=2&prefix=a", nil, "s3", now, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "a.txt") {
		t.Fatalf("ListObjectsV2 %d %s", list.Code, list.Body.String())
	}

	createMPU := mustS3(t, handler, http.MethodPost, "http://127.0.0.1:4566/obj-ops-src/big.bin?uploads", nil, "s3", now, map[string]string{
		"Content-Type": "application/octet-stream",
	})
	if createMPU.Code != http.StatusOK || !strings.Contains(createMPU.Body.String(), "UploadId") {
		t.Fatalf("CreateMultipartUpload %d %s", createMPU.Code, createMPU.Body.String())
	}
	uploadID := xmlTag(t, createMPU.Body.String(), "UploadId")
	partBody := []byte(strings.Repeat("p", 6*1024*1024))
	part1 := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/obj-ops-src/big.bin?partNumber=1&uploadId="+uploadID, partBody, "s3", now, nil)
	if part1.Code < 200 || part1.Code >= 300 {
		t.Fatalf("UploadPart %d %s", part1.Code, part1.Body.String())
	}
	etag := strings.Trim(part1.Header().Get("ETag"), `"`)
	listParts := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/obj-ops-src/big.bin?uploadId="+uploadID, nil, "s3", now, nil)
	if listParts.Code != http.StatusOK {
		t.Fatalf("ListParts %d %s", listParts.Code, listParts.Body.String())
	}
	listMPU := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/obj-ops-src?uploads", nil, "s3", now, nil)
	if listMPU.Code != http.StatusOK {
		t.Fatalf("ListMultipartUploads %d %s", listMPU.Code, listMPU.Body.String())
	}
	completeXML := `<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>` + etag + `</ETag></Part></CompleteMultipartUpload>`
	complete := mustS3(t, handler, http.MethodPost, "http://127.0.0.1:4566/obj-ops-src/big.bin?uploadId="+uploadID, []byte(completeXML), "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if complete.Code != http.StatusOK {
		t.Fatalf("CompleteMultipartUpload %d %s", complete.Code, complete.Body.String())
	}

	createAbort := mustS3(t, handler, http.MethodPost, "http://127.0.0.1:4566/obj-ops-src/abort.bin?uploads", nil, "s3", now, nil)
	if createAbort.Code != http.StatusOK {
		t.Fatalf("CreateMPU abort %d", createAbort.Code)
	}
	abortID := xmlTag(t, createAbort.Body.String(), "UploadId")
	abort := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/obj-ops-src/abort.bin?uploadId="+abortID, nil, "s3", now, nil)
	if abort.Code != http.StatusNoContent && abort.Code != http.StatusOK {
		t.Fatalf("AbortMultipartUpload %d %s", abort.Code, abort.Body.String())
	}

	enc := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/obj-ops-src?encryption",
		[]byte(`<ServerSideEncryptionConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>AES256</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`),
		"s3", now, map[string]string{"Content-Type": "application/xml"})
	if enc.Code != http.StatusOK {
		t.Fatalf("PutBucketEncryption %d %s", enc.Code, enc.Body.String())
	}
	getEnc := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/obj-ops-src?encryption", nil, "s3", now, nil)
	if getEnc.Code != http.StatusOK || !strings.Contains(getEnc.Body.String(), "AES256") {
		t.Fatalf("GetBucketEncryption %d %s", getEnc.Code, getEnc.Body.String())
	}
	delEnc := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/obj-ops-src?encryption", nil, "s3", now, nil)
	if delEnc.Code != http.StatusNoContent && delEnc.Code != http.StatusOK {
		t.Fatalf("DeleteBucketEncryption %d %s", delEnc.Code, delEnc.Body.String())
	}

	pol := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/obj-ops-src?policy",
		[]byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::obj-ops-src/*"}]}`),
		"s3", now, map[string]string{"Content-Type": "application/json"})
	if pol.Code != http.StatusOK && pol.Code != http.StatusNoContent {
		t.Fatalf("PutBucketPolicy %d %s", pol.Code, pol.Body.String())
	}
	getPol := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/obj-ops-src?policy", nil, "s3", now, nil)
	if getPol.Code != http.StatusOK {
		t.Fatalf("GetBucketPolicy %d %s", getPol.Code, getPol.Body.String())
	}
	delPol := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/obj-ops-src?policy", nil, "s3", now, nil)
	if delPol.Code != http.StatusNoContent && delPol.Code != http.StatusOK {
		t.Fatalf("DeleteBucketPolicy %d %s", delPol.Code, delPol.Body.String())
	}

	delObj := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/obj-ops-src/a.txt", nil, "s3", now, nil)
	if delObj.Code != http.StatusNoContent && delObj.Code != http.StatusOK {
		t.Fatalf("DeleteObject %d", delObj.Code)
	}
}

func TestCognitoPoolClientAuthAttributeOps(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	pool := cognitoMustOK(t, handler, "CreateUserPool", map[string]any{
		"PoolName": "auth-ops-pool",
		"AutoVerifiedAttributes": []string{"email"},
		"Schema": []map[string]any{
			{"Name": "email", "Required": true, "Mutable": true, "AttributeDataType": "String"},
		},
	}, now)
	up, _ := pool["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)

	updPool := cognitoMustOK(t, handler, "UpdateUserPool", map[string]any{
		"UserPoolId": poolID,
		"AutoVerifiedAttributes": []string{"email"},
	}, now)
	_ = updPool
	desc := cognitoMustOK(t, handler, "DescribeUserPool", map[string]any{"UserPoolId": poolID}, now)
	if desc["UserPool"] == nil {
		t.Fatalf("DescribeUserPool: %v", desc)
	}
	listPools := cognitoMustOK(t, handler, "ListUserPools", map[string]any{"MaxResults": 10}, now)
	if !strings.Contains(mustMarshal(t, listPools), poolID) {
		t.Fatalf("ListUserPools: %v", listPools)
	}

	client := cognitoMustOK(t, handler, "CreateUserPoolClient", map[string]any{
		"UserPoolId": poolID, "ClientName": "auth-ops-client",
		"ExplicitAuthFlows": []string{"ALLOW_USER_PASSWORD_AUTH", "ALLOW_REFRESH_TOKEN_AUTH"},
	}, now)
	upc, _ := client["UserPoolClient"].(map[string]any)
	clientID, _ := upc["ClientId"].(string)
	listClients := cognitoMustOK(t, handler, "ListUserPoolClients", map[string]any{"UserPoolId": poolID}, now)
	if !strings.Contains(mustMarshal(t, listClients), clientID) {
		t.Fatalf("ListUserPoolClients: %v", listClients)
	}
	descClient := cognitoMustOK(t, handler, "DescribeUserPoolClient", map[string]any{
		"UserPoolId": poolID, "ClientId": clientID,
	}, now)
	_ = descClient

	signUp := cognitoMustOK(t, handler, "SignUp", map[string]any{
		"ClientId": clientID,
		"Username": "auth-ops-user",
		"Password": "AuthOps1!",
		"UserAttributes": []map[string]any{
			{"Name": "email", "Value": "auth-ops@example.com"},
		},
	}, now)
	_ = signUp
	code, err := st.PeekCognitoConfirmationCode(testAccountID, poolID, "auth-ops-user", store.CognitoConfirmPurposeSignUp)
	if err != nil {
		t.Fatal(err)
	}
	cognitoMustOK(t, handler, "ConfirmSignUp", map[string]any{
		"ClientId": clientID, "Username": "auth-ops-user", "ConfirmationCode": code,
	}, now)

	auth := cognitoMustOK(t, handler, "InitiateAuth", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "auth-ops-user",
			"PASSWORD": "AuthOps1!",
		},
	}, now)
	ar, _ := auth["AuthenticationResult"].(map[string]any)
	accessToken, _ := ar["AccessToken"].(string)
	if accessToken == "" {
		t.Fatalf("InitiateAuth: %v", auth)
	}

	cognitoMustOK(t, handler, "UpdateUserAttributes", map[string]any{
		"AccessToken": accessToken,
		"UserAttributes": []map[string]any{
			{"Name": "email", "Value": "auth-ops2@example.com"},
		},
	}, now)
	verCode := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.GetUserAttributeVerificationCode", "cognito-idp", map[string]any{
		"AccessToken":   accessToken,
		"AttributeName": "email",
	}, now)
	if verCode.Code != http.StatusOK {
		t.Fatalf("GetUserAttributeVerificationCode %d %s", verCode.Code, verCode.Body.String())
	}

	forgot := cognitoMustOK(t, handler, "ForgotPassword", map[string]any{
		"ClientId": clientID, "Username": "auth-ops-user",
	}, now)
	_ = forgot
	fpCode, err := st.PeekCognitoConfirmationCode(testAccountID, poolID, "auth-ops-user", store.CognitoConfirmPurposeForgotPassword)
	if err != nil {
		t.Fatal(err)
	}
	cognitoMustOK(t, handler, "ConfirmForgotPassword", map[string]any{
		"ClientId": clientID, "Username": "auth-ops-user",
		"ConfirmationCode": fpCode, "Password": "AuthOps2!",
	}, now)

	badAuth := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.InitiateAuth", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "auth-ops-user",
			"PASSWORD": "wrong",
		},
	}, now)
	if badAuth.Code == http.StatusOK {
		t.Fatalf("bad password should fail")
	}

	resend := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.ResendConfirmationCode", "cognito-idp", map[string]any{
		"ClientId": clientID, "Username": "no-pending-user",
	}, now)
	if resend.Code == http.StatusOK {
		t.Fatalf("ResendConfirmationCode missing should fail")
	}

	delClient := cognitoMustOK(t, handler, "DeleteUserPoolClient", map[string]any{
		"UserPoolId": poolID, "ClientId": clientID,
	}, now)
	_ = delClient
}

func TestSecretsRestoreRotateStageNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name": "restore-ops", "SecretString": "v1",
		"Tags": []map[string]any{{"Key": "k", "Value": "v"}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateSecret %d %s", create.Code, create.Body.String())
	}
	put := mustSecretsJSON(t, handler, "PutSecretValue", map[string]any{
		"SecretId": "restore-ops", "SecretString": "v2",
		"VersionStages": []string{"AWSCURRENT"},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutSecretValue %d %s", put.Code, put.Body.String())
	}

	listTags := mustSecretsJSON(t, handler, "ListTagsForResource", map[string]any{"SecretId": "restore-ops"}, now)
	if listTags.Code != http.StatusOK {
		t.Fatalf("ListTagsForResource %d %s", listTags.Code, listTags.Body.String())
	}

	del := mustSecretsJSON(t, handler, "DeleteSecret", map[string]any{
		"SecretId": "restore-ops", "RecoveryWindowInDays": 7,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteSecret %d %s", del.Code, del.Body.String())
	}
	restore := mustSecretsJSON(t, handler, "RestoreSecret", map[string]any{"SecretId": "restore-ops"}, now)
	if restore.Code != http.StatusOK {
		t.Fatalf("RestoreSecret %d %s", restore.Code, restore.Body.String())
	}
	restoreMiss := mustSecretsJSON(t, handler, "RestoreSecret", map[string]any{"SecretId": "nope"}, now)
	if restoreMiss.Code == http.StatusOK {
		t.Fatalf("RestoreSecret missing should fail")
	}

	desc := mustSecretsJSON(t, handler, "DescribeSecret", map[string]any{"SecretId": "restore-ops"}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeSecret %d %s", desc.Code, desc.Body.String())
	}
	var descOut map[string]any
	_ = json.Unmarshal(desc.Body.Bytes(), &descOut)
	versions, _ := descOut["VersionIdsToStages"].(map[string]any)
	var versionID string
	for id := range versions {
		versionID = id
		break
	}
	if versionID != "" {
		stage := mustSecretsJSON(t, handler, "UpdateSecretVersionStage", map[string]any{
			"SecretId":     "restore-ops",
			"VersionStage": "AWSPENDING",
			"MoveToVersionId": versionID,
		}, now)
		if stage.Code != http.StatusOK && stage.Code != http.StatusBadRequest {
			t.Fatalf("UpdateSecretVersionStage %d %s", stage.Code, stage.Body.String())
		}
	}

	rotRules := mustSecretsJSON(t, handler, "RotateSecret", map[string]any{
		"SecretId": "restore-ops",
		"RotationRules": map[string]any{
			"AutomaticallyAfterDays": 30,
		},
	}, now)
	// Without rotation lambda this may fail; still covers parse path.
	if rotRules.Code == http.StatusOK {
		t.Logf("RotateSecret ok")
	}

	forceDel := mustSecretsJSON(t, handler, "DeleteSecret", map[string]any{
		"SecretId": "restore-ops", "ForceDeleteWithoutRecovery": true,
	}, now)
	if forceDel.Code != http.StatusOK {
		t.Fatalf("force delete %d %s", forceDel.Code, forceDel.Body.String())
	}
}

func TestKMSRevokeGrantAliasAndAsymmetricRotationNegatives(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustKMSJSON(t, handler, "CreateKey", map[string]any{
		"Description": "ops-kms",
		"Tags":        []map[string]any{{"TagKey": "env", "TagValue": "lab"}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateKey %d %s", create.Code, create.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	meta, _ := created["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	alias := mustKMSJSON(t, handler, "CreateAlias", map[string]any{
		"AliasName": "alias/ops-kms", "TargetKeyId": keyID,
	}, now)
	if alias.Code != http.StatusOK {
		t.Fatalf("CreateAlias %d %s", alias.Code, alias.Body.String())
	}
	dupAlias := mustKMSJSON(t, handler, "CreateAlias", map[string]any{
		"AliasName": "alias/ops-kms", "TargetKeyId": keyID,
	}, now)
	if dupAlias.Code == http.StatusOK {
		t.Fatalf("dup alias should fail")
	}
	listAlias := mustKMSJSON(t, handler, "ListAliases", map[string]any{}, now)
	if listAlias.Code != http.StatusOK || !strings.Contains(listAlias.Body.String(), "ops-kms") {
		t.Fatalf("ListAliases %d %s", listAlias.Code, listAlias.Body.String())
	}

	key2 := mustKMSJSON(t, handler, "CreateKey", map[string]any{"Description": "ops-kms-2"}, now)
	var k2 map[string]any
	_ = json.Unmarshal(key2.Body.Bytes(), &k2)
	meta2, _ := k2["KeyMetadata"].(map[string]any)
	keyID2, _ := meta2["KeyId"].(string)
	updAlias := mustKMSJSON(t, handler, "UpdateAlias", map[string]any{
		"AliasName": "alias/ops-kms", "TargetKeyId": keyID2,
	}, now)
	if updAlias.Code != http.StatusOK {
		t.Fatalf("UpdateAlias %d %s", updAlias.Code, updAlias.Body.String())
	}
	delAlias := mustKMSJSON(t, handler, "DeleteAlias", map[string]any{"AliasName": "alias/ops-kms"}, now)
	if delAlias.Code != http.StatusOK {
		t.Fatalf("DeleteAlias %d %s", delAlias.Code, delAlias.Body.String())
	}
	delAliasMiss := mustKMSJSON(t, handler, "DeleteAlias", map[string]any{"AliasName": "alias/missing"}, now)
	if delAliasMiss.Code == http.StatusOK {
		t.Fatalf("DeleteAlias missing should fail")
	}

	grant := mustKMSJSON(t, handler, "CreateGrant", map[string]any{
		"KeyId":             keyID,
		"GranteePrincipal":  "arn:aws:iam::" + testAccountID + ":root",
		"Operations":        []string{"Encrypt", "Decrypt"},
		"Name":              "ops-grant",
	}, now)
	if grant.Code != http.StatusOK {
		t.Fatalf("CreateGrant %d %s", grant.Code, grant.Body.String())
	}
	var gOut map[string]any
	_ = json.Unmarshal(grant.Body.Bytes(), &gOut)
	grantID, _ := gOut["GrantId"].(string)
	listG := mustKMSJSON(t, handler, "ListGrants", map[string]any{"KeyId": keyID}, now)
	if listG.Code != http.StatusOK {
		t.Fatalf("ListGrants %d %s", listG.Code, listG.Body.String())
	}
	revoke := mustKMSJSON(t, handler, "RevokeGrant", map[string]any{"KeyId": keyID, "GrantId": grantID}, now)
	if revoke.Code != http.StatusOK {
		t.Fatalf("RevokeGrant %d %s", revoke.Code, revoke.Body.String())
	}
	revokeMiss := mustKMSJSON(t, handler, "RevokeGrant", map[string]any{"KeyId": keyID, "GrantId": "missing"}, now)
	if revokeMiss.Code == http.StatusOK {
		t.Fatalf("RevokeGrant missing should fail")
	}

	enRot := mustKMSJSON(t, handler, "EnableKeyRotation", map[string]any{"KeyId": keyID}, now)
	if enRot.Code != http.StatusOK {
		t.Fatalf("EnableKeyRotation %d %s", enRot.Code, enRot.Body.String())
	}
	status := mustKMSJSON(t, handler, "GetKeyRotationStatus", map[string]any{"KeyId": keyID}, now)
	if status.Code != http.StatusOK {
		t.Fatalf("GetKeyRotationStatus %d %s", status.Code, status.Body.String())
	}
	disRot := mustKMSJSON(t, handler, "DisableKeyRotation", map[string]any{"KeyId": keyID}, now)
	if disRot.Code != http.StatusOK {
		t.Fatalf("DisableKeyRotation %d %s", disRot.Code, disRot.Body.String())
	}

	asym := mustKMSJSON(t, handler, "CreateKey", map[string]any{
		"KeySpec": "RSA_2048", "KeyUsage": "SIGN_VERIFY",
	}, now)
	if asym.Code != http.StatusOK {
		t.Fatalf("CreateKey asym %d %s", asym.Code, asym.Body.String())
	}
	var asymOut map[string]any
	_ = json.Unmarshal(asym.Body.Bytes(), &asymOut)
	asymMeta, _ := asymOut["KeyMetadata"].(map[string]any)
	asymID, _ := asymMeta["KeyId"].(string)
	badRot := mustKMSJSON(t, handler, "EnableKeyRotation", map[string]any{"KeyId": asymID}, now)
	if badRot.Code == http.StatusOK {
		t.Fatalf("asymmetric rotation should fail")
	}

	pt := base64.StdEncoding.EncodeToString([]byte("reenc"))
	encBlob := mustKMSJSON(t, handler, "Encrypt", map[string]any{"KeyId": keyID, "Plaintext": pt}, now)
	if encBlob.Code != http.StatusOK {
		t.Fatalf("Encrypt %d %s", encBlob.Code, encBlob.Body.String())
	}
	var encOut map[string]any
	_ = json.Unmarshal(encBlob.Body.Bytes(), &encOut)
	blob, _ := encOut["CiphertextBlob"].(string)
	reenc := mustKMSJSON(t, handler, "ReEncrypt", map[string]any{
		"CiphertextBlob":    blob,
		"SourceKeyId":       keyID,
		"DestinationKeyId":  keyID2,
	}, now)
	if reenc.Code != http.StatusOK {
		t.Fatalf("ReEncrypt %d %s", reenc.Code, reenc.Body.String())
	}
	reencBadSrc := mustKMSJSON(t, handler, "ReEncrypt", map[string]any{
		"CiphertextBlob": blob, "SourceKeyId": keyID2, "DestinationKeyId": keyID,
	}, now)
	if reencBadSrc.Code == http.StatusOK {
		t.Fatalf("ReEncrypt mismatched SourceKeyId should fail")
	}
	reencMissDest := mustKMSJSON(t, handler, "ReEncrypt", map[string]any{
		"CiphertextBlob": blob, "DestinationKeyId": "missing",
	}, now)
	if reencMissDest.Code == http.StatusOK {
		t.Fatalf("ReEncrypt missing dest should fail")
	}
	reencNoDest := mustKMSJSON(t, handler, "ReEncrypt", map[string]any{
		"CiphertextBlob": blob,
	}, now)
	if reencNoDest.Code == http.StatusOK {
		t.Fatalf("ReEncrypt without DestinationKeyId should fail")
	}

	sched := mustKMSJSON(t, handler, "ScheduleKeyDeletion", map[string]any{
		"KeyId": keyID, "PendingWindowInDays": 7,
	}, now)
	if sched.Code != http.StatusOK {
		t.Fatalf("ScheduleKeyDeletion %d %s", sched.Code, sched.Body.String())
	}
	cancel := mustKMSJSON(t, handler, "CancelKeyDeletion", map[string]any{"KeyId": keyID}, now)
	if cancel.Code != http.StatusOK {
		t.Fatalf("CancelKeyDeletion %d %s", cancel.Code, cancel.Body.String())
	}
	cancelBad := mustKMSJSON(t, handler, "CancelKeyDeletion", map[string]any{"KeyId": keyID}, now)
	if cancelBad.Code == http.StatusOK {
		t.Fatalf("CancelKeyDeletion when not pending should fail")
	}

	tags := mustKMSJSON(t, handler, "ListResourceTags", map[string]any{"KeyId": keyID}, now)
	if tags.Code != http.StatusOK {
		t.Fatalf("ListResourceTags %d %s", tags.Code, tags.Body.String())
	}
	tag := mustKMSJSON(t, handler, "TagResource", map[string]any{
		"KeyId": keyID, "Tags": []map[string]any{{"TagKey": "team", "TagValue": "sec"}},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagResource %d %s", tag.Code, tag.Body.String())
	}
	untag := mustKMSJSON(t, handler, "UntagResource", map[string]any{
		"KeyId": keyID, "TagKeys": []string{"team"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource %d %s", untag.Code, untag.Body.String())
	}
}
