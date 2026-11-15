package minimessage

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Hoppou-Hangout/micromessage"
	"github.com/google/uuid"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/key"
)

func Deserialize(input string) (*component.Text, error) {
	resolver := &extendedTagResolver{
		input:      input,
		nextMarker: 0xf0000,
		components: make(map[rune]component.Component),
	}
	parsed, err := micromessage.Deserialize(
		input,
		micromessage.WithTagResolver(resolver),
	)
	if err != nil {
		return nil, err
	}
	replaced := resolver.replaceMarkers(parsed)
	root, ok := replaced.(*component.Text)
	if !ok {
		return &component.Text{Extra: []component.Component{replaced}}, nil
	}
	return root, nil
}

type extendedTagResolver struct {
	input      string
	nextMarker rune
	components map[rune]component.Component
}

func (r *extendedTagResolver) ResolveTag(
	name string,
	args *micromessage.ArgumentQueue,
) (micromessage.Tag, bool) {
	switch strings.ToLower(name) {
	case "key":
		value, ok := args.Pop()
		if !ok {
			return micromessage.Tag{}, false
		}
		return r.componentTag(&component.Keybind{Key: value})
	case "selector", "sel":
		value, ok := args.Pop()
		if !ok {
			return micromessage.Tag{}, false
		}
		selector := &component.Selector{Pattern: value}
		if args.HasNext() {
			separator, err := Deserialize(args.PopOr(""))
			if err != nil {
				return micromessage.Tag{}, false
			}
			selector.Separator = separator
		}
		return r.componentTag(selector)
	case "score":
		name, ok := args.Pop()
		if !ok {
			return micromessage.Tag{}, false
		}
		objective, ok := args.Pop()
		if !ok {
			return micromessage.Tag{}, false
		}
		value, _ := args.Pop()
		return r.componentTag(&component.Score{
			Name:      name,
			Objective: objective,
			Value:     value,
		})
	case "nbt", "data":
		return r.resolveNBT(args)
	case "sprite":
		return r.resolveSprite(args)
	case "head":
		return r.resolveHead(args)
	case "pride":
		return resolvePride(args)
	default:
		return micromessage.Tag{}, false
	}
}

func (r *extendedTagResolver) componentTag(value component.Component) (micromessage.Tag, bool) {
	for r.nextMarker <= 0xffffd {
		marker := r.nextMarker
		r.nextMarker++
		if strings.ContainsRune(r.input, marker) {
			continue
		}
		r.components[marker] = value
		return micromessage.Text(string(marker)), true
	}
	return micromessage.Tag{}, false
}

func (r *extendedTagResolver) replaceMarkers(value component.Component) component.Component {
	if text, ok := value.(*component.Text); ok {
		if utf8.RuneCountInString(text.Content) == 1 {
			marker, _ := utf8.DecodeRuneInString(text.Content)
			if replacement, ok := r.components[marker]; ok {
				style := *replacement.Style()
				*replacement.Style() = inheritStyle(text.S, style)
				return replacement
			}
		}
	}
	children := value.Children()
	for i, child := range children {
		children[i] = r.replaceMarkers(child)
	}
	value.SetChildren(children)
	return value
}

func inheritStyle(inherited, own component.Style) component.Style {
	result := inherited
	if own.Obfuscated != component.NotSet {
		result.Obfuscated = own.Obfuscated
	}
	if own.Bold != component.NotSet {
		result.Bold = own.Bold
	}
	if own.Strikethrough != component.NotSet {
		result.Strikethrough = own.Strikethrough
	}
	if own.Underlined != component.NotSet {
		result.Underlined = own.Underlined
	}
	if own.Italic != component.NotSet {
		result.Italic = own.Italic
	}
	if own.Font != nil {
		result.Font = own.Font
	}
	if own.Color != nil {
		result.Color = own.Color
	}
	if own.ClickEvent != nil {
		result.ClickEvent = own.ClickEvent
	}
	if own.HoverEvent != nil {
		result.HoverEvent = own.HoverEvent
	}
	if own.Insertion != nil {
		result.Insertion = own.Insertion
	}
	if own.ShadowColor != nil {
		result.ShadowColor = own.ShadowColor
	}
	return result
}

