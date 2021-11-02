package cloudevents

import (
	"context"
	"errors"
	"testing"

	"github.com/brigadecore/brigade/sdk/v2/core"
	coreTesting "github.com/brigadecore/brigade/sdk/v2/testing/core"
	cloudEvents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/require"
)

func TestNewService(t *testing.T) {
	s, ok := NewService(
		// Totally unusable client that is enough to fulfill the dependencies for
		// this test...
		&coreTesting.MockEventsClient{
			LogsClient: &coreTesting.MockLogsClient{},
		},
	).(*service)
	require.True(t, ok)
	require.NotNil(t, s.eventsClient)
}

func TestHandle(t *testing.T) {
	const testSource = "foo"
	const testType = "bar"
	const testData = `{"foo": "bar"}`
	testCloudEvent := cloudEvents.NewEvent()
	testCloudEvent.SetSource(testSource)
	testCloudEvent.SetType(testType)
	err := testCloudEvent.SetData("application/json", testData)
	require.NoError(t, err)
	testCases := []struct {
		name       string
		service    *service
		assertions func(error)
	}{
		{
			name: "error creating brigade event",
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(context.Context, core.Event) (core.EventList, error) {
						return core.EventList{}, errors.New("something went wrong")
					},
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(
					t,
					err.Error(),
					"error creating brigade event from cloud event",
				)
				require.Contains(t, err.Error(), "something went wrong")
			},
		},
		{
			name: "success",
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						require.Equal(t, eventSource, event.Source)
						require.Equal(t, eventType, event.Type)
						require.Equal(
							t,
							map[string]string{
								"source": testSource,
								"type":   testType,
							},
							event.Qualifiers,
						)
						require.NotEmpty(t, event.Payload)
						return core.EventList{}, nil
					},
				},
			},
			assertions: func(err error) {
				require.NoError(t, err)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.service.Handle(context.Background(), testCloudEvent)
			testCase.assertions(err)
		})
	}
}
