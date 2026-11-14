package vulpescloud

import (
	"context"
	"errors"
	"fmt"
	"io"

	eventspb "buf.build/gen/go/vulpescloud/protospecs/grpc/go/vulpescloud/events/v1/eventsv1grpc"
	eventsv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/events/v1"
	"github.com/go-logr/logr"
	"github.com/vulpesmc/gate-proxy/plugins/vulpescloud/client"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var Plugin = proxy.Plugin{
	Name: "Ping",
	Init: func(ctx context.Context, p *proxy.Proxy) error {
		log := logr.FromContextOrDiscard(ctx)
		log.Info("Hello from VulpesCloud plugin!")

		for _, server := range p.Servers() {
			log.Info("Unregistering Server", "serverName", server.ServerInfo().Name())
			p.Unregister(server.ServerInfo())
		}

		client, err := client.Connect()
		if err != nil {
			log.Error(err, "Unable to Connect to Node!")
		}

		eventClient := eventspb.NewEventServiceClient(client)
		_ = EventManager{
			eventClient: eventClient,
			ctx:         ctx,
			log:         log,
		}

		return nil
	},
}

type EventManager struct {
	eventClient eventspb.EventServiceClient
	ctx         context.Context
	log         logr.Logger
}

func (m EventManager) EventHandler(method func(*eventsv1.Event, logr.Logger)) (grpc.ServerStreamingClient[eventsv1.Event], error) {
	stream, err := m.eventClient.Subscribe(m.ctx, &eventsv1.SubscribeRequest{
		EventId: "vulpescloud.events.v1.ServiceStateChangedEvent",
	})

	if err != nil {
		return nil, fmt.Errorf("unable to subscribe: %w", err)
	}

	go func() {
		for {
			event, err := stream.Recv()
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

			method(event, m.log)
		}
	}()

	return stream, nil
}