func (r *extendedTagResolver) resolveNBT(args *micromessage.ArgumentQueue) (micromessage.Tag, bool) {
	kind, ok := args.Pop()
	if !ok {
		return micromessage.Tag{}, false
	}
	id, ok := args.Pop()
	if !ok {
		return micromessage.Tag{}, false
	}
	path, ok := args.Pop()
	if !ok {
		return micromessage.Tag{}, false
	}

	nbt := component.NBT{Path: path}
	var result component.Component
	switch strings.ToLower(kind) {
	case "block":
		result = &component.BlockNBT{NBT: nbt, Block: id}
	case "entity":
		result = &component.EntityNBT{NBT: nbt, Entity: id}
	case "storage":
		storage, err := key.ParseValid(id)
		if err != nil {
			return micromessage.Tag{}, false
		}
		result = &component.StorageNBT{NBT: nbt, Storage: storage}
	default:
		return micromessage.Tag{}, false
	}

	remaining := args.Rest()
	if len(remaining) > 0 {
		if strings.EqualFold(remaining[0], "interpret") {
			nbt.Interpret = true
		} else {
			separator, err := Deserialize(remaining[0])
			if err != nil {
				return micromessage.Tag{}, false
			}
			nbt.Separator = separator
			if len(remaining) > 1 {
				if len(remaining) != 2 || !strings.EqualFold(remaining[1], "interpret") {
					return micromessage.Tag{}, false
				}
				nbt.Interpret = true
			}
		}
	}
	switch value := result.(type) {
	case *component.BlockNBT:
		value.NBT = nbt
	case *component.EntityNBT:
		value.NBT = nbt
	case *component.StorageNBT:
		value.NBT = nbt
	}
	return r.componentTag(result)
}

func (r *extendedTagResolver) resolveSprite(args *micromessage.ArgumentQueue) (micromessage.Tag, bool) {
	parts := args.Rest()
	if len(parts) == 1 {
		sprite, err := key.ParseValid(parts[0])
		if err != nil {
			return micromessage.Tag{}, false
		}
		return r.componentTag(component.AtlasSprite(component.DefaultSpriteAtlas, sprite))
	}
	if len(parts) != 2 {
		return micromessage.Tag{}, false
	}
	atlas, err := key.ParseValid(parts[0])
	if err != nil {
		return micromessage.Tag{}, false
	}
	sprite, err := key.ParseValid(parts[1])
	if err != nil {
		return micromessage.Tag{}, false
	}
	return r.componentTag(component.AtlasSprite(atlas, sprite))
}

func (r *extendedTagResolver) resolveHead(args *micromessage.ArgumentQueue) (micromessage.Tag, bool) {
	value, ok := args.Pop()
	if !ok {
		return micromessage.Tag{}, false
	}
	hat := true
	if args.HasNext() {
		hatValue, ok := args.Pop()
		if !ok || (hatValue != "true" && hatValue != "false") {
			return micromessage.Tag{}, false
		}
		hat = hatValue == "true"
	}

	profile := &component.PlayerProfile{}
	if id, err := uuid.Parse(value); err == nil {
		profile.Id = id
	} else if texture, err := key.ParseValid(value); err == nil && strings.ContainsAny(value, ":/") {
		profile.Texture = texture
	} else {
		profile.Name = value
	}
	return r.componentTag(component.PlayerHead(profile, hat))
}

func resolvePride(args *micromessage.ArgumentQueue) (micromessage.Tag, bool) {
	flag, phase := "pride", 0.0
	if args.HasNext() {
		value, _ := args.Pop()
		if _, ok := prideFlags[strings.ToLower(value)]; ok {
			flag = strings.ToLower(value)
		} else if value != "" {
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil || parsed < -1 || parsed > 1 {
				return micromessage.Tag{}, false
			}
			phase = parsed
		}
	}
	colors := append([]int(nil), prideFlags[flag]...)
	return micromessage.ModifyingTagValue(&prideGradient{
		colors: colors,
		phase:  phase,
	}), true
}

type prideGradient struct {
	colors  []int
	phase   float64
	size    int
	index   int
	reverse bool
}

func (g *prideGradient) Visit(node *micromessage.Node) {
	if node.Kind == micromessage.KindText {
		g.size += utf8.RuneCountInString(node.Text)
	}
}

func (g *prideGradient) PostVisit() {
	if g.phase < 0 {
		g.phase = 1 + g.phase
		g.reverse = true
	}
}

func (g *prideGradient) Apply(current component.Component, _ int) component.Component {
	if g.reverse {
		for i, j := 0, len(g.colors)-1; i < j; i, j = i+1, j-1 {
			g.colors[i], g.colors[j] = g.colors[j], g.colors[i]
		}
		g.reverse = false
	}
	return g.applyComponent(current)
}

func (g *prideGradient) applyComponent(current component.Component) component.Component {
	text, ok := current.(*component.Text)
	if !ok {
		if children, ok := current.(interface {
			Children() []component.Component
			SetChildren([]component.Component)
		}); ok {
			original := children.Children()
			updated := make([]component.Component, 0, len(original))
			for _, child := range original {
				updated = append(updated, g.applyComponent(child))
			}
			children.SetChildren(updated)
		}
		return current
	}

	parts := make([]component.Component, 0, utf8.RuneCountInString(text.Content)+len(text.Extra))
	for _, char := range text.Content {
		style := text.S
		style.Color = g.colorAt(g.index)
		g.index++
		parts = append(parts, &component.Text{Content: string(char), S: style})
	}
	for _, child := range text.Extra {
		parts = append(parts, g.applyComponent(child))
	}
	return &component.Text{S: text.S, Extra: parts}
}

