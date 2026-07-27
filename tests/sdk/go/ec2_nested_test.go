package sdk_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestEC2RunStopStartTerminatePendingOK(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	c := ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
	ctx := context.Background()

	run, err := c.RunInstances(ctx, &ec2.RunInstancesInput{
		ImageId:      aws.String("ami-alpine"),
		InstanceType: types.InstanceTypeT3Micro,
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
	})
	if err != nil {
		t.Fatalf("RunInstances: %v", err)
	}
	if len(run.Instances) != 1 || run.Instances[0].InstanceId == nil {
		t.Fatalf("instances=%+v", run.Instances)
	}
	id := aws.ToString(run.Instances[0].InstanceId)
	if !strings.HasPrefix(id, "i-") {
		t.Fatalf("instance id=%q", id)
	}
	t.Cleanup(func() {
		_, _ = c.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: []string{id},
		})
	})

	desc, err := c.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{id},
	})
	if err != nil {
		t.Fatalf("DescribeInstances: %v", err)
	}
	if len(desc.Reservations) == 0 || len(desc.Reservations[0].Instances) == 0 {
		t.Fatal("DescribeInstances empty")
	}
	state := ""
	if desc.Reservations[0].Instances[0].State != nil {
		state = string(desc.Reservations[0].Instances[0].State.Name)
	}
	if state != "pending" && state != "running" {
		t.Fatalf("after Run want pending|running got %q", state)
	}

	stop, err := c.StopInstances(ctx, &ec2.StopInstancesInput{InstanceIds: []string{id}})
	if err != nil {
		t.Fatalf("StopInstances: %v", err)
	}
	if len(stop.StoppingInstances) < 1 {
		t.Fatal("expected StoppingInstances")
	}

	start, err := c.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: []string{id}})
	if err != nil {
		t.Fatalf("StartInstances: %v", err)
	}
	if len(start.StartingInstances) < 1 {
		t.Fatal("expected StartingInstances")
	}
	// Without noctaxris-engine, Start leaves pending; with engine, running.
	started := ""
	if start.StartingInstances[0].CurrentState != nil {
		started = string(start.StartingInstances[0].CurrentState.Name)
	}
	if started != "pending" && started != "running" {
		t.Fatalf("after Start want pending|running got %q", started)
	}

	term, err := c.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{id}})
	if err != nil {
		t.Fatalf("TerminateInstances: %v", err)
	}
	if len(term.TerminatingInstances) < 1 {
		t.Fatal("expected TerminatingInstances")
	}
}
