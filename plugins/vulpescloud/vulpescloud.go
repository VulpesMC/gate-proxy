package vulpescloud

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/minekube/gate-plugin-template/plugins/vulpescloud/client"
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

		_, err := client.Connect()
		if err != nil {
			log.Error(err, "Unable to Connect to Node!")
		}

		return nil
	},
}
