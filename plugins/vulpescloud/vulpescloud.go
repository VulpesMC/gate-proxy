package vulpescloud

import (
	"context"
	"os"
	"time"

	eventspb "buf.build/gen/go/vulpescloud/protospecs/grpc/go/vulpescloud/events/v1/eventsv1grpc"
	servicesv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/services/v1"
	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"github.com/vulpesmc/gate-proxy/plugins/vulpescloud/client"
	"go.minekube.com/gate/pkg/edition/java/proxy"
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

		conn, err := client.Connect()
		if err != nil {
			log.Error(err, "Unable to Connect to Node!")
			return err
		}

		eventClient := eventspb.NewEventServiceClient(conn)
		eventManager := NewEventManager(ctx, eventClient, log)
		serviceAPI := NewServiceAPI(conn, os.Getenv("serviceUUID"))
		configAPI := NewConnectorConfigAPI(conn)
		maintenanceAPI := NewNodeMaintenanceConfigAPI(conn)

		if err := configAPI.Ensure(ctx); err != nil {
			log.Error(err, "Unable to initialize connector config")
			return err
		}
		registrationHandler := NewServiceRegistrationHandler(p, eventManager)

		if err := registrationHandler.Subscribe(); err != nil {
			log.Error(err, "Failed to Listen to ServiceStateChangeEvent!")
			return err
		}

		services, err := serviceAPI.GetAllServices(ctx)
		if err != nil {
			log.Error(err, "Unable to get services during startup")
			return err
		}
		registrationHandler.RegisterExisting(services, log)

		mgr := p.Event()
		playerEvents := NewPlayerEventHandler(
			ctx,
			p,
			serviceAPI,
			configAPI,
			maintenanceAPI,
			conn,
			eventClient,
			eventManager,
			log,
		)
		if err := playerEvents.Register(mgr); err != nil {
			log.Error(err, "Unable to register player event handlers")
			return err
		}

		event.Subscribe(mgr, 0, func(e *proxy.ReadyEvent) {
			localService, err := serviceAPI.GetLocalService(ctx)
			if err != nil {
				log.Error(err, "Unable to get local service")
				return
			}

			publishCtx, cancelPublish := context.WithTimeout(ctx, 5*time.Second)
			defer cancelPublish()
			if err := PublishServiceStateChanged(
				publishCtx,
				eventClient,
				localService,
				localService.GetState(),
				servicesv1.ServiceState_SERVICE_STATE_RUNNING,
			); err != nil {
				log.Error(err, "Unable to publish service state event")
			}
		})

		return nil
	},
}
