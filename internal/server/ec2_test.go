package server_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func mustEC2Query(t *testing.T, handler http.Handler, body string, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "ec2", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestEC2RunDescribeStopStartTerminateWithoutEngine(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	run := mustEC2Query(t, handler, strings.Join([]string{
		"Action=RunInstances",
		"Version=2016-11-15",
		"ImageId=ami-alpine",
		"InstanceType=t3.micro",
		"MinCount=1",
		"MaxCount=1",
	}, "&"), now)
	if run.Code != http.StatusOK {
		t.Fatalf("RunInstances status=%d body=%q", run.Code, run.Body.String())
	}
	body := run.Body.String()
	if !strings.Contains(body, "<name>pending</name>") {
		t.Fatalf("without DinD expect pending: %q", body)
	}
	re := regexp.MustCompile(`<instanceId>(i-[a-f0-9]+)</instanceId>`)
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		t.Fatalf("missing instanceId: %q", body)
	}
	id := m[1]

	desc := mustEC2Query(t, handler,
		"Action=DescribeInstances&Version=2016-11-15&InstanceId.1="+id, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), id) {
		t.Fatalf("DescribeInstances status=%d body=%q", desc.Code, desc.Body.String())
	}

	stop := mustEC2Query(t, handler,
		"Action=StopInstances&Version=2016-11-15&InstanceId.1="+id, now)
	if stop.Code != http.StatusOK || !strings.Contains(stop.Body.String(), "<name>stopped</name>") {
		t.Fatalf("StopInstances status=%d body=%q", stop.Code, stop.Body.String())
	}

	start := mustEC2Query(t, handler,
		"Action=StartInstances&Version=2016-11-15&InstanceId.1="+id, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartInstances status=%d body=%q", start.Code, start.Body.String())
	}
	// Without engine, Start leaves pending (no container to restart).
	if !strings.Contains(start.Body.String(), "<name>pending</name>") {
		t.Fatalf("Start without engine expect pending: %q", start.Body.String())
	}

	term := mustEC2Query(t, handler,
		"Action=TerminateInstances&Version=2016-11-15&InstanceId.1="+id, now)
	if term.Code != http.StatusOK || !strings.Contains(term.Body.String(), "<name>terminated</name>") {
		t.Fatalf("TerminateInstances status=%d body=%q", term.Code, term.Body.String())
	}
}

func TestEC2UnknownAMIFallsBackToAlpine(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	run := mustEC2Query(t, handler, strings.Join([]string{
		"Action=RunInstances",
		"Version=2016-11-15",
		"ImageId=ami-0realawsami12345",
		"MinCount=1",
		"MaxCount=1",
	}, "&"), now)
	if run.Code != http.StatusOK {
		t.Fatalf("RunInstances status=%d body=%q", run.Code, run.Body.String())
	}
	if !strings.Contains(run.Body.String(), "ami-0realawsami12345") {
		t.Fatalf("want request ImageId preserved: %q", run.Body.String())
	}
}

func TestEC2DescribeImagesCatalogAndFilter(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	all := mustEC2Query(t, handler, "Action=DescribeImages&Version=2016-11-15", now)
	if all.Code != http.StatusOK {
		t.Fatalf("DescribeImages status=%d body=%q", all.Code, all.Body.String())
	}
	body := all.Body.String()
	if !strings.Contains(body, "<imagesSet>") {
		t.Fatalf("want imagesSet: %q", body)
	}
	for _, id := range []string{"ami-alpine", "ami-amazonlinux2023", "ami-ubuntu2204"} {
		if !strings.Contains(body, "<imageId>"+id+"</imageId>") {
			t.Fatalf("missing %s: %q", id, body)
		}
	}
	if !strings.Contains(body, "<imageState>available</imageState>") {
		t.Fatalf("want available state: %q", body)
	}

	one := mustEC2Query(t, handler,
		"Action=DescribeImages&Version=2016-11-15&ImageId.1=ami-alpine", now)
	if one.Code != http.StatusOK {
		t.Fatalf("filter status=%d body=%q", one.Code, one.Body.String())
	}
	if strings.Count(one.Body.String(), "<imageId>") != 1 ||
		!strings.Contains(one.Body.String(), "ami-alpine") {
		t.Fatalf("want single ami-alpine: %q", one.Body.String())
	}

	alias := mustEC2Query(t, handler,
		"Action=DescribeImages&Version=2016-11-15&ImageId.1=ami-0abcdef123456789a", now)
	if alias.Code != http.StatusOK || !strings.Contains(alias.Body.String(), "ami-alpine") {
		t.Fatalf("alias filter: %q", alias.Body.String())
	}

	miss := mustEC2Query(t, handler,
		"Action=DescribeImages&Version=2016-11-15&ImageId.1=ami-does-not-exist", now)
	if miss.Code != http.StatusOK {
		t.Fatalf("unknown ImageId status=%d", miss.Code)
	}
	if strings.Contains(miss.Body.String(), "<imageId>") {
		t.Fatalf("unknown ImageId should return empty set: %q", miss.Body.String())
	}
}

func TestEC2VPCFlowStillWorksAlongsideInstances(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	run := mustEC2Query(t, handler, strings.Join([]string{
		"Action=RunInstances",
		"Version=2016-11-15",
		"ImageId=ami-alpine",
		"MinCount=1",
		"MaxCount=1",
	}, "&"), now)
	if run.Code != http.StatusOK {
		t.Fatalf("RunInstances status=%d", run.Code)
	}

	if _, err := st.CreateBucket(testAccountID, "vpc-flow-with-ec2"); err != nil {
		t.Fatal(err)
	}
	create := mustVPCFlowJSON(t, handler, "AmazonEC2.CreateFlowLogs", map[string]any{
		"ResourceIds":        []string{"vpc-labopaque002"},
		"ResourceType":       "VPC",
		"TrafficType":        "ALL",
		"LogDestinationType": "s3",
		"LogDestination":     "arn:aws:s3:::vpc-flow-with-ec2",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateFlowLogs status=%d body=%q", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "fl-") {
		t.Fatalf("expected FlowLogIds: %q", create.Body.String())
	}
}
