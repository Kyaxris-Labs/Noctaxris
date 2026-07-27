package elasticbeanstalk

import (
	"encoding/xml"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const beanstalkXMLNS = "http://elasticbeanstalk.amazonaws.com/docs/2010-12-01/"

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
	return xml.Marshal(envelope{
		XMLName:          xml.Name{Local: root},
		XMLNS:            beanstalkXMLNS,
		Result:           result,
		ResponseMetadata: responseMetadata{RequestID: requestID},
	})
}

func isoMilli(ms int64) string {
	return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")
}

// EmptyOKXML builds an empty DeleteApplication response.
func EmptyOKXML(action, requestID string) ([]byte, error) {
	type result struct {
		XMLName xml.Name
	}
	return marshal(action+"Response", result{XMLName: xml.Name{Local: action + "Result"}}, requestID)
}

// CreateApplicationXML builds CreateApplication response.
func CreateApplicationXML(app store.BeanstalkApplication, requestID string) ([]byte, error) {
	type application struct {
		ApplicationName string `xml:"ApplicationName"`
		ApplicationARN  string `xml:"ApplicationArn"`
		Description     string `xml:"Description,omitempty"`
		DateCreated     string `xml:"DateCreated"`
		DateUpdated     string `xml:"DateUpdated"`
		ConfigurationTemplates struct {
			Member []string `xml:"member"`
		} `xml:"ConfigurationTemplates"`
	}
	type result struct {
		XMLName     xml.Name    `xml:"CreateApplicationResult"`
		Application application `xml:"Application"`
	}
	r := result{Application: application{
		ApplicationName: app.ApplicationName, ApplicationARN: app.ARN, Description: app.Description,
		DateCreated: isoMilli(app.CreatedAt), DateUpdated: isoMilli(app.UpdatedAt),
	}}
	r.Application.ConfigurationTemplates.Member = []string{"Default"}
	return marshal("CreateApplicationResponse", r, requestID)
}

// DescribeApplicationsXML builds DescribeApplications response.
func DescribeApplicationsXML(apps []store.BeanstalkApplication, requestID string) ([]byte, error) {
	type application struct {
		ApplicationName string `xml:"ApplicationName"`
		ApplicationARN  string `xml:"ApplicationArn"`
		Description     string `xml:"Description,omitempty"`
		DateCreated     string `xml:"DateCreated"`
		DateUpdated     string `xml:"DateUpdated"`
		ConfigurationTemplates struct {
			Member []string `xml:"member"`
		} `xml:"ConfigurationTemplates"`
	}
	type result struct {
		XMLName      xml.Name `xml:"DescribeApplicationsResult"`
		Applications struct {
			Member []application `xml:"member"`
		} `xml:"Applications"`
	}
	var r result
	for _, app := range apps {
		a := application{
			ApplicationName: app.ApplicationName, ApplicationARN: app.ARN, Description: app.Description,
			DateCreated: isoMilli(app.CreatedAt), DateUpdated: isoMilli(app.UpdatedAt),
		}
		a.ConfigurationTemplates.Member = []string{"Default"}
		r.Applications.Member = append(r.Applications.Member, a)
	}
	return marshal("DescribeApplicationsResponse", r, requestID)
}

// CreateApplicationVersionXML builds CreateApplicationVersion response.
func CreateApplicationVersionXML(v store.BeanstalkApplicationVersion, requestID string) ([]byte, error) {
	type sourceBundle struct {
		S3Bucket string `xml:"S3Bucket,omitempty"`
		S3Key    string `xml:"S3Key,omitempty"`
	}
	type version struct {
		ApplicationName string       `xml:"ApplicationName"`
		VersionLabel    string       `xml:"VersionLabel"`
		Description     string       `xml:"Description,omitempty"`
		DateCreated     string       `xml:"DateCreated"`
		DateUpdated     string       `xml:"DateUpdated"`
		Status          string       `xml:"Status"`
		SourceBundle    sourceBundle `xml:"SourceBundle"`
	}
	type result struct {
		XMLName            xml.Name `xml:"CreateApplicationVersionResult"`
		ApplicationVersion version  `xml:"ApplicationVersion"`
	}
	return marshal("CreateApplicationVersionResponse", result{ApplicationVersion: version{
		ApplicationName: v.ApplicationName, VersionLabel: v.VersionLabel, Description: v.Description,
		DateCreated: isoMilli(v.CreatedAt), DateUpdated: isoMilli(v.CreatedAt), Status: v.Status,
		SourceBundle: sourceBundle{S3Bucket: v.S3Bucket, S3Key: v.S3Key},
	}}, requestID)
}

