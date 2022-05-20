package http

import (
	"net/http"
	"strings"

	"github.com/brigadecore/brigade-foundations/crypto"
	libHTTP "github.com/brigadecore/brigade-foundations/http"
)

// TokenFilterConfig is the interface for a component that encapsulates token
// filter configuration.
type TokenFilterConfig interface {
	// AddToken adds a token to the TokenFilterConfig implementation's instance's
	// internal list of tokens. Implementations MUST hash the provided token
	// before addition to the list so that plain text tokens do not float around
	// in memory long-term.
	AddToken(string)
	getHashedTokens() []string
}

// tokenFilterConfig encapsulates token filter configuration.
type tokenFilterConfig struct {
	hashedTokens []string
}

// NewTokenFilterConfig returns an initialized implementation of the
// TokenFilterConfig interface.
func NewTokenFilterConfig() TokenFilterConfig {
	return &tokenFilterConfig{
		hashedTokens: []string{},
	}
}

func (t *tokenFilterConfig) AddToken(token string) {
	t.hashedTokens = append(t.hashedTokens, crypto.Hash("", token))
}

func (t *tokenFilterConfig) getHashedTokens() []string {
	return t.hashedTokens
}

// tokenFilter is a component that implements the http.Filter interface and can
// conditionally allow or disallow a request on the basis of a recognized token
// having been provided.
type tokenFilter struct {
	config TokenFilterConfig
}

// NewTokenFilter returns a component that implements the http.Filter interface
// and can conditionally allow or disallow a request on the basis of a
// recognized token having been provided.
func NewTokenFilter(config TokenFilterConfig) libHTTP.Filter {
	return &tokenFilter{
		config: config,
	}
}

func (t *tokenFilter) Decorate(handle http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Look for a token in the Authorization header first
		var providedToken string
		if headerValue := r.Header.Get("Authorization"); headerValue != "" {
			if headerValueParts := strings.SplitN(
				headerValue,
				" ",
				2,
			); len(headerValueParts) == 2 && headerValueParts[0] == "Bearer" {
				providedToken = headerValueParts[1]
			}
		}
		// Then try the access_token query param
		if providedToken == "" {
			providedToken = r.URL.Query().Get("access_token")
		}
		// If no token was provided, then access is denied
		if providedToken == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		hashedProvidedToken := crypto.Hash("", providedToken)
		var allowed bool
		for _, hashedAllowedToken := range t.config.getHashedTokens() {
			if hashedProvidedToken == hashedAllowedToken {
				allowed = true
				break
			}
		}
		if !allowed {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		// If we get this far, everything checks out. Handle the request.
		handle(w, r)
	}
}
