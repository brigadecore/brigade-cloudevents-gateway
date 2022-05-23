package http

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/brigadecore/brigade-foundations/version"
	cloudHTTP "github.com/cloudevents/sdk-go/v2/protocol/http"
)

// ValidateEventSource responds to HTTP OPTIONS requests sent by a CloudEvents
// 1.0 source as part of the spec's abuse protection scheme.
func ValidateEventSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodOptions {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	headers := make(http.Header)
	headers.Set("WebHook-Allowed-Origin", "*")
	headers.Set(
		"WebHook-Allowed-Rate",
		strconv.Itoa(cloudHTTP.DefaultAllowedRate),
	)
	headers.Set("Allow", http.MethodPost)

	// Complete the handshake asynchronously if a callback URL was provided...
	if callbackURL :=
		r.Header.Get("WebHook-Request-Callback"); callbackURL != "" {
		headers.Set("User-Agent", userAgentHeaderValue())
		// The spec is somewhat vague here. It says we can send either GET or POST,
		// but it doesn't explicitly state that the receiver (the event source we're
		// validating) has to accept both. To cover our bases and ensure
		// compatibility with all CloudEvent 1.0-compatible event sources, we send
		// both. If one of the two fails, it's logged and it's not a big deal.
		//
		// See https://github.com/cloudevents/spec/issues/1018
		go executeSourceValidationCallback(http.MethodGet, callbackURL, headers)
		go executeSourceValidationCallback(http.MethodPost, callbackURL, headers)
		return
	}

	for k := range headers {
		w.Header().Set(k, headers.Get(k))
	}
}

func executeSourceValidationCallback(method, url string, headers http.Header) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		log.Printf(
			"error preparing HTTP %s for validation callback URL %s: %s",
			method,
			url,
			err,
		)
		return
	}
	for k := range headers {
		req.Header.Set(k, headers.Get(k))
	}
	// Experience has revealed that if we don't wait for at least a little while
	// before executing the callback, Azure Event Grid responds with a 200 as if
	// everything is OK, but does NOT actually complete the handshake.
	// Since we care about compatibility with Azure Event Grid, we'll tolerate
	// this. And because these callbacks are specifically meant to facilitate
	// asynchronous handshakes, this short delay really shouldn't cause any issue
	// for OTHER CloudEvents 1.0-compatible event sources.
	<-time.After(10 * time.Second)
	if _, err = http.DefaultClient.Do(req); err != nil {
		log.Printf(
			"error executing HTTP %s for validation callback URL %s: %s",
			method,
			url,
			err,
		)
	}
}

func userAgentHeaderValue() string {
	return fmt.Sprintf("Go/%s (%s-%s) brigade-cloudevents-gateway/%s",
		runtime.Version(),
		runtime.GOARCH,
		runtime.GOOS,
		version.Version(),
	)
}
