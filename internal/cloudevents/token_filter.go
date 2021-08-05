package cloudevents

import (
	"log"
	"net/http"
	"strings"

	"github.com/brigadecore/brigade-cloudevents-gateway/internal/crypto"
	libHTTP "github.com/brigadecore/brigade-foundations/http"
	"github.com/cloudevents/sdk-go/v2/binding"
	cloudHTTP "github.com/cloudevents/sdk-go/v2/protocol/http"
)

type TokenFilterConfig interface {
	// AddToken adds a token to the TokenFilterConfig instance's internal map of
	// sources to tokens. It hashes the token as it does this so that plain text
	// tokens do not float around in memory long-term.
	AddToken(source, token string)
	getHashedToken(source string) (string, bool)
}

type tokenFilterConfig struct {
	hashedTokensBySource map[string]string
}

func NewTokenFilterConfig() TokenFilterConfig {
	return &tokenFilterConfig{
		hashedTokensBySource: map[string]string{},
	}
}

func (t *tokenFilterConfig) AddToken(source, token string) {
	// Use the source as a salt
	t.hashedTokensBySource[source] = crypto.Hash(source, token)
}

func (t *tokenFilterConfig) getHashedToken(source string) (string, bool) {
	token, ok := t.hashedTokensBySource[source]
	return token, ok
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
		event, err := binding.ToEvent(
			r.Context(),
			cloudHTTP.NewMessageFromHttpRequest(r),
		)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Find the hashed token that goes along with the source for this CloudEvent
		correctHashedToken, ok := t.config.getHashedToken(event.Source())
		// If one wasn't found, access is denied
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		// Find the access token provided by the client...
		//
		// Try the Authorization header first
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
		// If no token was provided, or once hashed, didn't match what we expected,
		// then access is denied
		if providedToken == "" ||
			crypto.Hash(event.Source(), providedToken) != correctHashedToken {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		// If we get this far, everything checks out. Handle the request.
		handle(w, r)
	}
}
