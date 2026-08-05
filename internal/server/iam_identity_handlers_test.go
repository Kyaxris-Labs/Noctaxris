package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIAMIdentityHandlersNegativeCoverage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	emptyGroup := iamForm(t, handler, "Action=CreateGroup&Version=2010-05-08&GroupName=", now)
	if emptyGroup.Code != http.StatusBadRequest {
		t.Fatalf("CreateGroup empty want 400 got %d", emptyGroup.Code)
	}

	rec := iamForm(t, handler, "Action=CreateGroup&Version=2010-05-08&GroupName=iam-id-cov", now)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateGroup %d", rec.Code)
	}
	dup := iamForm(t, handler, "Action=CreateGroup&Version=2010-05-08&GroupName=iam-id-cov", now)
	if dup.Code != http.StatusConflict {
		t.Fatalf("CreateGroup dup want 409 got %d", dup.Code)
	}

	listGroups := iamForm(t, handler, "Action=ListGroups&Version=2010-05-08", now)
	if listGroups.Code != http.StatusOK || !strings.Contains(listGroups.Body.String(), "iam-id-cov") {
		t.Fatalf("ListGroups %d", listGroups.Code)
	}

	missGroup := iamForm(t, handler, "Action=GetGroup&Version=2010-05-08&GroupName=missing-group", now)
	if missGroup.Code != http.StatusNotFound {
		t.Fatalf("GetGroup missing want 404 got %d", missGroup.Code)
	}

	addMissing := iamForm(t, handler, "Action=AddUserToGroup&Version=2010-05-08&GroupName=iam-id-cov&UserName=nouser", now)
	if addMissing.Code != http.StatusNotFound {
		t.Fatalf("AddUserToGroup missing user want 404 got %d", addMissing.Code)
	}

	_ = iamForm(t, handler, "Action=CreateUser&Version=2010-05-08&UserName=iam-id-user", now)
	add := iamForm(t, handler, "Action=AddUserToGroup&Version=2010-05-08&GroupName=iam-id-cov&UserName=iam-id-user", now)
	if add.Code != http.StatusOK {
		t.Fatalf("AddUserToGroup %d", add.Code)
	}
	rem := iamForm(t, handler, "Action=RemoveUserFromGroup&Version=2010-05-08&GroupName=iam-id-cov&UserName=iam-id-user", now)
	if rem.Code != http.StatusOK {
		t.Fatalf("RemoveUserFromGroup %d", rem.Code)
	}

	policyDoc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:ListBucket","Resource":"*"}]}`)
	pol := iamForm(t, handler, "Action=CreatePolicy&Version=2010-05-08&PolicyName=iam-id-pol&PolicyDocument="+policyDoc, now)
	if pol.Code != http.StatusOK {
		t.Fatalf("CreatePolicy %d", pol.Code)
	}
	policyARN := xmlTag(t, pol.Body.String(), "Arn")
	attach := iamForm(t, handler, "Action=AttachGroupPolicy&Version=2010-05-08&GroupName=iam-id-cov&PolicyArn="+url.QueryEscape(policyARN), now)
	if attach.Code != http.StatusOK {
		t.Fatalf("AttachGroupPolicy %d", attach.Code)
	}
	detach := iamForm(t, handler, "Action=DetachGroupPolicy&Version=2010-05-08&GroupName=iam-id-cov&PolicyArn="+url.QueryEscape(policyARN), now)
	if detach.Code != http.StatusOK {
		t.Fatalf("DetachGroupPolicy %d", detach.Code)
	}

	inline := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]}`)
	putInline := iamForm(t, handler, "Action=PutGroupPolicy&Version=2010-05-08&GroupName=iam-id-cov&PolicyName=g-inline&PolicyDocument="+inline, now)
	if putInline.Code != http.StatusOK {
		t.Fatalf("PutGroupPolicy %d", putInline.Code)
	}
	getInline := iamForm(t, handler, "Action=GetGroupPolicy&Version=2010-05-08&GroupName=iam-id-cov&PolicyName=g-inline", now)
	if getInline.Code != http.StatusOK {
		t.Fatalf("GetGroupPolicy %d", getInline.Code)
	}
	delInline := iamForm(t, handler, "Action=DeleteGroupPolicy&Version=2010-05-08&GroupName=iam-id-cov&PolicyName=g-inline", now)
	if delInline.Code != http.StatusOK {
		t.Fatalf("DeleteGroupPolicy %d", delInline.Code)
	}

	delGroup := iamForm(t, handler, "Action=DeleteGroup&Version=2010-05-08&GroupName=iam-id-cov", now)
	if delGroup.Code != http.StatusOK {
		t.Fatalf("DeleteGroup %d", delGroup.Code)
	}
}
