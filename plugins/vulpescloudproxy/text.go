package vulpescloudproxy

import (
	"strings"

	"github.com/vulpesmc/gate-proxy/util/mini"
	"go.minekube.com/common/minecraft/component"
)

func parseMiniMessage(message string) component.Component {
	var parts []component.Component
	styles := []component.Style{{}}
	tags := []string{}
	appendText := func(text string) {
		if text != "" {
			parts = append(parts, &component.Text{Content: text, S: styles[len(styles)-1]})
		}
	}

	for len(message) > 0 {
		start := strings.IndexByte(message, '<')
		if start < 0 {
			appendText(message)
			break
		}
		appendText(message[:start])
		message = message[start:]
		end := strings.IndexByte(message, '>')
		if end < 0 {
			appendText(message)
			break
		}
		tag := message[1:end]
		message = message[end+1:]

		if strings.HasPrefix(tag, "/") {
			closingTag := strings.TrimPrefix(tag, "/")
			tagIndex := -1
			for i := len(tags) - 1; i >= 0; i-- {
				if tags[i] == closingTag {
					tagIndex = i
					break
				}
			}
			if tagIndex < 0 {
				appendText("<" + tag + ">")
				continue
			}
			tags = tags[:tagIndex]
			styles = styles[:tagIndex+1]
			continue
		}

		style, ok := parseStyle(tag, styles[len(styles)-1])
		if !ok {
			appendText("<" + tag + ">")
			continue
		}
		styles = append(styles, style)
		if strings.HasPrefix(tag, "color:") {
			tags = append(tags, "color")
		} else {
			tags = append(tags, tag)
		}
	}
	return &component.Text{Extra: parts}
}

func parseStyle(tag string, current component.Style) (component.Style, bool) {
	style := current
	switch strings.ToLower(tag) {
	case "bold", "b":
		style.Bold = component.True
	case "italic", "i":
		style.Italic = component.True
	case "underlined", "underline", "u":
		style.Underlined = component.True
	case "strikethrough", "st":
		style.Strikethrough = component.True
	case "obfuscated", "obf":
		style.Obfuscated = component.True
	case "reset":
		return component.Style{}, true
	default:
		colorName := tag
		if strings.HasPrefix(strings.ToLower(tag), "color:") {
			colorName = tag[len("color:"):]
		}
		colorName = strings.ToLower(colorName)
		if colorName == "grey" {
			colorName = "gray"
		}
		color, err := mini.ParseColor(colorName)
		if err != nil {
			return current, false
		}
		style.Color = color
	}
	return style, true
}
