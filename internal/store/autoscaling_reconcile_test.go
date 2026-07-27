package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestClampASGDesired(t *testing.T) {
	if got := store.ClampASGDesired(5, 1, 3); got != 3 {
		t.Fatalf("clamp high=%d", got)
	}
	if got := store.ClampASGDesired(0, 1, 3); got != 1 {
		t.Fatalf("clamp low=%d", got)
	}
	if got := store.ClampASGDesired(2, 1, 3); got != 2 {
		t.Fatalf("clamp mid=%d", got)
	}
}

func TestASGReconcileCapacityMath(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	region := "us-east-1"

	_, err := st.CreateLaunchConfiguration(account, region, "lc-cap", "ami-alpine", "t3.micro", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateAutoScalingGroup(account, region, "asg-cap", "lc-cap", 1, 4, 2, []string{"us-east-1a"}, "", "EC2", 300)
	if err != nil {
		t.Fatal(err)
	}

	g, err := st.ReconcileASGCapacity(account, region, "asg-cap")
	if err != nil {
		t.Fatal(err)
	}
	if g.DesiredCapacity != 2 || len(g.Instances) != 2 {
		t.Fatalf("after scale-out desired=%d instances=%d %+v", g.DesiredCapacity, len(g.Instances), g.Instances)
	}
	for _, inst := range g.Instances {
		if inst.LifecycleState != "Pending" && inst.LifecycleState != "InService" {
			t.Fatalf("lifecycle=%q", inst.LifecycleState)
		}
		if inst.InstanceID == "" {
			t.Fatal("empty InstanceId")
		}
	}

	if err := st.SetDesiredCapacity(account, region, "asg-cap", 3); err != nil {
		t.Fatal(err)
	}
	g, err = st.ReconcileASGCapacity(account, region, "asg-cap")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Instances) != 3 {
		t.Fatalf("scale to 3 got %d", len(g.Instances))
	}

	if err := st.SetDesiredCapacity(account, region, "asg-cap", 1); err != nil {
		t.Fatal(err)
	}
	g, err = st.ReconcileASGCapacity(account, region, "asg-cap")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Instances) != 1 {
		t.Fatalf("scale to 1 got %d", len(g.Instances))
	}

	if err := st.DeleteAutoScalingGroup(account, region, "asg-cap", false); !errors.Is(err, store.ErrASGBadRequest) {
		t.Fatalf("delete without force with instances: %v", err)
	}
	if err := st.DeleteAutoScalingGroup(account, region, "asg-cap", true); err != nil {
		t.Fatal(err)
	}
	list, err := st.DescribeAutoScalingGroups(account, region, []string{"asg-cap"})
	if err != nil || len(list) != 0 {
		t.Fatalf("after force delete list=%+v err=%v", list, err)
	}
}

func TestASGCreateClampsDesired(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	_, err := st.CreateLaunchConfiguration(account, "us-east-1", "lc-clamp", "ami-123", "t3.micro", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	g, err := st.CreateAutoScalingGroup(account, "us-east-1", "asg-clamp", "lc-clamp", 1, 2, 9, []string{"us-east-1a"}, "", "EC2", 300)
	if err != nil {
		t.Fatal(err)
	}
	if g.DesiredCapacity != 2 {
		t.Fatalf("desired=%d want clamped 2", g.DesiredCapacity)
	}
}
