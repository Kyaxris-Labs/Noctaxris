package elbv2

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateLoadBalancerJSON builds CreateLoadBalancer response.
func CreateLoadBalancerJSON(lb store.ELBv2LoadBalancer) ([]byte, error) {
	return json.Marshal(map[string]any{
		"LoadBalancers": []map[string]any{lbMap(lb)},
	})
}

func lbMap(lb store.ELBv2LoadBalancer) map[string]any {
	return map[string]any{
		"LoadBalancerArn":  lb.ARN,
		"LoadBalancerName": lb.Name,
		"DNSName":          lb.DNSName,
		"Type":             lb.Type,
		"Scheme":           lb.Scheme,
	}
}

// DescribeLoadBalancersJSON builds DescribeLoadBalancers response.
func DescribeLoadBalancersJSON(lbs []store.ELBv2LoadBalancer) ([]byte, error) {
	items := make([]map[string]any, 0, len(lbs))
	for _, lb := range lbs {
		items = append(items, lbMap(lb))
	}
	return json.Marshal(map[string]any{"LoadBalancers": items})
}

// CreateTargetGroupJSON builds CreateTargetGroup response.
func CreateTargetGroupJSON(tg store.ELBv2TargetGroup) ([]byte, error) {
	return json.Marshal(map[string]any{
		"TargetGroups": []map[string]any{tgMap(tg)},
	})
}

func tgMap(tg store.ELBv2TargetGroup) map[string]any {
	return map[string]any{
		"TargetGroupArn":  tg.ARN,
		"TargetGroupName": tg.Name,
		"TargetType":      tg.TargetType,
		"Protocol":        tg.Protocol,
		"Port":            tg.Port,
	}
}

// DescribeTargetGroupsJSON builds DescribeTargetGroups response.
func DescribeTargetGroupsJSON(tgs []store.ELBv2TargetGroup) ([]byte, error) {
	items := make([]map[string]any, 0, len(tgs))
	for _, tg := range tgs {
		items = append(items, tgMap(tg))
	}
	return json.Marshal(map[string]any{"TargetGroups": items})
}

// CreateListenerJSON builds CreateListener response.
func CreateListenerJSON(l store.ELBv2Listener) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Listeners": []map[string]any{listenerMap(l)},
	})
}

func listenerMap(l store.ELBv2Listener) map[string]any {
	return map[string]any{
		"ListenerArn":     l.ListenerARN,
		"LoadBalancerArn": l.LoadBalancerARN,
		"Port":            l.Port,
		"Protocol":        l.Protocol,
		"DefaultActions": []map[string]any{{
			"Type":           "forward",
			"TargetGroupArn": l.TargetGroupARN,
		}},
	}
}

// DescribeListenersJSON builds DescribeListeners response.
func DescribeListenersJSON(ls []store.ELBv2Listener) ([]byte, error) {
	items := make([]map[string]any, 0, len(ls))
	for _, l := range ls {
		items = append(items, listenerMap(l))
	}
	return json.Marshal(map[string]any{"Listeners": items})
}

// RegisterTargetsJSON is an empty OK body.
func RegisterTargetsJSON() ([]byte, error) { return []byte(`{}`), nil }

// DescribeTargetHealthJSON builds a lite target health list.
// Without a lab listener dataplane, registered targets stay unused (not healthy).
func DescribeTargetHealthJSON(targets []store.ELBv2Target) ([]byte, error) {
	items := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		items = append(items, map[string]any{
			"Target": map[string]any{"Id": t.ID, "Port": t.Port},
			"TargetHealth": map[string]any{
				"State":       "unused",
				"Reason":      "Target.NotInUse",
				"Description": "Control-plane stub: no lab listener dataplane.",
			},
		})
	}
	return json.Marshal(map[string]any{"TargetHealthDescriptions": items})
}

// DeleteOKJSON is an empty OK body.
func DeleteOKJSON() ([]byte, error) { return []byte(`{}`), nil }
