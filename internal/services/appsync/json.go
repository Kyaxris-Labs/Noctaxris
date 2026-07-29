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

// GetSchemaCreationStatusJSON builds GetSchemaCreationStatus response.
func GetSchemaCreationStatusJSON(st store.AppSyncSchemaCreationStatus) ([]byte, error) {
	return json.Marshal(map[string]any{
		"status":  st.Status,
		"details": st.Details,
	})
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

// ListApiKeysJSON builds ListApiKeys response.
// Lab: id is the opaque row UUID; plaintext secrets are not listed after Create.
func ListApiKeysJSON(keys []store.AppSyncAPIKey) ([]byte, error) {
	items := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		items = append(items, map[string]any{
			"id":      k.ID,
			"expires": k.ExpiresAt,
		})
	}
	return json.Marshal(map[string]any{"apiKeys": items})
}

// DeleteApiKeyJSON is an empty OK body.
func DeleteApiKeyJSON() ([]byte, error) { return []byte(`{}`), nil }

// CreateDataSourceJSON builds CreateDataSource response.
func CreateDataSourceJSON(ds store.AppSyncDataSource) ([]byte, error) {
	src := map[string]any{
		"name":          ds.Name,
		"type":          ds.Type,
		"lambdaConfig":  map[string]any{"lambdaFunctionArn": ds.LambdaFunctionARN},
		"dataSourceArn": "arn:aws:appsync:us-east-1:000000000001:apis/" + ds.APIID + "/datasources/" + ds.Name,
	}
	if ds.ServiceRoleArn != "" {
		src["serviceRoleArn"] = ds.ServiceRoleArn
	}
	return json.Marshal(map[string]any{"dataSource": src})
}

// GetDataSourceJSON builds GetDataSource response.
func GetDataSourceJSON(ds store.AppSyncDataSource) ([]byte, error) {
	return CreateDataSourceJSON(ds)
}

// UpdateDataSourceJSON builds UpdateDataSource response.
func UpdateDataSourceJSON(ds store.AppSyncDataSource) ([]byte, error) {
	return CreateDataSourceJSON(ds)
}

// ListDataSourcesJSON builds ListDataSources response.
func ListDataSourcesJSON(sources []store.AppSyncDataSource) ([]byte, error) {
	items := make([]map[string]any, 0, len(sources))
	for _, ds := range sources {
		item := map[string]any{
			"name":          ds.Name,
			"type":          ds.Type,
			"lambdaConfig":  map[string]any{"lambdaFunctionArn": ds.LambdaFunctionARN},
			"dataSourceArn": "arn:aws:appsync:us-east-1:000000000001:apis/" + ds.APIID + "/datasources/" + ds.Name,
		}
		if ds.ServiceRoleArn != "" {
			item["serviceRoleArn"] = ds.ServiceRoleArn
		}
		items = append(items, item)
	}
	return json.Marshal(map[string]any{"dataSources": items})
}

// DeleteDataSourceJSON is an empty OK body.
func DeleteDataSourceJSON() ([]byte, error) { return []byte(`{}`), nil }

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

// GetResolverJSON builds GetResolver response.
func GetResolverJSON(r store.AppSyncResolver) ([]byte, error) {
	return CreateResolverJSON(r)
}

// UpdateResolverJSON builds UpdateResolver response.
func UpdateResolverJSON(r store.AppSyncResolver) ([]byte, error) {
	return CreateResolverJSON(r)
}

// ListResolversJSON builds ListResolvers response.
func ListResolversJSON(resolvers []store.AppSyncResolver) ([]byte, error) {
	items := make([]map[string]any, 0, len(resolvers))
	for _, r := range resolvers {
		items = append(items, map[string]any{
			"typeName":       r.TypeName,
			"fieldName":      r.FieldName,
			"dataSourceName": r.DataSourceName,
		})
	}
	return json.Marshal(map[string]any{"resolvers": items})
}

// DeleteResolverJSON is an empty OK body.
func DeleteResolverJSON() ([]byte, error) { return []byte(`{}`), nil }

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
