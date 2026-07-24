package sdk_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

func newCognito(t *testing.T, cfg aws.Config) *cognitoidentityprovider.Client {
	t.Helper()
	return cognitoidentityprovider.NewFromConfig(cfg, func(o *cognitoidentityprovider.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func TestCognitoUSER_SRP_AUTHRoundTrip(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newCognito(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	poolOut, err := client.CreateUserPool(ctx, &cognitoidentityprovider.CreateUserPoolInput{
		PoolName: aws.String("sdk-srp-" + prefix),
	})
	if err != nil {
		t.Fatalf("CreateUserPool: %v", err)
	}
	poolID := aws.ToString(poolOut.UserPool.Id)
	t.Cleanup(func() {
		_, _ = client.DeleteUserPool(ctx, &cognitoidentityprovider.DeleteUserPoolInput{
			UserPoolId: aws.String(poolID),
		})
	})

	clientOut, err := client.CreateUserPoolClient(ctx, &cognitoidentityprovider.CreateUserPoolClientInput{
		UserPoolId: aws.String(poolID),
		ClientName: aws.String("sdk-srp-app"),
	})
	if err != nil {
		t.Fatalf("CreateUserPoolClient: %v", err)
	}
	clientID := aws.ToString(clientOut.UserPoolClient.ClientId)

	const password = "Secret1!"
	username := "srp-" + prefix
	_, err = client.AdminCreateUser(ctx, &cognitoidentityprovider.AdminCreateUserInput{
		UserPoolId:        aws.String(poolID),
		Username:          aws.String(username),
		TemporaryPassword: aws.String(password),
	})
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	srpClient, err := newSDKSRPClient(poolID, username, password)
	if err != nil {
		t.Fatalf("newSDKSRPClient: %v", err)
	}
	initOut, err := client.InitiateAuth(ctx, &cognitoidentityprovider.InitiateAuthInput{
		ClientId: aws.String(clientID),
		AuthFlow: types.AuthFlowTypeUserSrpAuth,
		AuthParameters: map[string]string{
			"USERNAME": username,
			"SRP_A":    srpClient.srpAHex(),
		},
	})
	if err != nil {
		t.Fatalf("InitiateAuth USER_SRP_AUTH: %v", err)
	}
	if initOut.ChallengeName != types.ChallengeNameTypePasswordVerifier {
		t.Fatalf("ChallengeName=%v want PASSWORD_VERIFIER", initOut.ChallengeName)
	}
	responses, err := srpClient.passwordVerifierResponses(initOut.ChallengeParameters, time.Now().UTC())
	if err != nil {
		t.Fatalf("passwordVerifierResponses: %v", err)
	}
	respondOut, err := client.RespondToAuthChallenge(ctx, &cognitoidentityprovider.RespondToAuthChallengeInput{
		ClientId:           aws.String(clientID),
		ChallengeName:      types.ChallengeNameTypePasswordVerifier,
		Session:            initOut.Session,
		ChallengeResponses: responses,
	})
	if err != nil {
		t.Fatalf("RespondToAuthChallenge: %v", err)
	}
	if respondOut.AuthenticationResult == nil ||
		aws.ToString(respondOut.AuthenticationResult.AccessToken) == "" ||
		aws.ToString(respondOut.AuthenticationResult.IdToken) == "" {
		t.Fatalf("missing tokens: %+v", respondOut.AuthenticationResult)
	}
}

func TestCognitoUpdateUserPoolLambdaConfig(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newCognito(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	poolOut, err := client.CreateUserPool(ctx, &cognitoidentityprovider.CreateUserPoolInput{
		PoolName: aws.String("sdk-trig-" + prefix),
	})
	if err != nil {
		t.Fatalf("CreateUserPool: %v", err)
	}
	poolID := aws.ToString(poolOut.UserPool.Id)
	t.Cleanup(func() {
		_, _ = client.DeleteUserPool(ctx, &cognitoidentityprovider.DeleteUserPoolInput{
			UserPoolId: aws.String(poolID),
		})
	})

	lambdaARN := "arn:aws:lambda:us-east-1:000000000001:function:pre-signup-" + prefix
	_, err = client.UpdateUserPool(ctx, &cognitoidentityprovider.UpdateUserPoolInput{
		UserPoolId: aws.String(poolID),
		LambdaConfig: &types.LambdaConfigType{
			PreSignUp: aws.String(lambdaARN),
		},
	})
	if err != nil {
		t.Fatalf("UpdateUserPool: %v", err)
	}
	desc, err := client.DescribeUserPool(ctx, &cognitoidentityprovider.DescribeUserPoolInput{
		UserPoolId: aws.String(poolID),
	})
	if err != nil {
		t.Fatalf("DescribeUserPool: %v", err)
	}
	if desc.UserPool == nil || desc.UserPool.LambdaConfig == nil ||
		aws.ToString(desc.UserPool.LambdaConfig.PreSignUp) != lambdaARN {
		t.Fatalf("LambdaConfig.PreSignUp=%v want %s", desc.UserPool.LambdaConfig, lambdaARN)
	}
}

func TestCognitoLambdaConfigTriggerFailClosed(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newCognito(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	poolOut, err := client.CreateUserPool(ctx, &cognitoidentityprovider.CreateUserPoolInput{
		PoolName: aws.String("sdk-failtrig-" + prefix),
	})
	if err != nil {
		t.Fatalf("CreateUserPool: %v", err)
	}
	poolID := aws.ToString(poolOut.UserPool.Id)
	t.Cleanup(func() {
		_, _ = client.DeleteUserPool(ctx, &cognitoidentityprovider.DeleteUserPoolInput{UserPoolId: aws.String(poolID)})
	})

	// AWS SDK omits SigV4 for SignUp (service auth_type none). Use a signed admin
	// auth path with PreAuthentication so fail-closed UnexpectedLambdaException is observable.
	missingARN := "arn:aws:lambda:us-east-1:000000000001:function:missing-" + prefix
	_, err = client.UpdateUserPool(ctx, &cognitoidentityprovider.UpdateUserPoolInput{
		UserPoolId: aws.String(poolID),
		LambdaConfig: &types.LambdaConfigType{
			PreAuthentication: aws.String(missingARN),
		},
	})
	if err != nil {
		t.Fatalf("UpdateUserPool LambdaConfig: %v", err)
	}

	app, err := client.CreateUserPoolClient(ctx, &cognitoidentityprovider.CreateUserPoolClientInput{
		UserPoolId: aws.String(poolID),
		ClientName: aws.String("app"),
	})
	if err != nil {
		t.Fatalf("CreateUserPoolClient: %v", err)
	}
	clientID := aws.ToString(app.UserPoolClient.ClientId)

	const password = "Secret1!"
	username := "u-" + prefix
	_, err = client.AdminCreateUser(ctx, &cognitoidentityprovider.AdminCreateUserInput{
		UserPoolId:        aws.String(poolID),
		Username:          aws.String(username),
		TemporaryPassword: aws.String(password),
	})
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	_, err = client.AdminInitiateAuth(ctx, &cognitoidentityprovider.AdminInitiateAuthInput{
		UserPoolId: aws.String(poolID),
		ClientId:   aws.String(clientID),
		AuthFlow:   types.AuthFlowTypeAdminUserPasswordAuth,
		AuthParameters: map[string]string{
			"USERNAME": username,
			"PASSWORD": password,
		},
	})
	if err == nil {
		t.Fatal("expected UnexpectedLambdaException when PreAuthentication ARN missing")
	}
	if !strings.Contains(err.Error(), "UnexpectedLambdaException") {
		t.Fatalf("want UnexpectedLambdaException, got %v", err)
	}
}
