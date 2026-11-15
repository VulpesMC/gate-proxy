package vulpescloud

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	eventspb "buf.build/gen/go/vulpescloud/protospecs/grpc/go/vulpescloud/events/v1/eventsv1grpc"
	playersv1grpc "buf.build/gen/go/vulpescloud/protospecs/grpc/go/vulpescloud/players/v1/playersv1grpc"
	eventsv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/events/v1"
	nodev1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/node/v1"
	playersv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/players/v1"
	servicesv1 "buf.build/gen/go/vulpescloud/protospecs/protocolbuffers/go/vulpescloud/services/v1"
	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/edition/java/title"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	PlayerJoinEventID         = "vulpescloud.events.v1.PlayerJoinEvent"
	PlayerQuitEventID         = "vulpescloud.events.v1.PlayerQuitEvent"
	PlayerSwitchServerEventID = "vulpescloud.events.v1.PlayerSwitchServerEvent"
	PlayerActionEventID       = "vulpescloud.events.v1.PlayerActionEvent"
)

// PlayerEventHandler publishes proxy player events and applies remote player actions.
type PlayerEventHandler struct {
	proxy       *proxy.Proxy
	services    *ServiceAPI
	config      *ConnectorConfigAPI
	maintenance *NodeMaintenanceConfigAPI
	players     playersv1grpc.PlayersServiceClient
	events      eventspb.EventServiceClient
	eventStream *EventManager
	ctx         context.Context
	log         logr.Logger
}

// NewPlayerEventHandler creates handlers for Gate lifecycle events and Cloud player actions.
func NewPlayerEventHandler(
	ctx context.Context,
	p *proxy.Proxy,
	services *ServiceAPI,
	config *ConnectorConfigAPI,
	maintenance *NodeMaintenanceConfigAPI,
	conn grpc.ClientConnInterface,
	events eventspb.EventServiceClient,
	eventStream *EventManager,
	log logr.Logger,
) *PlayerEventHandler {
	return &PlayerEventHandler{
		proxy:       p,
		services:    services,
		config:      config,
		maintenance: maintenance,
		players:     playersv1grpc.NewPlayersServiceClient(conn),
		events:      events,
		eventStream: eventStream,
		ctx:         ctx,
		log:         log,
	}
}

// Register subscribes the proxy's player lifecycle events and Cloud player actions.
func (h *PlayerEventHandler) Register(manager event.Manager) error {
	if _, err := h.eventStream.EventHandler(PlayerActionEventID, h.handlePlayerAction); err != nil {
		return err
	}

	event.Subscribe(manager, 0, h.handleLogin)
	event.Subscribe(manager, 0, h.handleDisconnect)
	event.Subscribe(manager, 0, h.handleServerConnected)
	event.Subscribe(manager, 0, h.handleChooseInitialServer)
	event.Subscribe(manager, 0, h.handleKickedFromServer)
	event.Subscribe(manager, 0, h.handleServerPreConnect)
	return nil
}

func (h *PlayerEventHandler) handleLogin(e *proxy.LoginEvent) {
	player := e.Player()
	service, err := h.services.GetLocalService(h.ctx)
	if err != nil {
		h.log.Error(err, "Unable to get local service for player login")
		return
	}

	if err := h.services.UpdatePlayerCount(h.ctx, service, h.playerCount(player, true)); err != nil {
		h.log.Error(err, "Unable to update player count after login")
	}

	if err := publishCloudEvent(h.ctx, h.events, PlayerJoinEventID, &eventsv1.PlayerJoinEvent{
		Player: &playersv1.OnlinePlayer{
			Name:             player.Username(),
			Uuid:             player.ID().String(),
			ProxyServiceName: serviceName(service),
		},
		Timestamp: time.Now().UnixMilli(),
	}); err != nil {
		h.log.Error(err, "Unable to publish player join event")
	}
}

