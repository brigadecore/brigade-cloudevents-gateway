package main

import (
	"log"
	"net/http"

	"github.com/brigadecore/brigade-cloudevents-gateway/internal/cloudevents"
	libHTTP "github.com/brigadecore/brigade-foundations/http"
	"github.com/brigadecore/brigade-foundations/signals"
	"github.com/brigadecore/brigade-foundations/version"
	"github.com/brigadecore/brigade/sdk/v2/core"
	"github.com/cloudevents/sdk-go/v2/client"
	cloudHTTP "github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/gorilla/mux"
)

func main() {

	log.Printf(
		"Starting Brigade CloudEvents Gateway -- version %s -- commit %s",
		version.Version(),
		version.Commit(),
	)

	ctx := signals.Context()

	var cloudEventsService cloudevents.Service
	{
		address, token, opts, err := apiClientConfig()
		if err != nil {
			log.Fatal(err)
		}
		cloudEventsService = cloudevents.NewService(
			core.NewEventsClient(address, token, &opts),
		)
	}

	var cloudEventsHandler *client.EventReceiver
	{
		proto, err := cloudHTTP.New()
		if err != nil {
			log.Fatal(err)
		}
		cloudEventsHandler, err =
			client.NewHTTPReceiveHandler(ctx, proto, cloudEventsService.Handle)
		if err != nil {
			log.Fatal(err)
		}
	}

	var tokenFilter libHTTP.Filter
	{
		config, err := tokenFilterConfig()
		if err != nil {
			log.Fatal(err)
		}
		tokenFilter = cloudevents.NewTokenFilter(config)
	}

	var server libHTTP.Server
	{
		router := mux.NewRouter()
		router.StrictSlash(true)
		router.Handle(
			"/events",
			http.HandlerFunc( // Make a handler from a function
				tokenFilter.Decorate(cloudEventsHandler.ServeHTTP),
			),
		).Methods(http.MethodPost)
		router.HandleFunc("/healthz", libHTTP.Healthz).Methods(http.MethodGet)
		serverConfig, err := serverConfig()
		if err != nil {
			log.Fatal(err)
		}
		server = libHTTP.NewServer(router, &serverConfig)
	}

	log.Println(
		server.ListenAndServe(ctx),
	)
}
