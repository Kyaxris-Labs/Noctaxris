package autoscaling

import (
	"encoding/json"
	"encoding/xml"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const asgXMLNS = "http://autoscaling.amazonaws.com/doc/2011-01-01/"

type responseMetadata struct {
	RequestID string `xml:"RequestId"`
}

func marshal(root string, result any, requestID string) ([]byte, error) {
	type envelope struct {
		XMLName          xml.Name
		XMLNS            string `xml:"xmlns,attr"`
		Result           any
		ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
	}
	env := envelope{
		XMLName:          xml.Name{Local: root},
		XMLNS:            asgXMLNS,
		Result:           result,
		ResponseMetadata: responseMetadata{RequestID: requestID},
	}
	return xml.Marshal(env)
}

func isoMilli(ms int64) string {
	return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")
}

func parseStringSlice(jsonArr string) []string {
	var out []string
	_ = json.Unmarshal([]byte(jsonArr), &out)
	return out
}

// EmptyOKXML builds an empty success response for Create/Delete/Update/SetDesiredCapacity.
func EmptyOKXML(action, requestID string) ([]byte, error) {
	type result struct {
		XMLName xml.Name
	}
	return marshal(action+"Response", result{XMLName: xml.Name{Local: action + "Result"}}, requestID)
}

// DescribeLaunchConfigurationsXML builds DescribeLaunchConfigurations response.
func DescribeLaunchConfigurationsXML(list []store.ASGLaunchConfiguration, requestID string) ([]byte, error) {
	type lc struct {
		LaunchConfigurationName string   `xml:"LaunchConfigurationName"`
		LaunchConfigurationARN  string   `xml:"LaunchConfigurationARN"`
		ImageID                 string   `xml:"ImageId"`
		InstanceType            string   `xml:"InstanceType"`
		KeyName                 string   `xml:"KeyName,omitempty"`
		SecurityGroups          struct {
			Member []string `xml:"member"`
		} `xml:"SecurityGroups"`
		UserData           string `xml:"UserData,omitempty"`
		IamInstanceProfile string `xml:"IamInstanceProfile,omitempty"`
		CreatedTime        string `xml:"CreatedTime"`
	}
	type result struct {
		XMLName              xml.Name `xml:"DescribeLaunchConfigurationsResult"`
		LaunchConfigurations struct {
			Member []lc `xml:"member"`
		} `xml:"LaunchConfigurations"`
	}
	var r result
	for _, item := range list {
		entry := lc{
			LaunchConfigurationName: item.LaunchConfigurationName,
			LaunchConfigurationARN:  item.ARN,
			ImageID:                 item.ImageID,
			InstanceType:            item.InstanceType,
			KeyName:                 item.KeyName,
			UserData:                item.UserData,
			IamInstanceProfile:      item.IamInstanceProfile,
			CreatedTime:             isoMilli(item.CreatedAt),
		}
		entry.SecurityGroups.Member = parseStringSlice(item.SecurityGroups)
		r.LaunchConfigurations.Member = append(r.LaunchConfigurations.Member, entry)
	}
	return marshal("DescribeLaunchConfigurationsResponse", r, requestID)
}

// DescribeAutoScalingGroupsXML builds DescribeAutoScalingGroups response with Instances.
func DescribeAutoScalingGroupsXML(list []store.AutoScalingGroup, requestID string) ([]byte, error) {
	type asgInst struct {
		InstanceID           string `xml:"InstanceId"`
		LifecycleState       string `xml:"LifecycleState"`
		HealthStatus         string `xml:"HealthStatus"`
		AvailabilityZone     string `xml:"AvailabilityZone,omitempty"`
		ProtectedFromScaleIn bool   `xml:"ProtectedFromScaleIn"`
	}
	type group struct {
		AutoScalingGroupName    string `xml:"AutoScalingGroupName"`
		AutoScalingGroupARN     string `xml:"AutoScalingGroupARN"`
		LaunchConfigurationName string `xml:"LaunchConfigurationName,omitempty"`
		MinSize                 int    `xml:"MinSize"`
		MaxSize                 int    `xml:"MaxSize"`
		DesiredCapacity         int    `xml:"DesiredCapacity"`
		DefaultCooldown         int    `xml:"DefaultCooldown"`
		AvailabilityZones       struct {
			Member []string `xml:"member"`
		} `xml:"AvailabilityZones"`
		VPCZoneIdentifier string `xml:"VPCZoneIdentifier,omitempty"`
		HealthCheckType   string `xml:"HealthCheckType"`
		CreatedTime       string `xml:"CreatedTime"`
		TargetGroupARNs   struct {
			Member []string `xml:"member"`
		} `xml:"TargetGroupARNs"`
		Instances struct {
			Member []asgInst `xml:"member"`
		} `xml:"Instances"`
	}
	type result struct {
		XMLName           xml.Name `xml:"DescribeAutoScalingGroupsResult"`
		AutoScalingGroups struct {
			Member []group `xml:"member"`
		} `xml:"AutoScalingGroups"`
	}
	var r result
	for _, item := range list {
		g := group{
			AutoScalingGroupName:    item.AutoScalingGroupName,
			AutoScalingGroupARN:     item.ARN,
			LaunchConfigurationName: item.LaunchConfigurationName,
			MinSize:                 item.MinSize,
			MaxSize:                 item.MaxSize,
			DesiredCapacity:         item.DesiredCapacity,
			DefaultCooldown:         item.DefaultCooldown,
			VPCZoneIdentifier:       item.VPCZoneIdentifier,
			HealthCheckType:         item.HealthCheckType,
			CreatedTime:             isoMilli(item.CreatedAt),
		}
		g.AvailabilityZones.Member = parseStringSlice(item.AvailabilityZones)
		g.TargetGroupARNs.Member = append([]string{}, item.TargetGroupARNs...)
		for _, inst := range item.Instances {
			g.Instances.Member = append(g.Instances.Member, asgInst{
				InstanceID:           inst.InstanceID,
				LifecycleState:       inst.LifecycleState,
				HealthStatus:         inst.HealthStatus,
				AvailabilityZone:     inst.AvailabilityZone,
				ProtectedFromScaleIn: inst.ProtectedFromScaleIn,
			})
		}
		r.AutoScalingGroups.Member = append(r.AutoScalingGroups.Member, g)
	}
	return marshal("DescribeAutoScalingGroupsResponse", r, requestID)
}

// PutScalingPolicyXML builds PutScalingPolicy response with PolicyARN.
func PutScalingPolicyXML(policyARN, requestID string) ([]byte, error) {
	type result struct {
		XMLName   xml.Name `xml:"PutScalingPolicyResult"`
		PolicyARN string   `xml:"PolicyARN"`
	}
	return marshal("PutScalingPolicyResponse", result{PolicyARN: policyARN}, requestID)
}

// DescribePoliciesXML builds DescribePolicies response.
func DescribePoliciesXML(list []store.ASGScalingPolicy, requestID string) ([]byte, error) {
	type predefined struct {
		PredefinedMetricType string `xml:"PredefinedMetricType"`
		ResourceLabel        string `xml:"ResourceLabel,omitempty"`
	}
	type targetTracking struct {
		TargetValue                   float64     `xml:"TargetValue"`
		PredefinedMetricSpecification *predefined `xml:"PredefinedMetricSpecification,omitempty"`
	}
	type policy struct {
		PolicyName              string          `xml:"PolicyName"`
		PolicyARN               string          `xml:"PolicyARN"`
		AutoScalingGroupName    string          `xml:"AutoScalingGroupName"`
		PolicyType              string          `xml:"PolicyType"`
		AdjustmentType          string          `xml:"AdjustmentType,omitempty"`
		ScalingAdjustment       int             `xml:"ScalingAdjustment"`
		Cooldown                int             `xml:"Cooldown"`
		EstimatedInstanceWarmup *int            `xml:"EstimatedInstanceWarmup,omitempty"`
		TargetTracking          *targetTracking `xml:"TargetTrackingConfiguration,omitempty"`
	}
	type result struct {
		XMLName         xml.Name `xml:"DescribePoliciesResult"`
		ScalingPolicies struct {
			Member []policy `xml:"member"`
		} `xml:"ScalingPolicies"`
	}
	var r result
	for _, item := range list {
		p := policy{
			PolicyName: item.PolicyName, PolicyARN: item.PolicyARN,
			AutoScalingGroupName: item.AutoScalingGroupName, PolicyType: item.PolicyType,
			AdjustmentType: item.AdjustmentType, ScalingAdjustment: item.ScalingAdjustment,
			Cooldown: item.Cooldown, EstimatedInstanceWarmup: item.EstimatedInstanceWarmup,
		}
		if item.TargetTracking != nil {
			tt := &targetTracking{TargetValue: item.TargetTracking.TargetValue}
			if item.TargetTracking.PredefinedMetricType != "" {
				tt.PredefinedMetricSpecification = &predefined{
					PredefinedMetricType: item.TargetTracking.PredefinedMetricType,
					ResourceLabel:        item.TargetTracking.ResourceLabel,
				}
			}
			p.TargetTracking = tt
		}
		r.ScalingPolicies.Member = append(r.ScalingPolicies.Member, p)
	}
	return marshal("DescribePoliciesResponse", r, requestID)
}

// DescribeLifecycleHooksXML builds DescribeLifecycleHooks response.
func DescribeLifecycleHooksXML(list []store.ASGLifecycleHook, requestID string) ([]byte, error) {
	type hook struct {
		LifecycleHookName     string `xml:"LifecycleHookName"`
		AutoScalingGroupName  string `xml:"AutoScalingGroupName"`
		LifecycleTransition   string `xml:"LifecycleTransition"`
		NotificationTargetARN string `xml:"NotificationTargetARN,omitempty"`
		RoleARN               string `xml:"RoleARN,omitempty"`
		NotificationMetadata  string `xml:"NotificationMetadata,omitempty"`
		HeartbeatTimeout      int    `xml:"HeartbeatTimeout"`
		GlobalTimeout         int    `xml:"GlobalTimeout"`
		DefaultResult         string `xml:"DefaultResult"`
	}
	type result struct {
		XMLName        xml.Name `xml:"DescribeLifecycleHooksResult"`
		LifecycleHooks struct {
			Member []hook `xml:"member"`
		} `xml:"LifecycleHooks"`
	}
	var r result
	for _, item := range list {
		r.LifecycleHooks.Member = append(r.LifecycleHooks.Member, hook{
			LifecycleHookName: item.LifecycleHookName, AutoScalingGroupName: item.AutoScalingGroupName,
			LifecycleTransition: item.LifecycleTransition, NotificationTargetARN: item.NotificationTargetARN,
			RoleARN: item.RoleARN, NotificationMetadata: item.NotificationMetadata,
			HeartbeatTimeout: item.HeartbeatTimeout, GlobalTimeout: item.GlobalTimeout,
			DefaultResult: item.DefaultResult,
		})
	}
	return marshal("DescribeLifecycleHooksResponse", r, requestID)
}

// DescribeAutoScalingInstancesXML builds DescribeAutoScalingInstances response.
func DescribeAutoScalingInstancesXML(list []store.ASGDescribedInstance, requestID string) ([]byte, error) {
	type member struct {
		InstanceID              string `xml:"InstanceId"`
		AutoScalingGroupName    string `xml:"AutoScalingGroupName"`
		AvailabilityZone        string `xml:"AvailabilityZone,omitempty"`
		LifecycleState          string `xml:"LifecycleState"`
		HealthStatus            string `xml:"HealthStatus"`
		ProtectedFromScaleIn    bool   `xml:"ProtectedFromScaleIn"`
		LaunchConfigurationName string `xml:"LaunchConfigurationName,omitempty"`
	}
	type result struct {
		XMLName              xml.Name `xml:"DescribeAutoScalingInstancesResult"`
		AutoScalingInstances struct {
			Member []member `xml:"member"`
		} `xml:"AutoScalingInstances"`
	}
	var r result
	for _, item := range list {
		r.AutoScalingInstances.Member = append(r.AutoScalingInstances.Member, member{
			InstanceID: item.InstanceID, AutoScalingGroupName: item.AutoScalingGroupName,
			AvailabilityZone: item.AvailabilityZone, LifecycleState: item.LifecycleState,
			HealthStatus: item.HealthStatus, ProtectedFromScaleIn: item.ProtectedFromScaleIn,
			LaunchConfigurationName: item.LaunchConfigurationName,
		})
	}
	return marshal("DescribeAutoScalingInstancesResponse", r, requestID)
}

// DetachInstancesXML builds DetachInstances response with empty Activities.
func DetachInstancesXML(requestID string) ([]byte, error) {
	type result struct {
		XMLName    xml.Name `xml:"DetachInstancesResult"`
		Activities struct {
			Member []struct{} `xml:"member"`
		} `xml:"Activities"`
	}
	return marshal("DetachInstancesResponse", result{}, requestID)
}

// DescribeLoadBalancerTargetGroupsXML builds DescribeLoadBalancerTargetGroups response.
func DescribeLoadBalancerTargetGroupsXML(arns []string, requestID string) ([]byte, error) {
	type member struct {
		LoadBalancerTargetGroupARN string `xml:"LoadBalancerTargetGroupARN"`
		State                      string `xml:"State"`
	}
	type result struct {
		XMLName                 xml.Name `xml:"DescribeLoadBalancerTargetGroupsResult"`
		LoadBalancerTargetGroups struct {
			Member []member `xml:"member"`
		} `xml:"LoadBalancerTargetGroups"`
	}
	var r result
	for _, arn := range arns {
		r.LoadBalancerTargetGroups.Member = append(r.LoadBalancerTargetGroups.Member, member{
			LoadBalancerTargetGroupARN: arn,
			State:                      "InService",
		})
	}
	return marshal("DescribeLoadBalancerTargetGroupsResponse", r, requestID)
}
