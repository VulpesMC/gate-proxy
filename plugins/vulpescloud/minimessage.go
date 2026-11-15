package vulpescloud

import (
	"strings"

	"github.com/vulpesmc/gate-proxy/util/mini"
	"go.minekube.com/common/minecraft/component"
)

func deserializeMiniMessage(message string) component.Component {
	var parts []component.Component
	styles := []component.Style{{}}
	var tags []string
	appendText := func(text string) {
		if text == "" {
			return
		}
		parts = append(parts, &component.Text{Content: text, S: styles[len(styles)-1]})
	}

	for len(message) > 0 {
		tagStart := strings.IndexByte(message, '<')
		if tagStart < 0 {
			appendText(message)
			break
		}
		appendText(message[:tagStart])
		message = message[tagStart:]

		tagEnd := strings.IndexByte(message, '>')
		if tagEnd < 0 {
			appendText(message)
			break
		}
		tag := message[1:tagEnd]
		message = message[tagEnd+1:]

		if strings.HasPrefix(tag, "/") {
			closingTag := strings.TrimPrefix(tag, "/")
			tagIndex := -1
			for i := len(tags) - 1; i >= 0; i-- {
				if tags[i] == closingTag {
					tagIndex = i
					break
				}
			}
			if tagIndex >= 0 {
				tags = tags[:tagIndex]
				styles = styles[:tagIndex+1]
			} else {
				appendText("<" + tag + ">")
			}
			continue
		}

		style, recognized := miniMessageStyle(tag, styles[len(styles)-1])
		if !recognized {
			appendText("<" + tag + ">")
			continue
		}
		styles = append(styles, style)
		tags = append(tags, miniMessageTagName(tag))
	}

	return &component.Text{Extra: parts}
}

func miniMessageTagName(tag string) string {
	if strings.HasPrefix(tag, "color:") {
		return "color"
	}
	return tag
}

func miniMessageStyle(tag string, current component.Style) (component.Style, bool) {
	style := current
	switch tag {
	case "bold":
		style.Bold = component.True
	case "italic":
		style.Italic = component.True
	case "underlined":
		style.Underlined = component.True
	case "strikethrough":
		style.Strikethrough = component.True
	case "obfuscated":
		style.Obfuscated = component.True
	case "reset":
		return component.Style{}, true
	default:
		colorName := tag
		if strings.HasPrefix(tag, "color:") {
			colorName = strings.TrimPrefix(tag, "color:")
		}
		color, err := mini.ParseColor(colorName)
		if err != nil {
			return current, false
		}
		style.Color = color
	}
	return style, true
}
