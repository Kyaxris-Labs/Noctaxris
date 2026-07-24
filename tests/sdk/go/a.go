// Package sdk_test holds live AWS SDK tests against a local Noctaxris endpoint.
//
// This non-test file must sort before apigateway_*_test.go. Otherwise go/build
// treats those files' package sdk_test clause as an external test of package sdk
// and conflicts with client.go.
package sdk_test
