package vulpescloud

import (
	"context"
	"errors"
	"fmt"
	"time"

	servicesv1grpc "buf.build/gen/go/vulpescloud/protospecs/grpc/go/vulpescloud/services/v1/servicesv1grpc"
	servicesv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/services/v1"
	"google.golang.org/grpc"
)

// ServiceAPI provides reusable lookups for local and remote VulpesCloud services.
type ServiceAPI struct {
	client           servicesv1grpc.ServiceAPIServiceClient
	localServiceUUID string
}

// NewServiceAPI creates service lookups using the given gRPC connection and local UUID.
func NewServiceAPI(conn grpc.ClientConnInterface, localServiceUUID string) *ServiceAPI {
	return &ServiceAPI{
		client:           servicesv1grpc.NewServiceAPIServiceClient(conn),
		localServiceUUID: localServiceUUID,
	}
}

// GetLocalService looks up the service identified as local when this API was created.
func (api *ServiceAPI) GetLocalService(ctx context.Context) (*servicesv1.ServiceDefinition, error) {
	if api.localServiceUUID == "" {
		return nil, errors.New("local service UUID is empty")
	}
	return api.GetServiceByUUID(ctx, api.localServiceUUID)
}

// GetServiceByUUID looks up a VulpesCloud service by its UUID.
func (api *ServiceAPI) GetServiceByUUID(ctx context.Context, uuid string) (*servicesv1.ServiceDefinition, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	response, err := api.client.GetByUuid(lookupCtx, &servicesv1.GetByUuidRequest{Uuid: uuid})
	if err != nil {
		return nil, fmt.Errorf("get service %q: %w", uuid, err)
	}
	service := response.GetService()
	if service == nil {
		return nil, fmt.Errorf("service %q not found", uuid)
	}
	return service, nil
}

// GetAllServices returns all services visible to this node.
func (api *ServiceAPI) GetAllServices(ctx context.Context) ([]*servicesv1.ServiceDefinition, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	response, err := api.client.GetAllServices(lookupCtx, &servicesv1.GetAllServicesRequest{})
	if err != nil {
		return nil, fmt.Errorf("get all services: %w", err)
	}
	return response.GetServices(), nil
}

// UpdatePlayerCount updates the registered player count for a service.
func (api *ServiceAPI) UpdatePlayerCount(
	ctx context.Context,
	service *servicesv1.ServiceDefinition,
	playerCount uint32,
) error {
	updateCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := api.client.UpdatePlayerCount(updateCtx, &servicesv1.UpdatePlayerCountRequest{
		Service:     service,
		PlayerCount: playerCount,
	})
	if err != nil {
		return fmt.Errorf("update player count for service %q: %w", service.GetUuid(), err)
	}
	return nil
}
