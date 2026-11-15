package vulpescloudproxy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"github.com/vulpesmc/gate-proxy/plugins/vulpescloud"
	"github.com/vulpesmc/gate-proxy/plugins/vulpescloud/client"
	"go.minekube.com/brigodier"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

const (
	PluginName        = "vulpescloud-proxy"
	PermissionCommand = "vulpescloud.proxy.commands"
	PermissionReload  = "vulpescloud.proxy.commands.reload"
)

var Plugin = proxy.Plugin{
	Name: PluginName,
	Init: func(ctx context.Context, p *proxy.Proxy) error {
		log := logr.FromContextOrDiscard(ctx).WithName(PluginName)
		log.Info("Initializing")

		conn, err := client.Connect()
		if err != nil {
			return fmt.Errorf("connect to VulpesCloud: %w", err)
		}

		configState, err := newProxyModuleConfigState(newProxyModuleConfigAPI(conn), ctx)
		if err != nil {
			_ = conn.Close()
			return err
		}

		service, err := vulpescloud.NewServiceAPI(conn, os.Getenv("serviceUUID")).GetLocalService(ctx)
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("get local proxy service: %w", err)
		}
		if service.GetTask() == nil {
			_ = conn.Close()
			return errors.New("local proxy service has no task")
		}
		serviceName := fmt.Sprintf("%s-%d", service.GetTask().GetName(), service.GetOrderedId())

		event.Subscribe(p.Event(), 0, func(e *proxy.PingEvent) {
			handlePing(e, configState.get(), serviceName)
		})
		event.Subscribe(p.Event(), 0, func(e *proxy.PostLoginEvent) {
			handlePostLogin(p, e, configState, service.GetTask().GetMaxPlayers())
		})
		event.Subscribe(p.Event(), 0, func(e *proxy.ShutdownEvent) {
			log.Info("Stopping")
			if err := conn.Close(); err != nil {
				log.Error(err, "Unable to close VulpesCloud connection")
			}
		})
		p.Command().Register(vulpesCloudProxyCommand(ctx, configState, log))
		return nil
	},
}

func handlePing(event *proxy.PingEvent, config ProxyModuleConfig, serviceName string) {
	maintenance := config.Maintenance
	motd := config.MOTD
	var firstLine, secondLine string
	switch {
	case maintenance.Active:
		firstLine, secondLine = maintenance.FirstLine, maintenance.SecondLine
	case motd.Enabled:
		firstLine, secondLine = motd.FirstLine, motd.SecondLine
	default:
		return
	}

	pingResponse := event.Ping()
	online, max := 0, 0
	if pingResponse.Players != nil {
		online = pingResponse.Players.Online
		max = pingResponse.Players.Max
	}
	replace := strings.NewReplacer(
		"%proxy%", serviceName,
		"%playerCount%", fmt.Sprint(online),
		"%playerMax%", fmt.Sprint(max),
	)
	pingResponse.Description = parseMiniMessage(replace.Replace(firstLine) + "\n" + replace.Replace(secondLine))
	event.SetPing(pingResponse)
}

func handlePostLogin(
	p *proxy.Proxy,
	event *proxy.PostLoginEvent,
	configState *proxyModuleConfigState,
	maxPlayers uint32,
) {
	config := configState.get()
	player := event.Player()
	if config.Maintenance.Active {
		if !player.HasPermission(config.Maintenance.BypassPermission) {
			player.Disconnect(parseMiniMessage(config.Maintenance.KickMessage))
		}
		return
	}
	if maxPlayers > 0 && p.PlayerCount() >= int(maxPlayers) {
		player.Disconnect(parseMiniMessage(config.FullKickMessage))
	}
}

func vulpesCloudProxyCommand(
	ctx context.Context,
	configState *proxyModuleConfigState,
	log logr.Logger,
) brigodier.LiteralNodeBuilder {
	return brigodier.Literal("vulpescloud-proxy").
		Requires(command.Requires(func(c *command.RequiresContext) bool {
			return c.Source != nil && c.Source.HasPermission(PermissionCommand)
		})).
		Then(brigodier.Literal("reload").
			Requires(command.Requires(func(c *command.RequiresContext) bool {
				return c.Source != nil && c.Source.HasPermission(PermissionReload)
			})).
			Executes(command.Command(func(c *command.Context) error {
				if err := c.SendMessage(parseMiniMessage(
					"<grey>[<aqua>VulpesCloud-Proxy-Module</aqua>]</grey> Pulling new configuration!",
				)); err != nil {
					return err
				}

				reloadCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				if err := configState.reload(reloadCtx); err != nil {
					log.Error(err, "Unable to reload proxy module config")
					return err
				}
				return c.SendMessage(parseMiniMessage(
					"<grey>[<aqua>VulpesCloud-Proxy-Module</aqua>]</grey> <gray>Successfully pulled new configuration!</gray>",
				))
			})))
}
