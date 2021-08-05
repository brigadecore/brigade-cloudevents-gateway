package cloudevents

import (
	"context"

	"github.com/brigadecore/brigade/sdk/v2/core"
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
	eventsClient core.EventsClient
}

// NewService returns an implementation of the Service interface for handling
// CloudEvents.
func NewService(eventsClient core.EventsClient) Service {
	return &service{
		eventsClient: eventsClient,
	}
}

func (s *service) Handle(ctx context.Context, event cloudEvents.Event) error {
	brigadeEvent := core.Event{
		Source: eventSource,
		Type:   eventType,
		Qualifiers: map[string]string{
			"source": event.Source(),
			"type":   event.Type(),
		},
		Payload: string(event.Data()),
	}
	_, err := s.eventsClient.Create(ctx, brigadeEvent)
	return errors.Wrap(err, "error creating brigade event from cloud event")
}
