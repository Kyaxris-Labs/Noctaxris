package elasticbeanstalk_test

import (
	"strings"
	"testing"

	ebsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/elasticbeanstalk"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestElasticBeanstalkXML(t *testing.T) {
	req := "req-eb"
	if _, err := ebsvc.EmptyOKXML("UpdateEnvironment", req); err != nil {
		t.Fatal(err)
	}
	app := store.BeanstalkApplication{ApplicationName: "lab-app", Description: "lab"}
	if _, err := ebsvc.CreateApplicationXML(app, req); err != nil {
		t.Fatal(err)
	}
	desc, err := ebsvc.DescribeApplicationsXML([]store.BeanstalkApplication{app}, req)
	if err != nil || !strings.Contains(string(desc), "lab-app") {
		t.Fatalf("describe apps: %s %v", desc, err)
	}
	ver := store.BeanstalkApplicationVersion{
		ApplicationName: app.ApplicationName, VersionLabel: "v1", S3Bucket: "b", S3Key: "k",
	}
	if _, err := ebsvc.CreateApplicationVersionXML(ver, req); err != nil {
		t.Fatal(err)
	}
	env := store.BeanstalkEnvironment{
		EnvironmentName: "lab-env", ApplicationName: app.ApplicationName, Status: "Ready",
		Health: "Green", CNAME: "lab.noctaxris.local",
	}
	if _, err := ebsvc.CreateEnvironmentXML(env, req); err != nil {
		t.Fatal(err)
	}
	if _, err := ebsvc.DescribeEnvironmentsXML([]store.BeanstalkEnvironment{env}, req); err != nil {
		t.Fatal(err)
	}
	if _, err := ebsvc.TerminateEnvironmentResponseXML(env, req); err != nil {
		t.Fatal(err)
	}
	if _, err := ebsvc.ListAvailableSolutionStacksXML([]string{"64bit Amazon Linux 2"}, req); err != nil {
		t.Fatal(err)
	}
}
