package main

// nolint: lll
import (
	"encoding/json"
	"io/ioutil"

	"github.com/brigadecore/brigade-cloudevents-gateway/internal/cloudevents"
	"github.com/brigadecore/brigade-foundations/file"
	"github.com/brigadecore/brigade-foundations/http"
	"github.com/brigadecore/brigade-foundations/os"
	"github.com/brigadecore/brigade/sdk/v3/restmachinery"
	"github.com/pkg/errors"
)

// apiClientConfig populates the Brigade SDK's APIClientOptions from
// environment variables.
func apiClientConfig() (string, string, restmachinery.APIClientOptions, error) {
	opts := restmachinery.APIClientOptions{}
	address, err := os.GetRequiredEnvVar("API_ADDRESS")
	if err != nil {
		return address, "", opts, err
	}
	token, err := os.GetRequiredEnvVar("API_TOKEN")
	if err != nil {
		return address, token, opts, err
	}
	opts.AllowInsecureConnections, err =
		os.GetBoolFromEnvVar("API_IGNORE_CERT_WARNINGS", false)
	return address, token, opts, err
}

func tokenFilterConfig() (cloudevents.TokenFilterConfig, error) {
	config := cloudevents.NewTokenFilterConfig()
	sourceTokensPath, err := os.GetRequiredEnvVar("SOURCE_TOKENS_PATH")
	if err != nil {
		return config, err
	}
	var exists bool
	if exists, err = file.Exists(sourceTokensPath); err != nil {
		return config, err
	}
	if !exists {
		return config, errors.Errorf("file %s does not exist", sourceTokensPath)
	}
	sourceTokenBytes, err := ioutil.ReadFile(sourceTokensPath)
	if err != nil {
		return config, err
	}
	plainTextSourceTokens := map[string]string{}
	if err :=
		json.Unmarshal(sourceTokenBytes, &plainTextSourceTokens); err != nil {
		return config, err
	}
	for source, token := range plainTextSourceTokens {
		config.AddToken(source, token)
	}
	return config, nil
}

// serverConfig populates configuration for the HTTP/S server from environment
// variables.
func serverConfig() (http.ServerConfig, error) {
	config := http.ServerConfig{}
	var err error
	config.Port, err = os.GetIntFromEnvVar("PORT", 8080)
	if err != nil {
		return config, err
	}
	config.TLSEnabled, err = os.GetBoolFromEnvVar("TLS_ENABLED", false)
	if err != nil {
		return config, err
	}
	if config.TLSEnabled {
		config.TLSCertPath, err = os.GetRequiredEnvVar("TLS_CERT_PATH")
		if err != nil {
			return config, err
		}
		config.TLSKeyPath, err = os.GetRequiredEnvVar("TLS_KEY_PATH")
		if err != nil {
			return config, err
		}
	}
	return config, nil
}
