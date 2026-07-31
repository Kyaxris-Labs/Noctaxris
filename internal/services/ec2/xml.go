package ec2

import (
	"encoding/xml"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/awsprotocol"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const ec2XMLNS = "http://ec2.amazonaws.com/doc/2016-11-15/"

func isoMilli(ms int64) string {
	return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")
}

type instanceStateXML struct {
	Code int    `xml:"code"`
	Name string `xml:"name"`
}

type instanceXML struct {
	InstanceID       string           `xml:"instanceId"`
	ImageID          string           `xml:"imageId"`
	InstanceType     string           `xml:"instanceType"`
	State            instanceStateXML `xml:"instanceState"`
	PrivateIPAddress string           `xml:"privateIpAddress,omitempty"`
	PublicIPAddress  string           `xml:"ipAddress,omitempty"`
	KeyName          string           `xml:"keyName,omitempty"`
	LaunchTime       string           `xml:"launchTime"`
	Placement        struct {
		AvailabilityZone string `xml:"availabilityZone"`
	} `xml:"placement"`
}

func toInstanceXML(inst store.EC2Instance) instanceXML {
	out := instanceXML{
		InstanceID:       inst.InstanceID,
		ImageID:          inst.ImageID,
		InstanceType:     inst.InstanceType,
		State:            instanceStateXML{Code: inst.StateCode, Name: inst.StateName},
		PrivateIPAddress: inst.PrivateIP,
		PublicIPAddress:  inst.PublicIP,
		KeyName:          inst.KeyName,
		LaunchTime:       isoMilli(inst.CreatedAt),
	}
	out.Placement.AvailabilityZone = inst.AvailabilityZone
	return out
}

// RunInstancesXML builds a RunInstances Query response (flat EC2 shape for AWS SDK).
func RunInstancesXML(instances []store.EC2Instance, requestID string) ([]byte, error) {
	type response struct {
		XMLName       xml.Name `xml:"RunInstancesResponse"`
		XMLNS         string   `xml:"xmlns,attr"`
		RequestID     string   `xml:"requestId"`
		ReservationID string   `xml:"reservationId"`
		OwnerID       string   `xml:"ownerId"`
		InstancesSet  struct {
			Item []instanceXML `xml:"item"`
		} `xml:"instancesSet"`
	}
	r := response{XMLNS: ec2XMLNS, RequestID: requestID, ReservationID: "r-lab", OwnerID: "000000000001"}
	if len(instances) > 0 {
		r.ReservationID = "r-" + instances[0].InstanceID
	}
	for _, inst := range instances {
		r.InstancesSet.Item = append(r.InstancesSet.Item, toInstanceXML(inst))
	}
	return xml.Marshal(r)
}

// DescribeImagesXML builds a DescribeImages response (EC2 Query imagesSet shape).
func DescribeImagesXML(images []store.EC2AMI, requestID string) ([]byte, error) {
	type imageXML struct {
		ImageID            string `xml:"imageId"`
		ImageLocation      string `xml:"imageLocation"`
		ImageState         string `xml:"imageState"`
		ImageOwnerID       string `xml:"imageOwnerId"`
		IsPublic           bool   `xml:"isPublic"`
		Architecture       string `xml:"architecture"`
		ImageType          string `xml:"imageType"`
		Name               string `xml:"name"`
		Description        string `xml:"description,omitempty"`
		RootDeviceType     string `xml:"rootDeviceType"`
		RootDeviceName     string `xml:"rootDeviceName"`
		VirtualizationType string `xml:"virtualizationType"`
		Hypervisor         string `xml:"hypervisor"`
		ImageOwnerAlias    string `xml:"imageOwnerAlias,omitempty"`
		CreationDate       string `xml:"creationDate,omitempty"`
	}
	type response struct {
		XMLName   xml.Name `xml:"DescribeImagesResponse"`
		XMLNS     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"requestId"`
		ImagesSet struct {
			Item []imageXML `xml:"item"`
		} `xml:"imagesSet"`
	}
	r := response{XMLNS: ec2XMLNS, RequestID: requestID}
	for _, img := range images {
		r.ImagesSet.Item = append(r.ImagesSet.Item, imageXML{
			ImageID:            img.ImageID,
			ImageLocation:      img.OwnerID + "/" + img.Name,
			ImageState:         img.State,
			ImageOwnerID:       img.OwnerID,
			IsPublic:           img.Public,
			Architecture:       img.Architecture,
			ImageType:          img.ImageType,
			Name:               img.Name,
			Description:        img.Description,
			RootDeviceType:     img.RootDeviceType,
			RootDeviceName:     img.RootDeviceName,
			VirtualizationType: img.VirtualizationType,
			Hypervisor:         img.Hypervisor,
			ImageOwnerAlias:    img.ImageOwnerAlias,
			CreationDate:       img.CreationDate,
		})
	}
	return xml.Marshal(r)
}

// DescribeInstancesXML builds a DescribeInstances Query response.
func DescribeInstancesXML(instances []store.EC2Instance, requestID string) ([]byte, error) {
	type reservation struct {
		ReservationID string `xml:"reservationId"`
		OwnerID       string `xml:"ownerId"`
		InstancesSet  struct {
			Item []instanceXML `xml:"item"`
		} `xml:"instancesSet"`
	}
	type response struct {
		XMLName        xml.Name `xml:"DescribeInstancesResponse"`
		XMLNS          string   `xml:"xmlns,attr"`
		RequestID      string   `xml:"requestId"`
		ReservationSet struct {
			Item []reservation `xml:"item"`
		} `xml:"reservationSet"`
	}
	r := response{XMLNS: ec2XMLNS, RequestID: requestID}
	if len(instances) == 0 {
		return xml.Marshal(r)
	}
	res := reservation{ReservationID: "r-lab", OwnerID: "000000000001"}
	for _, inst := range instances {
		res.InstancesSet.Item = append(res.InstancesSet.Item, toInstanceXML(inst))
	}
	r.ReservationSet.Item = append(r.ReservationSet.Item, res)
	return xml.Marshal(r)
}

type stateChangeXML struct {
	InstanceID    string           `xml:"instanceId"`
	CurrentState  instanceStateXML `xml:"currentState"`
	PreviousState instanceStateXML `xml:"previousState"`
}

func stateChangeSetXML(action string, changes []stateChangeXML, requestID string) ([]byte, error) {
	type response struct {
		XMLName      xml.Name
		XMLNS        string `xml:"xmlns,attr"`
		RequestID    string `xml:"requestId"`
		InstancesSet struct {
			Item []stateChangeXML `xml:"item"`
		} `xml:"instancesSet"`
	}
	r := response{
		XMLName:   xml.Name{Local: action + "Response"},
		XMLNS:     ec2XMLNS,
		RequestID: requestID,
	}
	r.InstancesSet.Item = changes
	return xml.Marshal(r)
}

// TerminateInstancesXML builds TerminateInstances response.
func TerminateInstancesXML(changes []StateChange, requestID string) ([]byte, error) {
	return stateChangeSetXML("TerminateInstances", toStateChanges(changes), requestID)
}

// StopInstancesXML builds StopInstances response.
func StopInstancesXML(changes []StateChange, requestID string) ([]byte, error) {
	return stateChangeSetXML("StopInstances", toStateChanges(changes), requestID)
}

// StartInstancesXML builds StartInstances response.
func StartInstancesXML(changes []StateChange, requestID string) ([]byte, error) {
	return stateChangeSetXML("StartInstances", toStateChanges(changes), requestID)
}

// StateChange is a previous→current instance state transition.
type StateChange struct {
	InstanceID   string
	PreviousName string
	PreviousCode int
	CurrentName  string
	CurrentCode  int
}

func toStateChanges(changes []StateChange) []stateChangeXML {
	out := make([]stateChangeXML, 0, len(changes))
	for _, c := range changes {
		out = append(out, stateChangeXML{
			InstanceID:    c.InstanceID,
			PreviousState: instanceStateXML{Code: c.PreviousCode, Name: c.PreviousName},
			CurrentState:  instanceStateXML{Code: c.CurrentCode, Name: c.CurrentName},
		})
	}
	return out
}

// ErrorXML builds an EC2 Query ErrorResponse.
func ErrorXML(code, message, requestID string) []byte {
	body, err := awsprotocol.MarshalQueryError(awsprotocol.QueryErrorEC2, "", code, message, requestID)
	if err != nil {
		return nil
	}
	return body
}
