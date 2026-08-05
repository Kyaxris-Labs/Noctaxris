package server_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBeanstalkDescribeDeleteVersionEnvironments(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	emptyApps := mustBeanstalkQuery(t, handler, "Action=DescribeApplications&Version=2010-12-01", now)
	if emptyApps.Code != http.StatusOK {
		t.Fatalf("DescribeApplications empty status=%d body=%q", emptyApps.Code, emptyApps.Body.String())
	}

	createApp := mustBeanstalkQuery(t, handler,
		"Action=CreateApplication&Version=2010-12-01&ApplicationName=cov-app&Description=lab", now)
	if createApp.Code != http.StatusOK {
		t.Fatalf("CreateApplication status=%d body=%q", createApp.Code, createApp.Body.String())
	}

	descApps := mustBeanstalkQuery(t, handler,
		"Action=DescribeApplications&Version=2010-12-01&ApplicationNames.member.1=cov-app", now)
	if descApps.Code != http.StatusOK || !strings.Contains(descApps.Body.String(), "cov-app") {
		t.Fatalf("DescribeApplications status=%d body=%q", descApps.Code, descApps.Body.String())
	}

	createVer := mustBeanstalkQuery(t, handler, strings.Join([]string{
		"Action=CreateApplicationVersion",
		"Version=2010-12-01",
		"ApplicationName=cov-app",
		"VersionLabel=v1",
		"Description=first",
		"SourceBundle.S3Bucket=lab-bucket",
		"SourceBundle.S3Key=app.zip",
	}, "&"), now)
	if createVer.Code != http.StatusOK || !strings.Contains(createVer.Body.String(), "v1") {
		t.Fatalf("CreateApplicationVersion status=%d body=%q", createVer.Code, createVer.Body.String())
	}

	dupVer := mustBeanstalkQuery(t, handler, strings.Join([]string{
		"Action=CreateApplicationVersion",
		"Version=2010-12-01",
		"ApplicationName=cov-app",
		"VersionLabel=v1",
	}, "&"), now)
	if dupVer.Code != http.StatusBadRequest {
		t.Fatalf("CreateApplicationVersion duplicate want 400 status=%d body=%q", dupVer.Code, dupVer.Body.String())
	}

	missingAppVer := mustBeanstalkQuery(t, handler, strings.Join([]string{
		"Action=CreateApplicationVersion",
		"Version=2010-12-01",
		"ApplicationName=no-app",
		"VersionLabel=v1",
	}, "&"), now)
	if missingAppVer.Code != http.StatusBadRequest {
		t.Fatalf("CreateApplicationVersion missing app want 400 status=%d body=%q", missingAppVer.Code, missingAppVer.Body.String())
	}

	emptyEnvs := mustBeanstalkQuery(t, handler, "Action=DescribeEnvironments&Version=2010-12-01", now)
	if emptyEnvs.Code != http.StatusOK {
		t.Fatalf("DescribeEnvironments empty status=%d body=%q", emptyEnvs.Code, emptyEnvs.Body.String())
	}

	createEnv := mustBeanstalkQuery(t, handler, strings.Join([]string{
		"Action=CreateEnvironment",
		"Version=2010-12-01",
		"ApplicationName=cov-app",
		"EnvironmentName=cov-env",
		"VersionLabel=v1",
	}, "&"), now)
	if createEnv.Code != http.StatusOK {
		t.Fatalf("CreateEnvironment status=%d body=%q", createEnv.Code, createEnv.Body.String())
	}

	descEnvs := mustBeanstalkQuery(t, handler, strings.Join([]string{
		"Action=DescribeEnvironments",
		"Version=2010-12-01",
		"ApplicationName=cov-app",
		"EnvironmentNames.member.1=cov-env",
	}, "&"), now)
	if descEnvs.Code != http.StatusOK || !strings.Contains(descEnvs.Body.String(), "cov-env") {
		t.Fatalf("DescribeEnvironments status=%d body=%q", descEnvs.Code, descEnvs.Body.String())
	}

	delBlocked := mustBeanstalkQuery(t, handler,
		"Action=DeleteApplication&Version=2010-12-01&ApplicationName=cov-app", now)
	if delBlocked.Code != http.StatusBadRequest {
		t.Fatalf("DeleteApplication with env want 400 status=%d body=%q", delBlocked.Code, delBlocked.Body.String())
	}

	term := mustBeanstalkQuery(t, handler,
		"Action=TerminateEnvironment&Version=2010-12-01&EnvironmentName=cov-env", now)
	if term.Code != http.StatusOK {
		t.Fatalf("TerminateEnvironment status=%d body=%q", term.Code, term.Body.String())
	}

	descDeleted := mustBeanstalkQuery(t, handler, strings.Join([]string{
		"Action=DescribeEnvironments",
		"Version=2010-12-01",
		"ApplicationName=cov-app",
		"IncludeDeleted=true",
	}, "&"), now)
	if descDeleted.Code != http.StatusOK || !strings.Contains(descDeleted.Body.String(), "Terminated") {
		t.Fatalf("DescribeEnvironments IncludeDeleted status=%d body=%q", descDeleted.Code, descDeleted.Body.String())
	}

	del := mustBeanstalkQuery(t, handler,
		"Action=DeleteApplication&Version=2010-12-01&ApplicationName=cov-app", now)
	if del.Code != http.StatusOK || !strings.Contains(del.Body.String(), "DeleteApplicationResponse") {
		t.Fatalf("DeleteApplication status=%d body=%q", del.Code, del.Body.String())
	}

	delGone := mustBeanstalkQuery(t, handler,
		"Action=DeleteApplication&Version=2010-12-01&ApplicationName=cov-app", now)
	if delGone.Code != http.StatusBadRequest || !strings.Contains(delGone.Body.String(), "InvalidParameterValue") {
		t.Fatalf("DeleteApplication missing want InvalidParameterValue status=%d body=%q", delGone.Code, delGone.Body.String())
	}
}

func TestBeanstalkUnknownActionError(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustBeanstalkQuery(t, handler, "Action=SwapEnvironmentCNAMEs&Version=2010-12-01", now)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "InvalidAction") {
		t.Fatalf("unknown Beanstalk action want InvalidAction status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestBeanstalkCreateApplicationMissingName(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustBeanstalkQuery(t, handler, "Action=CreateApplication&Version=2010-12-01&ApplicationName=", now)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("CreateApplication empty name want 400 status=%d body=%q", rec.Code, rec.Body.String())
	}
}
