package vulpescloud

import (
	"testing"

	"go.minekube.com/common/minecraft/component"
)

func TestDeserializeMiniMessageStylesAndPlainText(t *testing.T) {
	parsed := deserializeMiniMessage("<bold>hello</bold> plain").(*component.Text)
	if len(parsed.Extra) != 2 {
		t.Fatalf("expected 2 text parts, got %d", len(parsed.Extra))
	}

	bold, ok := parsed.Extra[0].(*component.Text)
	if !ok || bold.S.Bold != component.True {
		t.Fatal("expected bold style on tagged text")
	}
	plain, ok := parsed.Extra[1].(*component.Text)
	if !ok || plain.Content != " plain" || plain.S.Bold == component.True {
		t.Fatal("expected unstyled plain text after closing tag")
	}
}

func TestDeserializeMiniMessageUnknownTagsRemainLiteral(t *testing.T) {
	parsed := deserializeMiniMessage("<unknown>text</unknown>").(*component.Text)
	if len(parsed.Extra) != 3 {
		t.Fatalf("expected unknown tags to remain literal, got %d parts", len(parsed.Extra))
	}
	if parsed.Extra[0].(*component.Text).Content != "<unknown>" ||
		parsed.Extra[2].(*component.Text).Content != "</unknown>" {
		t.Fatal("expected unknown tags to be preserved")
	}
}
