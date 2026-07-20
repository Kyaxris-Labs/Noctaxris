package appsync

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateGraphqlApiJSON builds CreateGraphqlApi response.
func CreateGraphqlApiJSON(a store.AppSyncAPI) ([]byte, error) {
	api := map[string]any{
		"name":               a.Name,
		"apiId":              a.APIID,
		"arn":                a.ARN,
		"authenticationType": a.AuthenticationType,
		"uris": map[string]string{
			"GRAPHQL": "http://127.0.0.1:4566/appsync/" + a.APIID + "/graphql",
		},
	}
	if a.AuthenticationType == store.AppSyncAuthCognito {
		api["userPoolConfig"] = map[string]any{
			"userPoolId": a.UserPoolID,
			"awsRegion":  a.UserPoolRegion,
			"clientId":   a.UserPoolClientID,
			"issuer":     a.UserPoolIssuer,
		}
	}
	return json.Marshal(map[string]any{"graphqlApi": api})
}

// GetGraphqlApiJSON builds GetGraphqlApi response.
func GetGraphqlApiJSON(a store.AppSyncAPI) ([]byte, error) {
	return CreateGraphqlApiJSON(a)
}

// ListGraphqlApisJSON builds ListGraphqlApis response.
func ListGraphqlApisJSON(apis []store.AppSyncAPI) ([]byte, error) {
	items := make([]map[string]any, 0, len(apis))
	for _, a := range apis {
		items = append(items, map[string]any{
			"name":               a.Name,
			"apiId":              a.APIID,
			"arn":                a.ARN,
			"authenticationType": a.AuthenticationType,
		})
	}
	return json.Marshal(map[string]any{"graphqlApis": items})
}

// DeleteGraphqlApiJSON is an empty OK body.
func DeleteGraphqlApiJSON() ([]byte, error) { return []byte(`{}`), nil }

// StartSchemaCreationJSON builds StartSchemaCreation response.
func StartSchemaCreationJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"status": "SUCCESS"})
}

// CreateApiKeyJSON builds CreateApiKey response.
// Lab: apiKey.id is the secret key string (AWS AppSync shape).
func CreateApiKeyJSON(k store.AppSyncAPIKey) ([]byte, error) {
	return json.Marshal(map[string]any{
		"apiKey": map[string]any{
			"id":      k.APIKey,
			"expires": k.ExpiresAt,
		},
	})
}

// CreateDataSourceJSON builds CreateDataSource response.
func CreateDataSourceJSON(ds store.AppSyncDataSource) ([]byte, error) {
	return json.Marshal(map[string]any{
		"dataSource": map[string]any{
			"name":              ds.Name,
			"type":              ds.Type,
			"lambdaConfig":      map[string]any{"lambdaFunctionArn": ds.LambdaFunctionARN},
			"dataSourceArn":     "arn:aws:appsync:us-east-1:000000000001:apis/" + ds.APIID + "/datasources/" + ds.Name,
		},
	})
}

// CreateResolverJSON builds CreateResolver response.
func CreateResolverJSON(r store.AppSyncResolver) ([]byte, error) {
	return json.Marshal(map[string]any{
		"resolver": map[string]any{
			"typeName":       r.TypeName,
			"fieldName":      r.FieldName,
			"dataSourceName": r.DataSourceName,
		},
	})
}

// GraphQLDataJSON builds a GraphQL success envelope for one field.
func GraphQLDataJSON(field string, value any) ([]byte, error) {
	return json.Marshal(map[string]any{
		"data": map[string]any{field: value},
	})
}

// GraphQLErrorsJSON builds a GraphQL error envelope.
func GraphQLErrorsJSON(message string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"errors": []map[string]any{{"message": message}},
	})
}
