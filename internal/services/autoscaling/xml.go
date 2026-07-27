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
		Instances         struct {
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
