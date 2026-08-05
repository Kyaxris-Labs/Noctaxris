package cloudformation_test

import (
	"strings"
	"testing"

	cfnsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cloudformation"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCloudFormationXML(t *testing.T) {
	req := "req-cfn"
	if _, err := cfnsvc.CreateStackXML("stack-id", req); err != nil {
		t.Fatal(err)
	}
	if _, err := cfnsvc.DeleteStackXML(req); err != nil {
		t.Fatal(err)
	}
	stack := store.CFNStack{
		StackID: "arn:aws:cloudformation:us-east-1:1:stack/lab/id", StackName: "lab",
		Status: "CREATE_COMPLETE", TemplateBody: `{"Resources":{}}`,
	}
	desc, err := cfnsvc.DescribeStacksXML([]store.CFNStack{stack}, req)
	if err != nil || !strings.Contains(string(desc), "lab") {
		t.Fatalf("describe: %s %v", desc, err)
	}
	if _, err := cfnsvc.ListStacksXML([]store.CFNStack{stack}, req); err != nil {
		t.Fatal(err)
	}
	if _, err := cfnsvc.CreateChangeSetXML("cs-1", stack.StackID, req); err != nil {
		t.Fatal(err)
	}
	if _, err := cfnsvc.ExecuteChangeSetXML(req); err != nil {
		t.Fatal(err)
	}
	if _, err := cfnsvc.UpdateStackXML(stack.StackID, req); err != nil {
		t.Fatal(err)
	}
	cs := store.CFNChangeSet{ChangeSetID: "cs-1", StackID: stack.StackID, Status: "CREATE_COMPLETE"}
	if _, err := cfnsvc.DescribeChangeSetXML(cs, req); err != nil {
		t.Fatal(err)
	}
	if _, err := cfnsvc.DetectStackDriftXML("drift-1", req); err != nil {
		t.Fatal(err)
	}
}
