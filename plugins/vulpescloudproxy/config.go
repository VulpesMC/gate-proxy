package vulpescloudproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	virtualconfigv1grpc "buf.build/gen/go/vulpescloud/protospecs/grpc/go/vulpescloud/virtualconfig/v1/virtualconfigv1grpc"
	virtualconfigv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/virtualconfig/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const ProxyModuleConfigName = "module_proxy"

type ProxyModuleConfig struct {
	MOTD            MOTDSubConfig        `json:"motd"`
	Maintenance     MaintenanceSubConfig `json:"maintenance"`
	FullKickMessage string               `json:"fullKickMessage"`
}

type MOTDSubConfig struct {
	Enabled    bool   `json:"enabled"`
	FirstLine  string `json:"firstLine"`
	SecondLine string `json:"secondLine"`
}

type MaintenanceSubConfig struct {
	Active           bool   `json:"active"`
	FirstLine        string `json:"firstLine"`
	SecondLine       string `json:"secondLine"`
	KickMessage      string `json:"kickMessage"`
	BypassPermission string `json:"bypassPermission"`
}

func DefaultProxyModuleConfig() ProxyModuleConfig {
	return ProxyModuleConfig{
		MOTD: MOTDSubConfig{
			Enabled:    true,
			FirstLine:  "<blue>A <color:#ff6600>VulpesCloud</color> hosted Network!</blue>",
			SecondLine: "<color:#ff6600>Proxy: %proxy%</color>",
		},
		Maintenance: MaintenanceSubConfig{
			FirstLine:        "<blue>A <color:#ff6600>VulpesCloud</color> hosted Network!</blue>",
			SecondLine:       "<red>MAINTENANCE</red>",
			KickMessage:      "<gray>The Network is currently in <red>maintenance</red>!</gray>",
			BypassPermission: "vulpescloud.proxy.maintenance.bypass",
		},
		FullKickMessage: "<red>This Server is currently full!</red>",
	}
}

type proxyModuleConfigAPI struct {
	client virtualconfigv1grpc.VirtualConfigServiceClient
}

func newProxyModuleConfigAPI(conn grpc.ClientConnInterface) *proxyModuleConfigAPI {
	return &proxyModuleConfigAPI{
		client: virtualconfigv1grpc.NewVirtualConfigServiceClient(conn),
	}
}

func (api *proxyModuleConfigAPI) ensure(ctx context.Context) error {
	getCtx, cancelGet := context.WithTimeout(ctx, 5*time.Second)
	response, err := api.client.GetByName(getCtx, &virtualconfigv1.GetByNameRequest{
		Name: ProxyModuleConfigName,
	})
	cancelGet()
	if err == nil {
		if response.GetConfig() == nil {
			return errors.New("proxy module config lookup returned no config")
		}
		return nil
	}
	if status.Code(err) != codes.NotFound {
		return fmt.Errorf("get proxy module config: %w", err)
	}

	data, err := json.Marshal(DefaultProxyModuleConfig())
	if err != nil {
		return fmt.Errorf("encode default proxy module config: %w", err)
	}
	createCtx, cancelCreate := context.WithTimeout(ctx, 5*time.Second)
	defer cancelCreate()
	_, err = api.client.CreateVirtualConfig(createCtx, &virtualconfigv1.CreateVirtualConfigRequest{
		Name:   ProxyModuleConfigName,
		Config: string(data),
	})
	if err != nil {
		return fmt.Errorf("create proxy module config: %w", err)
	}
	return nil
}

func (api *proxyModuleConfigAPI) get(ctx context.Context) (ProxyModuleConfig, error) {
	getCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	response, err := api.client.GetByName(getCtx, &virtualconfigv1.GetByNameRequest{
		Name: ProxyModuleConfigName,
	})
	if err != nil {
		return ProxyModuleConfig{}, fmt.Errorf("get proxy module config: %w", err)
	}
	if response.GetConfig() == nil {
		return ProxyModuleConfig{}, errors.New("proxy module config lookup returned no config")
	}

	config := DefaultProxyModuleConfig()
	if err := json.Unmarshal([]byte(response.GetConfig().GetConfig()), &config); err != nil {
		return ProxyModuleConfig{}, fmt.Errorf("decode proxy module config: %w", err)
	}
	return config, nil
}

type proxyModuleConfigState struct {
	api    *proxyModuleConfigAPI
	mu     sync.RWMutex
	config ProxyModuleConfig
}

func newProxyModuleConfigState(api *proxyModuleConfigAPI, ctx context.Context) (*proxyModuleConfigState, error) {
	if err := api.ensure(ctx); err != nil {
		return nil, err
	}
	config, err := api.get(ctx)
	if err != nil {
		return nil, err
	}
	return &proxyModuleConfigState{api: api, config: config}, nil
}

func (state *proxyModuleConfigState) get() ProxyModuleConfig {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.config
}

func (state *proxyModuleConfigState) reload(ctx context.Context) error {
	config, err := state.api.get(ctx)
	if err != nil {
		return err
	}
	state.mu.Lock()
	state.config = config
	state.mu.Unlock()
	return nil
}
