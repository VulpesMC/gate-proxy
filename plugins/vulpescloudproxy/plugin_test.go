package vulpescloudproxy

import (
	"encoding/json"
	"testing"

	"github.com/vulpesmc/gate-proxy/util/minimessage"
)

func TestDefaultProxyModuleConfigMatchesModuleDefaults(t *testing.T) {
	config := DefaultProxyModuleConfig()
	if !config.MOTD.Enabled {
		t.Fatal("expected MOTD to be enabled by default")
	}
	if config.Maintenance.Active {
		t.Fatal("expected maintenance to be disabled by default")
	}
	if config.Maintenance.BypassPermission != "vulpescloud.proxy.maintenance.bypass" {
		t.Fatalf("unexpected bypass permission %q", config.Maintenance.BypassPermission)
	}
	if config.FullKickMessage != "<red>This Server is currently full!</red>" {
		t.Fatalf("unexpected full kick message %q", config.FullKickMessage)
	}

	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode marshaled config: %v", err)
	}
	if _, ok := decoded["motd"]; !ok {
		t.Fatal("expected lower camel-case motd config key")
	}
}

func TestMiniMessageDefaultTags(t *testing.T) {
	parsed, err := minimessage.Deserialize("<grey>[<aqua>VulpesCloud</aqua>]</grey>")
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Extra) != 3 {
		t.Fatalf("expected 3 formatted text components, got %d", len(parsed.Extra))
	}
}
