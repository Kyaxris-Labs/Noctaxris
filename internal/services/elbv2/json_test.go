package elbv2_test

import (
	"encoding/json"
	"testing"

	elbv2svc "github.com/Kyaxris-Labs/Noctaxris/internal/services/elbv2"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestELBv2JSON(t *testing.T) {
	lb := store.ELBv2LoadBalancer{ARN: "arn:lb", Name: "nlb", DNSName: "nlb.local", Type: "network", Scheme: "internal"}
	raw, err := elbv2svc.CreateLoadBalancerJSON(lb)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	raw, _ = elbv2svc.DescribeLoadBalancersJSON([]store.ELBv2LoadBalancer{lb})
	_ = json.Unmarshal(raw, &out)

	tg := store.ELBv2TargetGroup{ARN: "arn:tg", Name: "tg", TargetType: "ip", Protocol: "TCP", Port: 443}
	raw, _ = elbv2svc.CreateTargetGroupJSON(tg)
	_ = json.Unmarshal(raw, &out)
	raw, _ = elbv2svc.DescribeTargetGroupsJSON([]store.ELBv2TargetGroup{tg})

	l := store.ELBv2Listener{ListenerARN: "arn:lis", LoadBalancerARN: lb.ARN, Port: 443, Protocol: "TLS", TargetGroupARN: tg.ARN}
	raw, _ = elbv2svc.CreateListenerJSON(l)
	_ = json.Unmarshal(raw, &out)
	raw, _ = elbv2svc.DescribeListenersJSON([]store.ELBv2Listener{l})

	if _, err := elbv2svc.RegisterTargetsJSON(); err != nil {
		t.Fatal(err)
	}

	health := []store.ELBv2TargetHealthDesc{{
		Target: store.ELBv2Target{ID: "10.0.0.1", Port: 443},
		State: "healthy", Reason: "Target.ResponseCodeMismatch", Description: "d",
	}}
	raw, _ = elbv2svc.DescribeTargetHealthJSON(health)
	_ = json.Unmarshal(raw, &out)

	if _, err := elbv2svc.DeleteOKJSON(); err != nil {
		t.Fatal(err)
	}
}
