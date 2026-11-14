package vulpescloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	eventspb "buf.build/gen/go/vulpescloud/protospecs/grpc/go/vulpescloud/events/v1/eventsv1grpc"
	servicesv1grpc "buf.build/gen/go/vulpescloud/protospecs/grpc/go/vulpescloud/services/v1/servicesv1grpc"
	eventsv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/events/v1"
	servicesv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/services/v1"
	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"github.com/vulpesmc/gate-proxy/plugins/vulpescloud/client"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
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
		eventManager := EventManager{
			eventClient: eventClient,
			ctx:         ctx,
			log:         log,
		}

		_, err = eventManager.EventHandler("vulpescloud.events.v1.ServiceStateChangedEvent",
			func(event *eventsv1.Event, log logr.Logger) {
				data := event.GetData()
				if data == nil {
					log.Error(nil, "Event has no data")
					return
				}

				changed := new(eventsv1.ServiceStateChangedEvent)
				if err := data.UnmarshalTo(changed); err != nil {
					log.Error(err, "Unable to decode service state event")
					return
				}

				log.Info("Service state changed",
					"service", changed.GetService().GetUuid(),
					"oldState", changed.GetOldState(),
					"newState", changed.GetNewState(),
				)
			},
		)

		if err != nil {
			log.Error(err, "Failed to Listen to ServiceStateChangeEvent!")
		}

		mgr := p.Event()
		event.Subscribe(mgr, 0, func(e *proxy.ReadyEvent) {
			serviceUUID := os.Getenv("serviceUUID")

			serviceClient := servicesv1grpc.NewServiceAPIServiceClient(client)
			lookupCtx, cancelLookup := context.WithTimeout(ctx, 5*time.Second)
			response, err := serviceClient.GetByUuid(lookupCtx, &servicesv1.GetByUuidRequest{
				Uuid: serviceUUID,
			})
			cancelLookup()
			if err != nil {
				log.Error(err, "Unable to get local service")
				return
			}
			localService := response.GetService()
			if localService == nil {
				log.Error(errors.New("service not found"), "Unable to get local service", "uuid", serviceUUID)
				return
			}

			data, err := anypb.New(&eventsv1.ServiceStateChangedEvent{
				Service:  localService,
				OldState: localService.GetState(),
				NewState: servicesv1.ServiceState_SERVICE_STATE_RUNNING,
			})
			if err != nil {
				log.Error(err, "Unable to encode service state event")
				return
			}

			publishCtx, cancelPublish := context.WithTimeout(ctx, 5*time.Second)
			defer cancelPublish()
			_, err = eventClient.Publish(publishCtx, &eventsv1.PublishRequest{
				ForwardToOtherNodes: true,
				Event: &eventsv1.Event{
					Id:        "vulpescloud.events.v1.ServiceStateChangedEvent",
					Timestamp: time.Now().UnixMilli(),
					Data:      data,
				},
			})
			if err != nil {
				log.Error(err, "Unable to publish service state event")
			}
		})

		return nil
	},
}

type EventManager struct {
	eventClient eventspb.EventServiceClient
	ctx         context.Context
	log         logr.Logger
}

func (m EventManager) EventHandler(eventId string, method func(*eventsv1.Event, logr.Logger)) (grpc.ServerStreamingClient[eventsv1.Event], error) {
	stream, err := m.eventClient.Subscribe(m.ctx, &eventsv1.SubscribeRequest{
		EventId: eventId,
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
