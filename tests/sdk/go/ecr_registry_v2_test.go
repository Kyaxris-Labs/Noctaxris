package sdk_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func newECR(t *testing.T, cfg aws.Config) *ecr.Client {
	t.Helper()
	return ecr.NewFromConfig(cfg, func(o *ecr.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func TestRegistryV2ChunkedBlobPatchUploadLive(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	ecrClient := newECR(t, cfg)
	stsClient := newSTS(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	httpClient := &http.Client{Timeout: 30 * time.Second}

	caller, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		t.Fatalf("GetCallerIdentity: %v", err)
	}
	account := aws.ToString(caller.Account)
	if account == "" {
		t.Fatal("GetCallerIdentity missing Account")
	}

	repo := "sdk-chunked-" + prefix
	_, err = ecrClient.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName: aws.String(repo),
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ecrClient.DeleteRepository(ctx, &ecr.DeleteRepositoryInput{
			RepositoryName: aws.String(repo),
			Force:          true,
		})
	})

	auth := liveRegistryAuthHeader(t, ctx, ecrClient)

	part1 := []byte("noctaxris-chunk-one-")
	part2 := []byte("and-chunk-two")
	full := append(append([]byte{}, part1...), part2...)
	digest := "sha256:" + sha256HexLive(full)
	ep := endpoint()

	uploadReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/v2/%s/%s/blobs/uploads/", ep, account, repo), nil)
	if err != nil {
		t.Fatalf("upload NewRequest: %v", err)
	}
	uploadReq.Header.Set("Authorization", auth)
	uploadResp, err := httpClient.Do(uploadReq)
	if err != nil {
		t.Fatalf("blob upload start: %v", err)
	}
	_, _ = io.Copy(io.Discard, uploadResp.Body)
	_ = uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusAccepted {
		t.Fatalf("blob upload start status=%d", uploadResp.StatusCode)
	}
	location := uploadResp.Header.Get("Location")
	if location == "" {
		t.Fatal("missing upload Location")
	}

	patch1Req, err := http.NewRequest(http.MethodPatch, absoluteURL(ep, location), bytes.NewReader(part1))
	if err != nil {
		t.Fatalf("PATCH1 NewRequest: %v", err)
	}
	patch1Req.Header.Set("Authorization", auth)
	patch1Req.Header.Set("Content-Type", "application/octet-stream")
	patch1Req.Header.Set("Content-Range", fmt.Sprintf("0-%d", len(part1)-1))
	patch1Resp, err := httpClient.Do(patch1Req)
	if err != nil {
		t.Fatalf("PATCH chunk1: %v", err)
	}
	_, _ = io.Copy(io.Discard, patch1Resp.Body)
	_ = patch1Resp.Body.Close()
	if patch1Resp.StatusCode != http.StatusAccepted {
		t.Fatalf("PATCH chunk1 status=%d", patch1Resp.StatusCode)
	}
	if got := patch1Resp.Header.Get("Range"); got != fmt.Sprintf("0-%d", len(part1)-1) {
		t.Fatalf("PATCH chunk1 Range=%q want 0-%d", got, len(part1)-1)
	}
	location = patch1Resp.Header.Get("Location")
	if location == "" {
		t.Fatal("missing Location after PATCH chunk1")
	}

	patch2Req, err := http.NewRequest(http.MethodPatch, absoluteURL(ep, location), bytes.NewReader(part2))
	if err != nil {
		t.Fatalf("PATCH2 NewRequest: %v", err)
	}
	patch2Req.Header.Set("Authorization", auth)
	patch2Req.Header.Set("Content-Type", "application/octet-stream")
	patch2Req.Header.Set("Content-Range", fmt.Sprintf("%d-%d", len(part1), len(full)-1))
	patch2Resp, err := httpClient.Do(patch2Req)
	if err != nil {
		t.Fatalf("PATCH chunk2: %v", err)
	}
	_, _ = io.Copy(io.Discard, patch2Resp.Body)
	_ = patch2Resp.Body.Close()
	if patch2Resp.StatusCode != http.StatusAccepted {
		t.Fatalf("PATCH chunk2 status=%d", patch2Resp.StatusCode)
	}
	if got := patch2Resp.Header.Get("Range"); got != fmt.Sprintf("0-%d", len(full)-1) {
		t.Fatalf("PATCH chunk2 Range=%q want 0-%d", got, len(full)-1)
	}
	location = patch2Resp.Header.Get("Location")
	if location == "" {
		t.Fatal("missing Location after PATCH chunk2")
	}

	putReq, err := http.NewRequest(http.MethodPut, absoluteURL(ep, location)+"?digest="+digest, nil)
	if err != nil {
		t.Fatalf("PUT NewRequest: %v", err)
	}
	putReq.Header.Set("Authorization", auth)
	putResp, err := httpClient.Do(putReq)
	if err != nil {
		t.Fatalf("PUT finalize: %v", err)
	}
	_, _ = io.Copy(io.Discard, putResp.Body)
	_ = putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT finalize status=%d", putResp.StatusCode)
	}
	if putResp.Header.Get("Docker-Content-Digest") != digest {
		t.Fatalf("digest header=%q want %q", putResp.Header.Get("Docker-Content-Digest"), digest)
	}

	getReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v2/%s/%s/blobs/%s", ep, account, repo, digest), nil)
	if err != nil {
		t.Fatalf("GET NewRequest: %v", err)
	}
	getReq.Header.Set("Authorization", auth)
	getResp, err := httpClient.Do(getReq)
	if err != nil {
		t.Fatalf("GET blob: %v", err)
	}
	got, err := io.ReadAll(getResp.Body)
	_ = getResp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET blob status=%d body=%q", getResp.StatusCode, got)
	}
	if !bytes.Equal(got, full) {
		t.Fatalf("blob bytes mismatch got=%q want=%q", got, full)
	}
}

func liveRegistryAuthHeader(t *testing.T, ctx context.Context, client *ecr.Client) string {
	t.Helper()
	out, err := client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		t.Fatalf("GetAuthorizationToken: %v", err)
	}
	if len(out.AuthorizationData) == 0 || out.AuthorizationData[0].AuthorizationToken == nil {
		t.Fatal("GetAuthorizationToken missing authorizationData")
	}
	raw, err := base64.StdEncoding.DecodeString(aws.ToString(out.AuthorizationData[0].AuthorizationToken))
	if err != nil {
		t.Fatalf("decode authorizationToken: %v", err)
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		t.Fatalf("authorizationToken=%q", string(raw))
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte("AWS:"+parts[1]))
}

func sha256HexLive(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func absoluteURL(ep, location string) string {
	if strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://") {
		return location
	}
	if strings.HasPrefix(location, "/") {
		return ep + location
	}
	return ep + "/" + location
}
