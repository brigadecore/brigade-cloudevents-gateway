package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	cloudHTTP "github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/stretchr/testify/require"
)

func TestValidateEventSource(t *testing.T) {
	// callbackReceiver extends *httptest.Server to add channels that can be used
	// to assert that expected callbacks were received.
	type callbackReceiver struct {
		getCallbackCh  chan struct{}
		postCallbackCh chan struct{}
		*httptest.Server
	}
	testCases := []struct {
		name  string
		setup func(
			context.Context,
			*testing.T,
		) (*http.Request, *callbackReceiver)
		assertions func(
			context.Context,
			*testing.T,
			*http.Response,
			*callbackReceiver,
		)
	}{
		{
			name: "method is not OPTIONS",
			setup: func(
				_ context.Context,
				t *testing.T,
			) (*http.Request, *callbackReceiver) {
				r, err := http.NewRequest(http.MethodGet, "/", nil)
				require.NoError(t, err)
				return r, nil
			},
			assertions: func(
				_ context.Context,
				t *testing.T,
				r *http.Response,
				_ *callbackReceiver,
			) {
				require.Equal(t, http.StatusMethodNotAllowed, r.StatusCode)
			},
		},
		{
			name: "WebHook-Request-Callback header NOT set",
			// This tests a synchronous handshake
			setup: func(
				_ context.Context,
				t *testing.T,
			) (*http.Request, *callbackReceiver) {
				r, err := http.NewRequest(http.MethodOptions, "/", nil)
				require.NoError(t, err)
				return r, nil
			},
			assertions: func(
				_ context.Context,
				t *testing.T,
				r *http.Response,
				_ *callbackReceiver,
			) {
				require.Equal(t, http.StatusOK, r.StatusCode)
				require.Equal(t, "*", r.Header.Get("WebHook-Allowed-Origin"))
				require.Equal(
					t,
					strconv.Itoa(cloudHTTP.DefaultAllowedRate),
					r.Header.Get("WebHook-Allowed-Rate"),
				)
				require.Equal(t, http.MethodPost, r.Header.Get("Allow"))
			},
		},
		{
			name: "WebHook-Request-Callback header set",
			// This tests an asynchronous handshake
			setup: func(
				ctx context.Context,
				t *testing.T,
			) (*http.Request, *callbackReceiver) {
				s := &callbackReceiver{
					getCallbackCh:  make(chan struct{}),
					postCallbackCh: make(chan struct{}),
				}
				s.Server = httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						switch r.Method {
						case http.MethodGet:
							// Indicate the GET callback was received
							select {
							case s.getCallbackCh <- struct{}{}:
							case <-ctx.Done():
							}
						case http.MethodPost:
							// Indicate the POST callback was received
							select {
							case s.postCallbackCh <- struct{}{}:
							case <-ctx.Done():
							}
						default:
							require.FailNow(
								t,
								fmt.Sprintf(
									"callback received with unexpected method %s",
									r.Method,
								),
							)
						}
					},
				))
				r, err := http.NewRequest(http.MethodOptions, "/", nil)
				require.NoError(t, err)
				r.Header.Set("WebHook-Request-Callback", s.URL)
				return r, s
			},
			assertions: func(
				ctx context.Context,
				t *testing.T,
				r *http.Response,
				s *callbackReceiver,
			) {
				require.Equal(t, http.StatusOK, r.StatusCode)
				require.Empty(t, r.Header.Get("WebHook-Allowed-Origin"))
				require.Empty(t, r.Header.Get("WebHook-Allowed-Rate"))
				require.Empty(t, r.Header.Get("Allow"))
				// Validate that the GET callback was received
				select {
				case <-s.getCallbackCh:
				case <-ctx.Done():
					require.FailNow(t, "did not receive GET callback by deadline")
				}
				// Validate that the POST callback was received
				select {
				case <-s.postCallbackCh:
				case <-ctx.Done():
					require.FailNow(t, "did not receive POST callback by deadline")
				}
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// One test case involves an asynchronous handshake that takes 10 seconds.
			// For good measure, we're setting a deadline of 12 seconds.
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			r, s := testCase.setup(ctx, t)
			if s != nil {
				defer s.Close()
			}
			rr := httptest.NewRecorder()
			ValidateEventSource(rr, r)
			testCase.assertions(ctx, t, rr.Result(), s)
		})
	}
}