// CreateEnvironmentXML builds CreateEnvironment response.
func CreateEnvironmentXML(env store.BeanstalkEnvironment, requestID string) ([]byte, error) {
	type typed struct {
		XMLName           xml.Name `xml:"CreateEnvironmentResult"`
		EnvironmentName   string   `xml:"EnvironmentName"`
		EnvironmentID     string   `xml:"EnvironmentId"`
		EnvironmentARN    string   `xml:"EnvironmentArn"`
		ApplicationName   string   `xml:"ApplicationName"`
		VersionLabel      string   `xml:"VersionLabel,omitempty"`
		SolutionStackName string   `xml:"SolutionStackName"`
		Description       string   `xml:"Description,omitempty"`
		CNAME             string   `xml:"CNAME"`
		Status            string   `xml:"Status"`
		Health            string   `xml:"Health"`
		HealthStatus      string   `xml:"HealthStatus"`
		DateCreated       string   `xml:"DateCreated"`
		DateUpdated       string   `xml:"DateUpdated"`
		Tier              struct {
			Name    string `xml:"Name"`
			Type    string `xml:"Type"`
			Version string `xml:"Version"`
		} `xml:"Tier"`
	}
	r := typed{
		EnvironmentName: env.EnvironmentName, EnvironmentID: env.EnvironmentID, EnvironmentARN: env.ARN,
		ApplicationName: env.ApplicationName, VersionLabel: env.VersionLabel, SolutionStackName: env.SolutionStackName,
		Description: env.Description, CNAME: env.CNAME, Status: env.Status, Health: env.Health,
		HealthStatus: env.HealthStatus, DateCreated: isoMilli(env.CreatedAt), DateUpdated: isoMilli(env.UpdatedAt),
	}
	r.Tier.Name, r.Tier.Type, r.Tier.Version = "WebServer", "Standard", "1.0"
	return marshal("CreateEnvironmentResponse", r, requestID)
}
func DescribeEnvironmentsXML(envs []store.BeanstalkEnvironment, requestID string) ([]byte, error) {
	type envMember struct {
		EnvironmentName   string `xml:"EnvironmentName"`
		EnvironmentID     string `xml:"EnvironmentId"`
		EnvironmentARN    string `xml:"EnvironmentArn"`
		ApplicationName   string `xml:"ApplicationName"`
		VersionLabel      string `xml:"VersionLabel,omitempty"`
		SolutionStackName string `xml:"SolutionStackName"`
		Description       string `xml:"Description,omitempty"`
		CNAME             string `xml:"CNAME"`
		Status            string `xml:"Status"`
		Health            string `xml:"Health"`
		HealthStatus      string `xml:"HealthStatus"`
		DateCreated       string `xml:"DateCreated"`
		DateUpdated       string `xml:"DateUpdated"`
	}
	type result struct {
		XMLName      xml.Name `xml:"DescribeEnvironmentsResult"`
		Environments struct {
			Member []envMember `xml:"member"`
		} `xml:"Environments"`
	}
	var r result
	for _, env := range envs {
		r.Environments.Member = append(r.Environments.Member, envMember{
			EnvironmentName: env.EnvironmentName, EnvironmentID: env.EnvironmentID, EnvironmentARN: env.ARN,
			ApplicationName: env.ApplicationName, VersionLabel: env.VersionLabel, SolutionStackName: env.SolutionStackName,
			Description: env.Description, CNAME: env.CNAME, Status: env.Status, Health: env.Health,
			HealthStatus: env.HealthStatus, DateCreated: isoMilli(env.CreatedAt), DateUpdated: isoMilli(env.UpdatedAt),
		})
	}
	return marshal("DescribeEnvironmentsResponse", r, requestID)
}

// TerminateEnvironmentResponseXML builds TerminateEnvironment response with correct root.
func TerminateEnvironmentResponseXML(env store.BeanstalkEnvironment, requestID string) ([]byte, error) {
	type typed struct {
		XMLName           xml.Name `xml:"TerminateEnvironmentResult"`
		EnvironmentName   string   `xml:"EnvironmentName"`
		EnvironmentID     string   `xml:"EnvironmentId"`
		EnvironmentARN    string   `xml:"EnvironmentArn"`
		ApplicationName   string   `xml:"ApplicationName"`
		Status            string   `xml:"Status"`
		Health            string   `xml:"Health"`
		HealthStatus      string   `xml:"HealthStatus"`
		DateCreated       string   `xml:"DateCreated"`
		DateUpdated       string   `xml:"DateUpdated"`
	}
	return marshal("TerminateEnvironmentResponse", typed{
		EnvironmentName: env.EnvironmentName, EnvironmentID: env.EnvironmentID, EnvironmentARN: env.ARN,
		ApplicationName: env.ApplicationName, Status: env.Status, Health: env.Health, HealthStatus: env.HealthStatus,
		DateCreated: isoMilli(env.CreatedAt), DateUpdated: isoMilli(env.UpdatedAt),
	}, requestID)
}

// ListAvailableSolutionStacksXML builds ListAvailableSolutionStacks response.
func ListAvailableSolutionStacksXML(stacks []string, requestID string) ([]byte, error) {
	type result struct {
		XMLName        xml.Name `xml:"ListAvailableSolutionStacksResult"`
		SolutionStacks struct {
			Member []string `xml:"member"`
		} `xml:"SolutionStacks"`
		SolutionStackDetails struct {
			Member []struct{} `xml:"member"`
		} `xml:"SolutionStackDetails"`
	}
	var r result
	r.SolutionStacks.Member = stacks
	return marshal("ListAvailableSolutionStacksResponse", r, requestID)
}