func (g *prideGradient) colorAt(index int) *color.RGB {
	if g.size <= 1 {
		return color.HexInt(g.colors[0])
	}
	multiplier := float64(len(g.colors)-1) / float64(g.size-1)
	position := float64(index)*multiplier + g.phase*float64(len(g.colors)-1)
	lowUnclamped := int(math.Floor(position))
	low := positiveMod(lowUnclamped, len(g.colors))
	high := positiveMod(int(math.Ceil(position)), len(g.colors))
	amount := position - float64(lowUnclamped)
	start, end := g.colors[low], g.colors[high]
	lerp := func(shift uint) int {
		a := float64((start >> shift) & 0xff)
		b := float64((end >> shift) & 0xff)
		return int(math.Round(a + (b-a)*amount))
	}
	return color.HexInt(lerp(16)<<16 | lerp(8)<<8 | lerp(0))
}

func positiveMod(value, modulus int) int {
	return (value%modulus + modulus) % modulus
}

// Palette values follow Kyori Adventure's standard MiniMessage pride tag.
var prideFlags = map[string][]int{
	"pride":       {0xe50000, 0xff8d00, 0xffee00, 0x028121, 0x004cff, 0x770088},
	"progress":    {0xffffff, 0xffafc7, 0x73d7ee, 0x613915, 0x000000, 0xe50000, 0xff8d00, 0xffee00, 0x028121, 0x004cff, 0x770088},
	"trans":       {0x5bcffb, 0xf5abb9, 0xffffff, 0xf5abb9, 0x5bcffb},
	"bi":          {0xd60270, 0x9b4f96, 0x0038a8},
	"pan":         {0xff1c8d, 0xffd700, 0x1ab3ff},
	"nb":          {0xfcf431, 0xfcfcfc, 0x9d59d2, 0x282828},
	"lesbian":     {0xd62800, 0xff9b56, 0xffffff, 0xd462a6, 0xa40062},
	"ace":         {0x000000, 0xa4a4a4, 0xffffff, 0x810081},
	"agender":     {0x000000, 0xbababa, 0xffffff, 0xbaf484, 0xffffff, 0xbababa, 0x000000},
	"demisexual":  {0x000000, 0xffffff, 0x6e0071, 0xd3d3d3},
	"genderqueer": {0xb57fdd, 0xffffff, 0x49821e},
	"genderfluid": {0xfe76a2, 0xffffff, 0xbf12d7, 0x000000, 0x303cbe},
	"intersex":    {0xffd800, 0x7902aa, 0xffd800},
	"aro":         {0x3ba740, 0xa8d47a, 0xffffff, 0xababab, 0x000000},
	"baker":       {0xcd66ff, 0xff6599, 0xfe0000, 0xfe9900, 0xffff01, 0x009900, 0x0099cb, 0x350099, 0x990099},
	"philly":      {0x000000, 0x784f17, 0xfe0000, 0xfd8c00, 0xffe500, 0x119f0b, 0x0644b3, 0xc22edc},
	"queer":       {0x000000, 0x9ad9ea, 0x00a3e8, 0xb5e51d, 0xffffff, 0xffc90d, 0xfc6667, 0xfeaec9, 0x000000},
	"gay":         {0x078e70, 0x26ceaa, 0x98e8c1, 0xffffff, 0x7bade2, 0x5049cb, 0x3d1a78},
	"bigender":    {0xc479a0, 0xeca6cb, 0xd5c7e8, 0xffffff, 0xd5c7e8, 0x9ac7e8, 0x6c83cf},
	"demigender":  {0x7f7f7f, 0xc3c3c3, 0xfbff74, 0xffffff, 0xfbff74, 0xc3c3c3, 0x7f7f7f},
	"femboy":      {0xf6a8b8, 0xf6c4d2, 0xffffff, 0xc8dff0, 0x7faee0},
	"intersex_inclusive": {
		0xffffff, 0xffafc7, 0x73d7ee, 0x613915, 0x000000, 0xe50000, 0xff8d00,
		0xffee00, 0x028121, 0x004cff, 0x770088, 0xffd800, 0x7902aa, 0xffd800,
	},
	"intersex-inclusive": {
		0xffffff, 0xffafc7, 0x73d7ee, 0x613915, 0x000000, 0xe50000, 0xff8d00,
		0xffee00, 0x028121, 0x004cff, 0x770088, 0xffd800, 0x7902aa, 0xffd800,
	},
}
