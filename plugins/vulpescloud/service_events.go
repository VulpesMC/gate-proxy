package vulpescloud

import (
	"context"
	"time"

	eventspb "buf.build/gen/go/vulpescloud/protospecs/grpc/go/vulpescloud/events/v1/eventsv1grpc"
	eventsv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/events/v1"
	servicesv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/services/v1"
	"google.golang.org/protobuf/types/known/anypb"
)

// ServiceStateChangedEventID is the VulpesCloud event ID for service state transitions.
const ServiceStateChangedEventID = "vulpescloud.events.v1.ServiceStateChangedEvent"

// PublishServiceStateChanged publishes a service state transition to other VulpesCloud nodes.
func PublishServiceStateChanged(
	ctx context.Context,
	eventClient eventspb.EventServiceClient,
	service *servicesv1.ServiceDefinition,
	oldState servicesv1.ServiceState,
	newState servicesv1.ServiceState,
) error {
	data, err := anypb.New(&eventsv1.ServiceStateChangedEvent{
		Service:  service,
		OldState: oldState,
		NewState: newState,
	})
	if err != nil {
		return err
	}

	_, err = eventClient.Publish(ctx, &eventsv1.PublishRequest{
		ForwardToOtherNodes: true,
		Event: &eventsv1.Event{
			Id:        ServiceStateChangedEventID,
			Timestamp: time.Now().UnixMilli(),
			Data:      data,
		},
	})
	return err
}
