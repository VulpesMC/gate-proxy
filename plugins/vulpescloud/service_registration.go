package vulpescloud

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	eventsv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/events/v1"
	nodev1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/node/v1"
	servicesv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/services/v1"
	tasksv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/tasks/v1"
	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

// ServiceRegistrationHandler registers running VulpesCloud server services with Gate.
type ServiceRegistrationHandler struct {
	proxy  *proxy.Proxy
	events *EventManager
}

// NewServiceRegistrationHandler creates a handler for service-state events.
func NewServiceRegistrationHandler(
	p *proxy.Proxy,
	events *EventManager,
) *ServiceRegistrationHandler {
	return &ServiceRegistrationHandler{
		proxy:  p,
		events: events,
	}
}

// Subscribe starts handling service-state events.
func (h *ServiceRegistrationHandler) Subscribe() error {
	_, err := h.events.EventHandler(ServiceStateChangedEventID, h.Handle)
	return err
}

// RegisterExisting registers services that are already running when the plugin starts.
func (h *ServiceRegistrationHandler) RegisterExisting(
	services []*servicesv1.ServiceDefinition,
	log logr.Logger,
) {
	for _, service := range services {
		if service.GetState() != servicesv1.ServiceState_SERVICE_STATE_RUNNING {
			continue
		}
		h.registerService(service, log)
	}
}

// Handle registers a running server service with Gate.
func (h *ServiceRegistrationHandler) Handle(event *eventsv1.Event, log logr.Logger) {
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

	service := changed.GetService()
	if service == nil || service.GetTask() == nil {
		log.Error(errors.New("event is missing service or task"), "Unable to register service")
		return
	}

	log.Info("Service state changed",
		"service", service.GetUuid(),
		"oldState", changed.GetOldState(),
		"newState", changed.GetNewState(),
	)

	task := service.GetTask()
	if !isGateServerTask(task) {
		return
	}

	switch changed.GetNewState() {
	case servicesv1.ServiceState_SERVICE_STATE_RUNNING:
		h.registerService(service, log)
	case servicesv1.ServiceState_SERVICE_STATE_STOPPED:
		h.unregisterService(service, log)
	}
}

func isGateServerTask(task *tasksv1.TaskDefinition) bool {
	return task != nil &&
		task.GetServerSoftware().GetType() == nodev1.SoftwareType_SOFTWARE_TYPE_SERVER
}

func serviceName(service *servicesv1.ServiceDefinition) string {
	return fmt.Sprintf("%s-%d", service.GetTask().GetName(), service.GetOrderedId())
}

func (h *ServiceRegistrationHandler) registerService(
	service *servicesv1.ServiceDefinition,
	log logr.Logger,
) {
	if service == nil || !isGateServerTask(service.GetTask()) {
		return
	}
	address, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(
		service.GetHostname(),
		strconv.Itoa(int(service.GetPort())),
	))
	if err != nil {
		log.Error(err, "Unable to resolve service address", "service", service.GetUuid())
		return
	}

	serverName := serviceName(service)
	if h.proxy.Server(serverName) != nil {
		return
	}
	if _, err := h.proxy.Register(proxy.NewServerInfo(serverName, address)); err != nil {
		log.Error(err, "Unable to register service", "service", service.GetUuid(), "serverName", serverName)
	}
}

func (h *ServiceRegistrationHandler) unregisterService(
	service *servicesv1.ServiceDefinition,
	log logr.Logger,
) {
	serverName := serviceName(service)
	server := h.proxy.Server(serverName)
	if server == nil {
		return
	}
	if !h.proxy.Unregister(server.ServerInfo()) {
		log.Error(nil, "Unable to unregister service", "service", service.GetUuid(), "serverName", serverName)
	}
}
