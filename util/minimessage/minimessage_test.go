package minimessage

import (
	"bytes"
	"strings"
	"testing"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec"
)

func marshalComponent(t *testing.T, value component.Component) string {
	t.Helper()
	var output bytes.Buffer
	if err := (&codec.Json{NoDownsampleColor: true}).Marshal(&output, value); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestDeserializeGradientAndTrailingPlainText(t *testing.T) {
	input := "<gradient:#FC67FA:#F4C4F3>VulpesMC Network</gradient> – Showcase Network"
	parsed, err := Deserialize(input)
	if err != nil {
		t.Fatal(err)
	}
	encoded := marshalComponent(t, parsed)
	if strings.Contains(encoded, "<gradient") {
		t.Fatalf("gradient tag was not parsed: %s", encoded)
	}
	if !strings.Contains(encoded, "Showcase Network") {
		t.Fatalf("plain trailing text was lost: %s", encoded)
	}
	if !strings.Contains(encoded, "#fc67fa") ||
		!strings.Contains(encoded, "#f4c4f3") {
		t.Fatalf("gradient endpoint colors were not applied: %s", encoded)
	}
}

func TestDeserializeSupportsEscapingAndQuotedArguments(t *testing.T) {
	parsed, err := Deserialize(`\<not-a-tag> <click:run_command:"/say hello:world">click me</click>`)
	if err != nil {
		t.Fatal(err)
	}
	encoded := marshalComponent(t, parsed)
	if !strings.Contains(encoded, "<not-a-tag>") ||
		!strings.Contains(encoded, "/say hello:world") {
		t.Fatalf("escaped or quoted content was not preserved: %s", encoded)
	}
}

func TestDeserializeExtendedStandardComponentTags(t *testing.T) {
	parsed, err := Deserialize(
		`<key:key.jump/> <selector:"@a[limit=1]"/> <score:Notch:objective/> ` +
			`<nbt:entity:"@s":Health:interpret/> <sprite:"minecraft:blocks":"minecraft:stone"/> ` +
			`<head:Notch/>`,
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded := marshalComponent(t, parsed)
	for _, expected := range []string{
		"keybind", "selector", "score", "nbt", "minecraft:stone", `"player":"Notch"`,
	} {
		if !strings.Contains(encoded, expected) {
			t.Errorf("expected parsed output to contain %q: %s", expected, encoded)
		}
	}
}

func TestDeserializeComponentTagsInheritAmbientStyle(t *testing.T) {
	parsed, err := Deserialize("<red><key:key.jump/></red>")
	if err != nil {
		t.Fatal(err)
	}
	encoded := marshalComponent(t, parsed)
	if !strings.Contains(encoded, `"color":"#ff5555"`) {
		t.Fatalf("component tag did not inherit ambient color: %s", encoded)
	}
}

func TestDeserializePrideTagAppliesColors(t *testing.T) {
	parsed, err := Deserialize("<pride>VulpesCloud</pride>")
	if err != nil {
		t.Fatal(err)
	}
	colors := make(map[string]bool)
	var visit func(component.Component)
	visit = func(current component.Component) {
		if text, ok := current.(*component.Text); ok {
			if text.Content != "" && text.S.Color != nil {
				colors[text.S.Color.Hex()] = true
			}
		}
		if children, ok := current.(interface{ Children() []component.Component }); ok {
			for _, child := range children.Children() {
				visit(child)
			}
		}
	}
	visit(parsed)
	if len(colors) < 2 {
		t.Fatalf("expected pride tag to color individual text, got %v", colors)
	}
}

func TestDeserializeUnknownTagsRemainLiteral(t *testing.T) {
	parsed, err := Deserialize("<unknown>text</unknown>")
	if err != nil {
		t.Fatal(err)
	}
	encoded := marshalComponent(t, parsed)
	if !strings.Contains(encoded, "<unknown>") ||
		!strings.Contains(encoded, "</unknown>") {
		t.Fatalf("expected unknown tags to remain literal: %s", encoded)
	}
}
