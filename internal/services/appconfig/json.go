package appconfig

import (
	"encoding/base64"
	"encoding/json"
	"strconv"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateApplicationJSON builds a CreateApplication response.
func CreateApplicationJSON(a store.AppConfigApplication) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Id":          a.ID,
		"Name":        a.Name,
		"Description": a.Description,
	})
}

// CreateEnvironmentJSON builds a CreateEnvironment response.
func CreateEnvironmentJSON(e store.AppConfigEnvironment) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ApplicationId": e.ApplicationID,
		"Id":            e.ID,
		"Name":          e.Name,
		"Description":   e.Description,
		"State":         "ReadyForDeployment",
	})
}

// CreateConfigurationProfileJSON builds a CreateConfigurationProfile response.
func CreateConfigurationProfileJSON(p store.AppConfigProfile) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ApplicationId": p.ApplicationID,
		"Id":            p.ID,
		"Name":          p.Name,
		"LocationUri":   p.LocationURI,
		"Description":   p.Description,
		"Type":          "AWS.Freeform",
	})
}

// CreateHostedConfigurationVersionJSON builds a CreateHostedConfigurationVersion response.
func CreateHostedConfigurationVersionJSON(v store.AppConfigHostedVersion) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ApplicationId":          v.ApplicationID,
		"ConfigurationProfileId": v.ProfileID,
		"VersionNumber":          v.VersionNumber,
		"ContentType":            v.ContentType,
	})
}

// GetConfigurationJSON builds a GetConfiguration response.
func GetConfigurationJSON(v store.AppConfigHostedVersion) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Content":              base64.StdEncoding.EncodeToString(v.Content),
		"ConfigurationVersion": strconv.Itoa(v.VersionNumber),
		"ContentType":          v.ContentType,
	})
}

// StartConfigurationSessionJSON builds a StartConfigurationSession response.
func StartConfigurationSessionJSON(token string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"InitialConfigurationToken": token,
	})
}
