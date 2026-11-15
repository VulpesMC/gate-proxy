package vulpescloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	virtualconfigv1grpc "buf.build/gen/go/vulpescloud/protospecs/grpc/go/vulpescloud/virtualconfig/v1/virtualconfigv1grpc"
	virtualconfigv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/virtualconfig/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const ConnectorConfigName = "vc_connector"

type ConnectorConfig struct {
	HubCommandConfig                   HubCommandConfig `json:"hubCommandConfig"`
	DisconnectNoAvailableServerMessage string           `json:"disconnectNoAvailableServerMessage"`
	Prefix                             string           `json:"prefix"`
}

type HubCommandConfig struct {
	Enabled bool `json:"enabled"`
}

type ConnectorConfigAPI struct {
	client virtualconfigv1grpc.VirtualConfigServiceClient
}

func NewConnectorConfigAPI(conn grpc.ClientConnInterface) *ConnectorConfigAPI {
	return &ConnectorConfigAPI{
		client: virtualconfigv1grpc.NewVirtualConfigServiceClient(conn),
	}
}

func DefaultConnectorConfig() ConnectorConfig {
	return ConnectorConfig{
		HubCommandConfig:                   HubCommandConfig{Enabled: true},
		DisconnectNoAvailableServerMessage: "<red>There is no available server for you to connect to!</red>",
		Prefix:                             "<gray>[<gradient:#EE660A:#D9BC40>VulpesCloud</gradient>]</gray> ",
	}
}

// Ensure creates the default connector config only when it does not exist.
func (api *ConnectorConfigAPI) Ensure(ctx context.Context) error {
	getCtx, cancelGet := context.WithTimeout(ctx, 5*time.Second)
	response, err := api.client.GetByName(getCtx, &virtualconfigv1.GetByNameRequest{Name: ConnectorConfigName})
	cancelGet()
	if err == nil {
		if response.GetConfig() == nil {
			return errors.New("connector config lookup returned no config")
		}
		return nil
	}
	if status.Code(err) != codes.NotFound {
		return fmt.Errorf("get connector config: %w", err)
	}

	config, err := json.Marshal(DefaultConnectorConfig())
	if err != nil {
		return fmt.Errorf("encode default connector config: %w", err)
	}
	createCtx, cancelCreate := context.WithTimeout(ctx, 5*time.Second)
	defer cancelCreate()
	_, err = api.client.CreateVirtualConfig(createCtx, &virtualconfigv1.CreateVirtualConfigRequest{
		Name:   ConnectorConfigName,
		Config: string(config),
	})
	if err != nil {
		return fmt.Errorf("create connector config: %w", err)
	}
	return nil
}

// Get loads connector config and applies defaults for omitted fields.
func (api *ConnectorConfigAPI) Get(ctx context.Context) (*ConnectorConfig, error) {
	getCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	response, err := api.client.GetByName(getCtx, &virtualconfigv1.GetByNameRequest{Name: ConnectorConfigName})
	if err != nil {
		return nil, fmt.Errorf("get connector config: %w", err)
	}
	if response.GetConfig() == nil {
		return nil, errors.New("connector config lookup returned no config")
	}

	config := DefaultConnectorConfig()
	if err := json.Unmarshal([]byte(response.GetConfig().GetConfig()), &config); err != nil {
		return nil, fmt.Errorf("decode connector config: %w", err)
	}
	return &config, nil
}
