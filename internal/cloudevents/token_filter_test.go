package cloudevents

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brigadecore/brigade-cloudevents-gateway/internal/crypto"
	"github.com/stretchr/testify/require"
)

func TestNewTokenFilterConfig(t *testing.T) {
	config := NewTokenFilterConfig()
	require.NotNil(t, config.(*tokenFilterConfig).hashedTokensBySource)
}

func TestAddToken(t *testing.T) {
	const testSource = "foo"
	const testToken = "bar"
	config := NewTokenFilterConfig().(*tokenFilterConfig)
	config.AddToken(testSource, testToken)
	hashedToken, ok :=
		config.hashedTokensBySource[testSource]
	require.True(t, ok)
	require.Equal(
		t,
		crypto.Hash(testSource, testToken),
		hashedToken,
	)
}

func TestGetHashedToken(t *testing.T) {
	const testSource = "foo"
	testHashedToken := crypto.Hash(testSource, "bar")
	config := NewTokenFilterConfig().(*tokenFilterConfig)
	config.hashedTokensBySource[testSource] = testHashedToken
	hashedToken, ok := config.getHashedToken(testSource)
	require.True(t, ok)
	require.Equal(t, testHashedToken, hashedToken)
}

func TestNewTokenFilter(t *testing.T) {
	testConfig := NewTokenFilterConfig()
	filter := NewTokenFilter(testConfig).(*tokenFilter)
	require.Equal(t, testConfig, filter.config)
}

func TestTokenFilter(t *testing.T) {
	testEmptyConfig := NewTokenFilterConfig()
	testConfig := NewTokenFilterConfig()
	const testSource = "foo"
	const testToken = "bar"
	testConfig.AddToken(testSource, testToken)
	testCases := []struct {
		name       string
		filter     *tokenFilter
		setup      func() *http.Request
		assertions func(handlerCalled bool, rr *httptest.ResponseRecorder)
	}{
		{
			name: "cannot parse event from request",
			filter: &tokenFilter{
				config: testEmptyConfig,
			},
			setup: func() *http.Request {
				// This request lacks anything that makes it look like a CloudEvent
				req, err := http.NewRequest(http.MethodPost, "/", nil)
				require.NoError(t, err)
				return req
			},
			assertions: func(handlerCalled bool, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusInternalServerError, rr.Code)
				require.False(t, handlerCalled)
			},
		},
		{
			name: "hashed token not found for source",
			filter: &tokenFilter{
				config: testEmptyConfig,
			},
			setup: func() *http.Request {
				// This request looks like a CloudEvent, but the source isn't recognized
				req, err := http.NewRequest(http.MethodPost, "/", nil)
				require.NoError(t, err)
				req.Header.Add("ce-id", "1234-1234-1234")
				req.Header.Add("ce-specversion", "1.0")
				req.Header.Add("ce-source", testSource)
				req.Header.Add("ce-type", "myevent")
				req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", testToken))
				return req
			},
			assertions: func(handlerCalled bool, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusForbidden, rr.Code)
				require.False(t, handlerCalled)
			},
		},
		{
			name: "valid token provided as header",
			filter: &tokenFilter{
				config: testConfig,
			},
			setup: func() *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/", nil)
				require.NoError(t, err)
				req.Header.Add("ce-id", "1234-1234-1234")
				req.Header.Add("ce-specversion", "1.0")
				req.Header.Add("ce-source", testSource)
				req.Header.Add("ce-type", "myevent")
				req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", testToken))
				return req
			},
			assertions: func(handlerCalled bool, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, rr.Code)
				require.True(t, handlerCalled)
			},
		},
		{
			name: "valid token provided as query param",
			filter: &tokenFilter{
				config: testConfig,
			},
			setup: func() *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/", nil)
				require.NoError(t, err)
				req.Header.Add("ce-id", "1234-1234-1234")
				req.Header.Add("ce-specversion", "1.0")
				req.Header.Add("ce-source", testSource)
				req.Header.Add("ce-type", "myevent")
				q := req.URL.Query()
				q.Set("access_token", testToken)
				req.URL.RawQuery = q.Encode()
				return req
			},
			assertions: func(handlerCalled bool, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, rr.Code)
				require.True(t, handlerCalled)
			},
		},
		{
			name: "no token provided",
			filter: &tokenFilter{
				config: testConfig,
			},
			setup: func() *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/", nil)
				require.NoError(t, err)
				req.Header.Add("ce-id", "1234-1234-1234")
				req.Header.Add("ce-specversion", "1.0")
				req.Header.Add("ce-source", testSource)
				req.Header.Add("ce-type", "myevent")
				return req
			},
			assertions: func(handlerCalled bool, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusForbidden, rr.Code)
				require.False(t, handlerCalled)
			},
		},
		{
			name: "wrong token provided",
			filter: &tokenFilter{
				config: testConfig,
			},
			setup: func() *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/", nil)
				require.NoError(t, err)
				req.Header.Add("ce-id", "1234-1234-1234")
				req.Header.Add("ce-specversion", "1.0")
				req.Header.Add("ce-source", testSource)
				req.Header.Add("ce-type", "myevent")
				req.Header.Add("Authorization", "Bearer bogus-token")
				return req
			},
			assertions: func(handlerCalled bool, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusForbidden, rr.Code)
				require.False(t, handlerCalled)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := testCase.setup()
			handlerCalled := false
			testCase.filter.Decorate(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusOK)
			})(rr, req)
			testCase.assertions(handlerCalled, rr)
		})
	}
}
