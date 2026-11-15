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
	"github.com/vulpesmc/gate-proxy/util/minimessage"
	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/component"
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
			if err := handlePing(e, configState.get(), serviceName); err != nil {
				log.Error(err, "Unable to apply proxy MOTD")
			}
		})
		event.Subscribe(p.Event(), 0, func(e *proxy.PostLoginEvent) {
			if err := handlePostLogin(p, e, configState, service.GetTask().GetMaxPlayers()); err != nil {
				log.Error(err, "Unable to apply proxy login policy", "player", e.Player().Username())
			}
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

func handlePing(event *proxy.PingEvent, config ProxyModuleConfig, serviceName string) error {
	maintenance := config.Maintenance
	motd := config.MOTD
	var firstLine, secondLine string
	switch {
	case maintenance.Active:
		firstLine, secondLine = maintenance.FirstLine, maintenance.SecondLine
	case motd.Enabled:
		firstLine, secondLine = motd.FirstLine, motd.SecondLine
	default:
		return nil
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
	description, err := minimessage.Deserialize(replace.Replace(firstLine) + "\n" + replace.Replace(secondLine))
	if err != nil {
		return fmt.Errorf("parse proxy MOTD: %w", err)
	}
	pingResponse.Description = description
	event.SetPing(pingResponse)
	return nil
}

func handlePostLogin(
	p *proxy.Proxy,
	event *proxy.PostLoginEvent,
	configState *proxyModuleConfigState,
	maxPlayers uint32,
) error {
	config := configState.get()
	player := event.Player()
	if config.Maintenance.Active {
		if !player.HasPermission(config.Maintenance.BypassPermission) {
			message, err := minimessage.Deserialize(config.Maintenance.KickMessage)
			if err != nil {
				player.Disconnect(&component.Text{Content: config.Maintenance.KickMessage})
				return fmt.Errorf("parse maintenance kick message: %w", err)
			}
			player.Disconnect(message)
		}
		return nil
	}
	if maxPlayers > 0 && p.PlayerCount() >= int(maxPlayers) {
		message, err := minimessage.Deserialize(config.FullKickMessage)
		if err != nil {
			player.Disconnect(&component.Text{Content: config.FullKickMessage})
			return fmt.Errorf("parse full-server kick message: %w", err)
		}
		player.Disconnect(message)
	}
	return nil
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
				pulling, err := minimessage.Deserialize(
					"<grey>[<aqua>VulpesCloud-Proxy-Module</aqua>]</grey> Pulling new configuration!",
				)
				if err != nil {
					return fmt.Errorf("parse reload feedback: %w", err)
				}
				if err := c.SendMessage(pulling); err != nil {
					return err
				}

				reloadCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				if err := configState.reload(reloadCtx); err != nil {
					log.Error(err, "Unable to reload proxy module config")
					return err
				}
				success, err := minimessage.Deserialize(
					"<grey>[<aqua>VulpesCloud-Proxy-Module</aqua>]</grey> <gray>Successfully pulled new configuration!</gray>",
				)
				if err != nil {
					return fmt.Errorf("parse reload feedback: %w", err)
				}
				return c.SendMessage(success)
			})))
}
