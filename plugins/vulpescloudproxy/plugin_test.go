package vulpescloudproxy

import (
	"encoding/json"
	"strings"
	"testing"

	"go.minekube.com/common/minecraft/component"
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

func TestParseMiniMessageDefaultTags(t *testing.T) {
	parsed := parseMiniMessage("<grey>[<aqua>VulpesCloud</aqua>]</grey>").(*component.Text)
	if len(parsed.Extra) != 3 {
		t.Fatalf("expected 3 formatted text components, got %d", len(parsed.Extra))
	}
	for _, child := range parsed.Extra {
		text, ok := child.(*component.Text)
		if !ok {
			t.Fatalf("expected text component, got %T", child)
		}
		if strings.Contains(text.Content, "<") || strings.Contains(text.Content, ">") {
			t.Fatalf("expected recognized tags to be removed, got %q", text.Content)
		}
	}
}
