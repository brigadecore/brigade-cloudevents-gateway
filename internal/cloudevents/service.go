package cloudevents

import (
	"context"
	"encoding/json"
	"log"

	"github.com/brigadecore/brigade/sdk/v3"
	cloudEvents "github.com/cloudevents/sdk-go/v2"
	"github.com/pkg/errors"
)

const (
	eventSource = "brigade.sh/cloudevents"
	eventType   = "cloudevent"
)

// Service is an interface for components that can handle CloudEvents.
// Implementations of this interface are transport-agnostic.
type Service interface {
	// Handle handles a CloudEvent.
	Handle(context.Context, cloudEvents.Event) error
}

type service struct {
	eventsClient sdk.EventsClient
}

// NewService returns an implementation of the Service interface for handling
// CloudEvents.
func NewService(eventsClient sdk.EventsClient) Service {
	return &service{
		eventsClient: eventsClient,
	}
}

func (s *service) Handle(ctx context.Context, event cloudEvents.Event) error {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		err = errors.Wrap(err, "error marshaling cloud event to JSON")
		// Nothing in cloudevents/sdk-go's HTTP protocol bindings seems to log the
		// error we return, so we log it ourselves here.
		log.Println(err)
		return err
	}
	if _, err = s.eventsClient.Create(
		ctx,
		sdk.Event{
			Source: eventSource,
			Type:   eventType,
			Qualifiers: map[string]string{
				"source": event.Source(),
				"type":   event.Type(),
			},
			Payload: string(eventJSON),
		},
		nil,
	); err != nil {
		err = errors.Wrap(err, "error creating brigade event from cloud event")
		// Nothing in cloudevents/sdk-go's HTTP protocol bindings seems to log the
		// error we return, so we log it ourselves here.
		log.Println(err)
	}
	return err
}