func (h *PlayerEventHandler) handleDisconnect(e *proxy.DisconnectEvent) {
	player := e.Player()
	service, err := h.services.GetLocalService(h.ctx)
	if err != nil {
		h.log.Error(err, "Unable to get local service for player disconnect")
		return
	}

	if err := h.services.UpdatePlayerCount(h.ctx, service, h.playerCount(player, false)); err != nil {
		h.log.Error(err, "Unable to update player count after disconnect")
	}

	offlinePlayer, err := h.getOfflinePlayer(player.ID().String())
	if err != nil {
		h.log.Error(err, "Unable to find registered player for disconnect", "uuid", player.ID().String())
		return
	}

	if err := publishCloudEvent(h.ctx, h.events, PlayerQuitEventID, &eventsv1.PlayerQuitEvent{
		Player: &playersv1.OfflinePlayer{
			Name:      player.Username(),
			Uuid:      player.ID().String(),
			FirstSeen: offlinePlayer.GetFirstSeen(),
			LastSeen:  uint64(time.Now().UnixMilli()),
		},
		LastProxyName: serviceName(service),
		Timestamp:     time.Now().UnixMilli(),
	}); err != nil {
		h.log.Error(err, "Unable to publish player quit event")
	}
}

func (h *PlayerEventHandler) handleServerConnected(e *proxy.ServerConnectedEvent) {
	player := e.Player()
	serverName := e.Server().ServerInfo().Name()

	service, err := h.services.GetLocalService(h.ctx)
	if err != nil {
		h.log.Error(err, "Unable to get local service for server switch")
		return
	}

	oldServer := ""
	if previous := e.PreviousServer(); previous != nil {
		oldServer = previous.ServerInfo().Name()
	}

	if err := publishCloudEvent(h.ctx, h.events, PlayerSwitchServerEventID, &eventsv1.PlayerSwitchServerEvent{
		Player: &playersv1.OnlinePlayer{
			Name:              player.Username(),
			Uuid:              player.ID().String(),
			ProxyServiceName:  serviceName(service),
			ServerServiceName: serverName,
		},
		OldServer: oldServer,
		NewServer: serverName,
		Timestamp: time.Now().UnixMilli(),
	}); err != nil {
		h.log.Error(err, "Unable to publish player server switch event")
	}
}

func (h *PlayerEventHandler) handleChooseInitialServer(e *proxy.PlayerChooseInitialServerEvent) {
	server, err := h.bestFallbackServer()
	if err != nil {
		h.log.Error(err, "Unable to find fallback server for initial connection")
		return
	}
	if server != nil {
		e.SetInitialServer(server)
	}
}

func (h *PlayerEventHandler) handleServerPreConnect(e *proxy.ServerPreConnectEvent) {
	originalServer := e.OriginalServer()
	if originalServer == nil {
		return
	}

	services, err := h.services.GetAllServices(h.ctx)
	if err != nil {
		h.log.Error(err, "Unable to find service for pre-connect maintenance check")
		return
	}
	var target *servicesv1.ServiceDefinition
	for _, service := range services {
		if serviceName(service) == originalServer.ServerInfo().Name() {
			target = service
			break
		}
	}
	if target == nil {
		return
	}

	config, err := h.maintenance.Get(h.ctx)
	if err != nil {
		h.log.Error(err, "Unable to load node maintenance config")
		return
	}
	if !config.IsInMaintenance(target.GetNode()) {
		return
	}
	permission, required := config.ResolveJoinPermission(target.GetNode())
	if required && !e.Player().HasPermission(permission) {
		e.Deny()
	}
}

func (h *PlayerEventHandler) handleKickedFromServer(e *proxy.KickedFromServerEvent) {
	server, err := h.bestFallbackServer()
	if err != nil {
		h.log.Error(err, "Unable to find fallback server after server kick")
		return
	}
	if server == nil {
		config, err := h.config.Get(h.ctx)
		if err != nil {
			h.log.Error(err, "Unable to load connector config for missing fallback server")
			e.SetResult(&proxy.DisconnectPlayerKickResult{
				Reason: &component.Text{Content: "There is no available server for you to connect to!"},
			})
			return
		}
		e.SetResult(&proxy.DisconnectPlayerKickResult{
			Reason: deserializeMiniMessage(config.DisconnectNoAvailableServerMessage),
		})
		return
	}

	if current := e.Player().CurrentServer(); current != nil &&
		current.Server().ServerInfo().Name() == server.ServerInfo().Name() {
		return
	}
	e.SetResult(&proxy.RedirectPlayerKickResult{Server: server})
}

func (h *PlayerEventHandler) bestFallbackServer() (proxy.RegisteredServer, error) {
	services, err := h.services.GetAllServices(h.ctx)
	if err != nil {
		return nil, err
	}

	candidates := make([]*servicesv1.ServiceDefinition, 0, len(services))
	for _, service := range services {
		task := service.GetTask()
		if task == nil ||
			task.GetServerSoftware().GetType() == nodev1.SoftwareType_SOFTWARE_TYPE_PROXY ||
			!task.GetFallback() ||
			service.GetState() != servicesv1.ServiceState_SERVICE_STATE_RUNNING {
			continue
		}
		candidates = append(candidates, service)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].GetPlayerCount() < candidates[j].GetPlayerCount()
	})

	for _, service := range candidates {
		if server := h.proxy.Server(serviceName(service)); server != nil {
			return server, nil
		}
	}
	return nil, nil
}

