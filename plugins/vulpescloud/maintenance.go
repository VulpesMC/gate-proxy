package vulpescloud

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	virtualconfigv1grpc "buf.build/gen/go/vulpescloud/protospecs/grpc/go/vulpescloud/virtualconfig/v1/virtualconfigv1grpc"
	virtualconfigv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/virtualconfig/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const NodeMaintenanceConfigName = "vc_node_maintenance"

type NodeMaintenanceConfig struct {
	JoinPermission               *string                         `json:"joinPermission"`
	CountMaintenanceServices     bool                            `json:"countMaintenanceServices"`
	MaintenanceOnStopDuringDrain bool                            `json:"maintenanceOnStopDuringDrain"`
	Nodes                        map[string]NodeMaintenanceEntry `json:"nodes"`
}

type NodeMaintenanceEntry struct {
	Maintenance    bool    `json:"maintenance"`
	JoinPermission *string `json:"joinPermission"`
}

func DefaultNodeMaintenanceConfig() NodeMaintenanceConfig {
	return NodeMaintenanceConfig{
		CountMaintenanceServices:     true,
		MaintenanceOnStopDuringDrain: true,
		Nodes:                        map[string]NodeMaintenanceEntry{},
	}
}

func (config *NodeMaintenanceConfig) IsInMaintenance(nodeName string) bool {
	if config == nil {
		return false
	}
	return config.Nodes[nodeName].Maintenance
}

func (config *NodeMaintenanceConfig) ResolveJoinPermission(nodeName string) (string, bool) {
	if config == nil {
		return "", false
	}
	if entry, ok := config.Nodes[nodeName]; ok && entry.JoinPermission != nil {
		if *entry.JoinPermission == "" {
			return "", false
		}
		return *entry.JoinPermission, true
	}
	if config.JoinPermission == nil || *config.JoinPermission == "" {
		return "", false
	}
	return *config.JoinPermission, true
}

type NodeMaintenanceConfigAPI struct {
	client virtualconfigv1grpc.VirtualConfigServiceClient
}

func NewNodeMaintenanceConfigAPI(conn grpc.ClientConnInterface) *NodeMaintenanceConfigAPI {
	return &NodeMaintenanceConfigAPI{
		client: virtualconfigv1grpc.NewVirtualConfigServiceClient(conn),
	}
}

// Get returns nil config when no maintenance config has been created.
func (api *NodeMaintenanceConfigAPI) Get(ctx context.Context) (*NodeMaintenanceConfig, error) {
	getCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	response, err := api.client.GetByName(getCtx, &virtualconfigv1.GetByNameRequest{
		Name: NodeMaintenanceConfigName,
	})
	if status.Code(err) == codes.NotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get node maintenance config: %w", err)
	}
	if response.GetConfig() == nil {
		return nil, nil
	}

	config := DefaultNodeMaintenanceConfig()
	if err := json.Unmarshal([]byte(response.GetConfig().GetConfig()), &config); err != nil {
		return nil, fmt.Errorf("decode node maintenance config: %w", err)
	}
	return &config, nil
}
