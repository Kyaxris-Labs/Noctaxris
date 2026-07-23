package store_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSecretsRotationConfigAndEnqueue(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"

	created, err := st.CreateSecret(account, "us-east-1", "rot-lambda", "v1", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "secret-rotator",
		Runtime: store.LambdaRuntimePython312,
		RoleARN: "arn:aws:iam::" + account + ":role/rotator-exec",
		Handler: "app.handler",
		Zip:     testZip(t, map[string]string{"app.py": "def handler(e,c): return e"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSecretRotationConfig(account, created.Name, fn.FunctionARN, ""); err != nil {
		t.Fatal(err)
	}
	meta, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if meta.RotationLambdaARN != fn.FunctionARN {
		t.Fatalf("RotationLambdaARN=%q want %q", meta.RotationLambdaARN, fn.FunctionARN)
	}

	fnAccount, name, qual, passRole, err := st.ResolveSecretRotationLambda(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if fnAccount != account || name != "secret-rotator" || qual != "$LATEST" {
		t.Fatalf("resolve=%s %s %s", fnAccount, name, qual)
	}
	if passRole != fn.RoleARN {
		t.Fatalf("passRole=%q want %q", passRole, fn.RoleARN)
	}

	job, err := st.EnqueueSecretRotationInvoke(account, created.Name, "tok-1")
	if err != nil {
		t.Fatal(err)
	}
	if job.FunctionName != "secret-rotator" || job.Status != "pending" {
		t.Fatalf("job=%+v", job)
	}
	var evt map[string]string
	if err := json.Unmarshal([]byte(job.EventJSON), &evt); err != nil {
		t.Fatal(err)
	}
	if evt["Step"] != store.SecretRotationStepFinishSecret || evt["SecretId"] != created.ARN || evt["ClientRequestToken"] != "tok-1" {
		t.Fatalf("event=%v", evt)
	}

	before, err := st.GetSecretValue(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ProcessAsyncInvocation(job.InvocationID, 0, func() error {
		return errors.New("rotator boom")
	}); err != nil {
		t.Fatal(err)
	}
	failedJob, err := st.GetAsyncInvocation(job.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if failedJob.Status != "failed" {
		t.Fatalf("status=%q want failed", failedJob.Status)
	}
	afterFail, err := st.GetSecretValue(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if afterFail.SecretString != before.SecretString || afterFail.Version != before.Version {
		t.Fatalf("value changed after failed invoke: %+v vs %+v", afterFail, before)
	}

	job2, err := st.EnqueueSecretRotationInvoke(account, created.Name, "tok-2")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ProcessAsyncInvocation(job2.InvocationID, 0, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	okJob, err := st.GetAsyncInvocation(job2.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if okJob.Status != "succeeded" {
		t.Fatalf("status=%q want succeeded", okJob.Status)
	}
	finished, err := st.FinishSecretRotation(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if finished.SecretString == "" || finished.SecretString == before.SecretString || finished.Version != before.Version+1 {
		t.Fatalf("finish=%+v before=%+v", finished, before)
	}
}

func TestSecretsRotationInvalidLambdaARN(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"
	if _, err := st.CreateSecret(account, "us-east-1", "bad-rot", "x", nil, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSecretRotationConfig(account, "bad-rot", "arn:aws:lambda:us-east-1:"+account+":function:missing", ""); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, err := st.ResolveSecretRotationLambda(account, "bad-rot")
	if !errors.Is(err, store.ErrSecretRotationLambdaInvalid) {
		t.Fatalf("want ErrSecretRotationLambdaInvalid, got %v", err)
	}
}

func TestSecretRotationEventJSON(t *testing.T) {
	raw, err := store.SecretRotationEventJSON("arn:aws:secretsmanager:us-east-1:1:secret:x-abcdef", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"Step":"finishSecret"`) || !strings.Contains(raw, "SecretId") {
		t.Fatalf("event=%s", raw)
	}
}
