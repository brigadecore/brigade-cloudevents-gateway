package cloudevents

import (
	"context"

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
	brigadeEvent := sdk.Event{
		Source: eventSource,
		Type:   eventType,
		Qualifiers: map[string]string{
			"source": event.Source(),
			"type":   event.Type(),
		},
		Payload: string(event.Data()),
	}
	_, err := s.eventsClient.Create(ctx, brigadeEvent, nil)
	return errors.Wrap(err, "error creating brigade event from cloud event")
}
