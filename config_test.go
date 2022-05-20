package main

import (
	"io/ioutil"
	"testing"

	ourCloudHTTP "github.com/brigadecore/brigade-cloudevents-gateway/internal/cloudevents/http" // nolint: lll
	"github.com/brigadecore/brigade-foundations/http"
	"github.com/brigadecore/brigade/sdk/v3/restmachinery"
	"github.com/stretchr/testify/require"
)

// Note that unit testing in Go does NOT clear environment variables between
// tests, which can sometimes be a pain, but it's fine here-- so each of these
// test functions uses a series of test cases that cumulatively build upon one
// another.

func TestAPIClientConfig(t *testing.T) {
	testCases := []struct {
		name       string
		setup      func()
		assertions func(
			address string,
			token string,
			opts restmachinery.APIClientOptions,
			err error,
		)
	}{
		{
			name:  "API_ADDRESS not set",
			setup: func() {},
			assertions: func(
				_ string,
				_ string,
				_ restmachinery.APIClientOptions,
				err error,
			) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "value not found for")
				require.Contains(t, err.Error(), "API_ADDRESS")
			},
		},
		{
			name: "API_TOKEN not set",
			setup: func() {
				t.Setenv("API_ADDRESS", "foo")
			},
			assertions: func(
				_ string,
				_ string,
				_ restmachinery.APIClientOptions,
				err error,
			) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "value not found for")
				require.Contains(t, err.Error(), "API_TOKEN")
			},
		},
		{
			name: "success",
			setup: func() {
				t.Setenv("API_TOKEN", "bar")
				t.Setenv("API_IGNORE_CERT_WARNINGS", "true")
			},
			assertions: func(
				address string,
				token string,
				opts restmachinery.APIClientOptions,
				err error,
			) {
				require.NoError(t, err)
				require.Equal(t, "foo", address)
				require.Equal(t, "bar", token)
				require.True(t, opts.AllowInsecureConnections)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.setup()
			address, token, opts, err := apiClientConfig()
			testCase.assertions(address, token, opts, err)
		})
	}
}

func TestTokenFilterConfig(t *testing.T) {
	testCases := []struct {
		name       string
		setup      func()
		assertions func(ourCloudHTTP.TokenFilterConfig, error)
	}{
		{
			name: "TOKENS_PATH not set",
			assertions: func(_ ourCloudHTTP.TokenFilterConfig, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "value not found for")
				require.Contains(t, err.Error(), "TOKENS_PATH")
			},
		},
		{
			name: "TOKENS_PATH path does not exist",
			setup: func() {
				t.Setenv("TOKENS_PATH", "/completely/bogus/path")
			},
			assertions: func(_ ourCloudHTTP.TokenFilterConfig, err error) {
				require.Error(t, err)
				require.Contains(
					t,
					err.Error(),
					"file /completely/bogus/path does not exist",
				)
			},
		},
		{
			name: "TOKENS_PATH does not contain valid json",
			setup: func() {
				tokensFile, err := ioutil.TempFile("", "tokens.json")
				require.NoError(t, err)
				defer tokensFile.Close()
				_, err = tokensFile.Write([]byte("this is not json"))
				require.NoError(t, err)
				t.Setenv("TOKENS_PATH", tokensFile.Name())
			},
			assertions: func(_ ourCloudHTTP.TokenFilterConfig, err error) {
				require.Error(t, err)
				require.Contains(
					t, err.Error(), "invalid character",
				)
			},
		},
		{
			name: "success",
			setup: func() {
				tokensFile, err := ioutil.TempFile("", "tokens.json")
				require.NoError(t, err)
				defer tokensFile.Close()
				_, err = tokensFile.Write([]byte(`{"foo": "bar"}`))
				require.NoError(t, err)
				t.Setenv("TOKENS_PATH", tokensFile.Name())
			},
			assertions: func(config ourCloudHTTP.TokenFilterConfig, err error) {
				require.NoError(t, err)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.setup != nil {
				testCase.setup()
			}
			config, err := tokenFilterConfig()
			testCase.assertions(config, err)
		})
	}
}

func TestServerConfig(t *testing.T) {
	testCases := []struct {
		name       string
		setup      func()
		assertions func(http.ServerConfig, error)
	}{
		{
			name: "PORT not an int",
			setup: func() {
				t.Setenv("PORT", "foo")
			},
			assertions: func(_ http.ServerConfig, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "was not parsable as an int")
				require.Contains(t, err.Error(), "PORT")
			},
		},
		{
			name: "TLS_ENABLED not a bool",
			setup: func() {
				t.Setenv("PORT", "8080")
				t.Setenv("TLS_ENABLED", "nope")
			},
			assertions: func(_ http.ServerConfig, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "was not parsable as a bool")
				require.Contains(t, err.Error(), "TLS_ENABLED")
			},
		},
		{
			name: "TLS_CERT_PATH required but not set",
			setup: func() {
				t.Setenv("TLS_ENABLED", "true")
			},
			assertions: func(_ http.ServerConfig, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "value not found for")
				require.Contains(t, err.Error(), "TLS_CERT_PATH")
			},
		},
		{
			name: "TLS_KEY_PATH required but not set",
			setup: func() {
				t.Setenv("TLS_CERT_PATH", "/var/ssl/cert")
			},
			assertions: func(_ http.ServerConfig, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "value not found for")
				require.Contains(t, err.Error(), "TLS_KEY_PATH")
			},
		},
		{
			name: "success",
			setup: func() {
				t.Setenv("TLS_KEY_PATH", "/var/ssl/key")
			},
			assertions: func(config http.ServerConfig, err error) {
				require.NoError(t, err)
				require.Equal(
					t,
					http.ServerConfig{
						Port:        8080,
						TLSEnabled:  true,
						TLSCertPath: "/var/ssl/cert",
						TLSKeyPath:  "/var/ssl/key",
					},
					config,
				)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.setup()
			config, err := serverConfig()
			testCase.assertions(config, err)
		})
	}
}
