// Package mini provides compatibility helpers for MiniMessage and Minecraft text colors.
package mini

import (
	"fmt"
	"math"
	"strings"

	"github.com/vulpesmc/gate-proxy/util/minimessage"
	"go.minekube.com/common/minecraft/color"
	c "go.minekube.com/common/minecraft/component"
)

func Deserialize(input string) (*c.Text, error) {
	return minimessage.Deserialize(input)
}

func Parse(input string) *c.Text {
	parsed, err := Deserialize(input)
	if err != nil {
		panic(err)
	}
	return parsed
}

// ParseColor takes a string as input and returns a `color.Color` object. It checks if the input string
// starts with "#". If it does, it tries to parse it as a hex color. If it doesn't, it tries to find a
// named color that matches the input string.
func ParseColor(name string) (color.Color, error) {
	if strings.HasPrefix(name, "#") {
		newColor, err := color.Hex(name)
		if err != nil {
			return nil, err
		}
		return newColor, nil
	} else {
		return FromName(name)
	}
}

// FromName takes a string as input and returns a `color.Color` object.
// It iterates over the named colors and returns the one that matches the input string.
func FromName(name string) (color.Color, error) {
	col, ok := color.Names[name]
	if ok {
		return col, nil
	}
	for _, a := range color.Names {
		if strings.EqualFold(a.String(), name) {
			return a, nil
		}
	}
	return nil, fmt.Errorf("unknown color name: %s", name)
}

// Gradient takes a string, a style, and a variable number of colors as input and returns a `c.Text` object.
// It creates a gradient effect by interpolating between the input colors based on their position in the input string.
func Gradient(content string, style c.Style, colors ...color.RGB) *c.Text {
	var component []c.Component
	for id, i := range strings.Split(content, "") {
		t := float64(id) / float64(len(content))
		hex, _ := color.Hex(LerpColor(t, colors...).Hex())

		style.Color = hex
		component = append(component, &c.Text{
			Content: string(i),
			S:       style,
		})
	}

	return &c.Text{
		Extra: component,
	}
}

// LerpColor takes a float and a variable number of colors as input and returns a `color.Color` object.
// It interpolates between the input colors based on the input float.
func LerpColor(t float64, colors ...color.RGB) color.Color {
	t = math.Min(t, 1)

	if t == 1 {
		return &colors[len(colors)-1]
	}

	colorT := t * float64(len(colors)-1)
	newT := colorT - math.Floor(colorT)
	lastColor := colors[int(colorT)]
	nextColor := colors[int(colorT+1)]

	return &color.RGB{
		R: lerpInt(newT, nextColor.R, lastColor.R),
		G: lerpInt(newT, nextColor.G, lastColor.G),
		B: lerpInt(newT, nextColor.B, lastColor.B),
	}
}

// lerpInt takes three floats as input and returns a float. It performs linear interpolation between the
// second and third input floats based on the first input float.
func lerpInt(t float64, a float64, b float64) float64 {
	return a*t + b*(1-t)
}
