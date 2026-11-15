package mini

import (
	"testing"

	"go.minekube.com/common/minecraft/component"
)

func TestParseUsesFullMiniMessage(t *testing.T) {
	parsed := Parse("<gradient:#FC67FA:#F4C4F3>VulpesMC Network</gradient>")
	if len(parsed.Extra) == 0 {
		t.Fatal("expected gradient text components")
	}
	first, ok := parsed.Extra[0].(*component.Text)
	if !ok || first.S.Color == nil {
		t.Fatal("expected gradient colors from the full MiniMessage parser")
	}
}
