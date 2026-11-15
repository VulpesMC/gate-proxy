package vulpescloud

import (
	"context"
	"errors"
	"fmt"
	"io"

	eventspb "buf.build/gen/go/vulpescloud/protospecs/grpc/go/vulpescloud/events/v1/eventsv1grpc"
	eventsv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/events/v1"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EventManager subscribes to VulpesCloud event streams and dispatches received events.
type EventManager struct {
	eventClient eventspb.EventServiceClient
	ctx         context.Context
	log         logr.Logger
}

// NewEventManager creates an event subscriber bound to the given context and logger.
func NewEventManager(
	ctx context.Context,
	eventClient eventspb.EventServiceClient,
	log logr.Logger,
) *EventManager {
	return &EventManager{
		eventClient: eventClient,
		ctx:         ctx,
		log:         log,
	}
}

// EventHandler subscribes to an event ID and dispatches events until the context ends.
func (m *EventManager) EventHandler(
	eventID string,
	method func(*eventsv1.Event, logr.Logger),
) (grpc.ServerStreamingClient[eventsv1.Event], error) {
	stream, err := m.eventClient.Subscribe(m.ctx, &eventsv1.SubscribeRequest{
		EventId: eventID,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to subscribe: %w", err)
	}

	go func() {
		for {
			receivedEvent, err := stream.Recv()
			if err != nil {
				switch {
				case errors.Is(err, io.EOF):
					m.log.Info("Event stream closed by server")
				case m.ctx.Err() != nil || status.Code(err) == codes.Canceled:
					m.log.Info("Event stream cancelled")
				default:
					m.log.Error(err, "Event stream failed")
				}
				return
			}

			method(receivedEvent, m.log)
		}
	}()

	return stream, nil
}
