package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/brigadecore/brigade-foundations/crypto"
	"github.com/stretchr/testify/require"
)

func TestNewTokenFilterConfig(t *testing.T) {
	config, ok := NewTokenFilterConfig().(*tokenFilterConfig)
	require.True(t, ok)
	require.NotNil(t, config.hashedTokens)
}

func TestAddToken(t *testing.T) {
	const testToken = "foo"
	config, ok := NewTokenFilterConfig().(*tokenFilterConfig)
	require.True(t, ok)
	require.Empty(t, config.hashedTokens)
	config.AddToken(testToken)
	require.Len(t, config.hashedTokens, 1)
	require.Equal(t, crypto.Hash("", testToken), config.hashedTokens[0])
}

func TestGetHashedTokens(t *testing.T) {
	testHashedTokens := []string{"foo", "bar"}
	config := tokenFilterConfig{
		hashedTokens: testHashedTokens,
	}
	require.Equal(t, testHashedTokens, config.getHashedTokens())
}

func TestNewTokenFilter(t *testing.T) {
	testConfig := NewTokenFilterConfig()
	filter, ok := NewTokenFilter(testConfig).(*tokenFilter)
	require.True(t, ok)
	require.Equal(t, testConfig, filter.config)
}

func TestTokenFilter(t *testing.T) {
	testConfig := NewTokenFilterConfig()
	const testToken = "bar"
	testConfig.AddToken(testToken)
	testCases := []struct {
		name       string
		filter     *tokenFilter
		setup      func() *http.Request
		assertions func(handlerCalled bool, r *http.Response)
	}{
		{
			name: "valid token provided in Authorization header",
			filter: &tokenFilter{
				config: testConfig,
			},
			setup: func() *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/", nil)
				require.NoError(t, err)
				req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", testToken))
				return req
			},
			assertions: func(handlerCalled bool, r *http.Response) {
				require.Equal(t, http.StatusOK, r.StatusCode)
				require.True(t, handlerCalled)
			},
		},
		{
			name: "valid token provided as query parameter",
			filter: &tokenFilter{
				config: testConfig,
			},
			setup: func() *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/", nil)
				require.NoError(t, err)
				q := url.Values{}
				q.Set("access_token", testToken)
				req.URL.RawQuery = q.Encode()
				return req
			},
			assertions: func(handlerCalled bool, r *http.Response) {
				require.Equal(t, http.StatusOK, r.StatusCode)
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
				return req
			},
			assertions: func(handlerCalled bool, r *http.Response) {
				require.Equal(t, http.StatusForbidden, r.StatusCode)
				require.False(t, handlerCalled)
			},
		},
		{
			name: "invalid token provided",
			filter: &tokenFilter{
				config: testConfig,
			},
			setup: func() *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/", nil)
				require.NoError(t, err)
				req.Header.Add("Authorization", "Bearer bogus-token")
				return req
			},
			assertions: func(handlerCalled bool, r *http.Response) {
				require.Equal(t, http.StatusForbidden, r.StatusCode)
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
			res := rr.Result()
			defer res.Body.Close()
			testCase.assertions(handlerCalled, res)
		})
	}
}