func (h *PlayerEventHandler) handlePlayerAction(e *eventsv1.Event, log logr.Logger) {
	data := e.GetData()
	if data == nil {
		log.Error(nil, "Player action event has no data")
		return
	}

	action := new(eventsv1.PlayerActionEvent)
	if err := data.UnmarshalTo(action); err != nil {
		log.Error(err, "Unable to decode player action event")
		return
	}
	if action.GetPlayer() == nil {
		log.Error(errors.New("player action has no player"), "Unable to handle player action")
		return
	}

	player := h.findPlayer(action.GetPlayer())
	if player == nil {
		return
	}
	values := action.GetData()
	message := func(key string) component.Component {
		return deserializeMiniMessage(values[key])
	}

	switch action.GetAction() {
	case eventsv1.PlayerActions_KICK:
		player.Disconnect(message("reason"))
	case eventsv1.PlayerActions_MESSAGE:
		if err := player.SendMessage(message("message")); err != nil {
			log.Error(err, "Unable to send player message", "player", player.Username())
		}
	case eventsv1.PlayerActions_TITLE:
		if err := title.ShowTitle(player, &title.Options{
			Title:    message("title"),
			Subtitle: message("subtitle"),
		}); err != nil {
			log.Error(err, "Unable to show player title", "player", player.Username())
		}
	case eventsv1.PlayerActions_ACTION_BAR:
		if err := player.SendActionBar(message("message")); err != nil {
			log.Error(err, "Unable to send player action bar", "player", player.Username())
		}
	case eventsv1.PlayerActions_CONNECT:
		targetName := values["targetServer"]
		server := h.proxy.Server(targetName)
		if server == nil {
			log.Error(fmt.Errorf("server %q is not registered", targetName), "Unable to connect player")
			return
		}
		player.CreateConnectionRequest(server).ConnectWithIndication(player.Context())
	case eventsv1.PlayerActions_UNSPECIFIED:
		log.Error(errors.New("player action is unspecified"), "Unable to handle player action")
	default:
		log.Error(fmt.Errorf("unrecognized player action %d", action.GetAction()), "Unable to handle player action")
	}
}

func (h *PlayerEventHandler) findPlayer(target *playersv1.OnlinePlayer) proxy.Player {
	for _, player := range h.proxy.Players() {
		if strings.EqualFold(player.ID().String(), target.GetUuid()) ||
			strings.EqualFold(player.Username(), target.GetName()) {
			return player
		}
	}
	return nil
}

func (h *PlayerEventHandler) playerCount(player proxy.Player, joining bool) uint32 {
	count := len(h.proxy.Players())
	isOnline := h.proxy.Player(player.ID()) != nil
	if joining && !isOnline {
		count++
	} else if !joining && isOnline {
		count--
	}
	return uint32(count)
}

func (h *PlayerEventHandler) getOfflinePlayer(uuid string) (*playersv1.OfflinePlayer, error) {
	ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
	defer cancel()

	response, err := h.players.GetAllOfflinePlayers(ctx, &playersv1.GetOfflinePlayersRequest{})
	if err != nil {
		return nil, fmt.Errorf("get offline players: %w", err)
	}
	for _, player := range response.GetOfflinePlayers() {
		if strings.EqualFold(player.GetUuid(), uuid) {
			return player, nil
		}
	}
	return nil, fmt.Errorf("player %q not found", uuid)
}

func publishCloudEvent(
	ctx context.Context,
	eventClient eventspb.EventServiceClient,
	eventID string,
	message proto.Message,
) error {
	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	data, err := anypb.New(message)
	if err != nil {
		return fmt.Errorf("encode event %q: %w", eventID, err)
	}
	_, err = eventClient.Publish(publishCtx, &eventsv1.PublishRequest{
		ForwardToOtherNodes: true,
		Event: &eventsv1.Event{
			Id:        eventID,
			Timestamp: time.Now().UnixMilli(),
			Data:      data,
		},
	})
	if err != nil {
		return fmt.Errorf("publish event %q: %w", eventID, err)
	}
	return nil
}
