package seal

import (
	"bytes"
	"fmt"
	"html"
	"math"
	"sort"
	"strings"
)

type Renderer struct{}
type random32 struct{ value uint32 }

func newRandom32(seed int64) *random32 {
	value := uint32(seed)
	if value == 0 {
		value = 123456789
	}
	return &random32{value: value}
}

func (random *random32) Float64() float64 {
	random.value ^= random.value << 13
	random.value ^= random.value >> 17
	random.value ^= random.value << 5
	return float64(random.value) / 4294967296.0
}

func (Renderer) SVG(config Config, watermark bool) ([]byte, error) {
	config.ApplyDefaults()
	if err := Validate(config); err != nil {
		return nil, err
	}

	layers := append([]Layer(nil), config.Layers...)
	sort.SliceStable(layers, func(i, j int) bool { return layers[i].ZIndex < layers[j].ZIndex })
	byID := make(map[string]Layer, len(layers))
	for _, layer := range layers {
		byID[layer.ID] = layer
	}
	main := byID["main-text"]
	inner := byID["inner-text"]
	bottom := byID["bottom-text"]

	var output bytes.Buffer
	fmt.Fprintf(&output, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 1000 1000">`, config.Canvas.ExportWidth, config.Canvas.ExportWidth)
	output.WriteString(`<defs>`)
	mainRX, mainRY := main.RadiusX, main.RadiusY
	if config.Shape == "ellipse" {
		mainRX = math.Min(mainRX, 355)
		mainRY = math.Min(mainRY, 280)
	}
	fmt.Fprintf(&output, `<path id="mainArc" d="M%.2f %.2f A%.2f %.2f 0 0 1 %.2f %.2f"/>`, 500-mainRX, 500+main.OffsetY, mainRX, mainRY, 500+mainRX, 500+main.OffsetY)
	fmt.Fprintf(&output, `<path id="innerArc" d="M%.2f %.2f A%.2f %.2f 0 0 1 %.2f %.2f"/>`, 500-inner.RadiusX, 500+inner.OffsetY, inner.RadiusX, inner.RadiusY, 500+inner.RadiusX, 500+inner.OffsetY)
	// Traverse the lower arc from right to left so glyphs stay upright.
	fmt.Fprintf(&output, `<path id="bottomArc" d="M%.2f %.2f A%.2f %.2f 0 0 1 %.2f %.2f"/>`, 500+bottom.RadiusX, 500+bottom.OffsetY, bottom.RadiusX, bottom.RadiusY, 500-bottom.RadiusX, 500+bottom.OffsetY)
	if config.Texture.Enabled {
		output.WriteString(`<mask id="wearMask" maskUnits="userSpaceOnUse"><rect width="1000" height="1000" fill="white"/>`)
		writeTexture(&output, config.Texture)
		output.WriteString(`</mask>`)
	}
	output.WriteString(`</defs>`)

	if !config.Canvas.Transparent {
		output.WriteString(`<rect width="1000" height="1000" fill="white"/>`)
	}
	writeBorder(&output, config, textureMask(config.Texture, "border"))
	output.WriteString(`<g fill="` + config.Color + `" stroke="none"` + textureMask(config.Texture, "text") + `>`)
	for _, layer := range layers {
		if !layer.Visible || (layer.Kind != "arcText" && layer.Kind != "text") {
			continue
		}
		writeTextLayer(&output, config.Shape, layer)
	}
	output.WriteString(`</g>`)
	output.WriteString(`<g fill="` + config.Color + `" stroke="none"` + textureMask(config.Texture, "center") + `>`)
	for _, layer := range layers {
		if !layer.Visible || (layer.Kind != "centerText" && layer.Kind != "centerImage") {
			continue
		}
		if layer.Kind == "centerText" {
			writeCenterLayer(&output, layer)
		}
		if layer.Kind == "centerImage" && strings.HasPrefix(layer.AssetData, "data:image/png;base64,") {
			fmt.Fprintf(&output, `<image href="%s" x="%.2f" y="%.2f" width="300" height="300" preserveAspectRatio="xMidYMid meet" transform="%s"/>`, layer.AssetData, 350+layer.OffsetX, 350+layer.OffsetY, layerTransform(layer, 500))
		}
	}
	output.WriteString(`</g>`)
	if watermark {
		output.WriteString(`<g fill="#d92626"><text x="500" y="490" text-anchor="middle" transform="rotate(-25 500 500)" font-size="28" font-weight="800" opacity="0.13">PREVIEW · 未解锁</text>`)
		fmt.Fprintf(&output, `<text x="500" y="535" text-anchor="middle" font-size="20" opacity="0.14">SESSION-%X</text></g>`, config.Texture.Seed)
	}
	output.WriteString(`</svg>`)
	return output.Bytes(), nil
}

func textureMask(texture TextureConfig, target string) string {
	if !texture.Enabled {
		return ""
	}
	for _, item := range texture.ApplyTo {
		if item == target {
			return ` mask="url(#wearMask)"`
		}
	}
	return ""
}

func writeBorder(output *bytes.Buffer, config Config, mask string) {
	fmt.Fprintf(output, `<g fill="%s" stroke="%s"%s>`, config.Color, config.Color, mask)
	if config.Shape == "square" {
		fmt.Fprintf(output, `<rect x="120" y="120" width="760" height="760" rx="14" fill="none" stroke-width="%.2f"/>`, config.Border.Width)
		if config.Border.DoubleLine {
			fmt.Fprintf(output, `<rect x="138" y="138" width="724" height="724" rx="10" fill="none" stroke-width="%.2f"/>`, math.Max(2, config.Border.Width/2))
		}
	} else if config.Shape == "ellipse" {
		fmt.Fprintf(output, `<ellipse cx="500" cy="500" rx="410" ry="330" fill="none" stroke-width="%.2f"/>`, config.Border.Width)
		if config.Border.DoubleLine {
			fmt.Fprintf(output, `<ellipse cx="500" cy="500" rx="390" ry="312" fill="none" stroke-width="%.2f"/>`, math.Max(2, config.Border.Width/2))
		}
	} else {
		fmt.Fprintf(output, `<circle cx="500" cy="500" r="390" fill="none" stroke-width="%.2f"/>`, config.Border.Width)
		if config.Border.DoubleLine {
			fmt.Fprintf(output, `<circle cx="500" cy="500" r="370" fill="none" stroke-width="%.2f"/>`, math.Max(2, config.Border.Width/2))
		}
	}
	if config.Border.InnerRing {
		if config.Shape == "ellipse" {
			fmt.Fprintf(output, `<ellipse cx="500" cy="500" rx="%.2f" ry="%.2f" fill="none" stroke-width="3"/>`, 280+config.Border.InnerAdjust, 220+config.Border.InnerAdjust)
		} else {
			fmt.Fprintf(output, `<circle cx="500" cy="500" r="%.2f" fill="none" stroke-width="3"/>`, 280+config.Border.InnerAdjust)
		}
	}
	output.WriteString(`</g>`)
}

func fontFamily(layer Layer) string {
	if layer.FontID == "system-sans" {
		return "Arial,sans-serif"
	}
	if layer.FontID != "" && layer.FontID != "system-serif" {
		return html.EscapeString(layer.FontID)
	}
	return "STSong,SimSun,serif"
}

func layerTransform(layer Layer, baseY float64) string {
	x, y := 500+layer.OffsetX, baseY+layer.OffsetY
	return fmt.Sprintf("translate(%.2f %.2f) rotate(%.2f) scale(%.2f %.2f) translate(%.2f %.2f)", x, y, layer.Rotation, layer.ScaleX, layer.ScaleY, -x, -y)
}

func writeTextLayer(output *bytes.Buffer, shape string, layer Layer) {
	content := layer.Content
	if layer.ID == "bottom-text" && layer.Kind == "arcText" && shape != "square" {
		content = reverseText(content)
	}
	content = html.EscapeString(content)
	style := fmt.Sprintf(`font-family="%s" font-size="%.2f" letter-spacing="%.2f" font-weight="700"`, fontFamily(layer), layer.FontSize, layer.LetterSpacing)
	if layer.Kind == "arcText" && shape != "square" {
		path := "mainArc"
		if layer.ID == "inner-text" {
			path = "innerArc"
		} else if layer.ID == "bottom-text" {
			path = "bottomArc"
		}
		fmt.Fprintf(output, `<text %s transform="%s"><textPath href="#%s" startOffset="50%%" text-anchor="middle">%s</textPath></text>`, style, layerTransform(layer, 500), path, content)
		return
	}
	baseY := 500.0
	if layer.ID == "main-text" && shape == "square" {
		baseY = 350
	}
	fmt.Fprintf(output, `<text x="%.2f" y="%.2f" text-anchor="middle" %s transform="%s">%s</text>`, 500+layer.OffsetX, baseY+layer.OffsetY, style, layerTransform(layer, baseY), content)
}

func reverseText(value string) string {
	runes := []rune(value)
	for left, right := 0, len(runes)-1; left < right; left, right = left+1, right-1 {
		runes[left], runes[right] = runes[right], runes[left]
	}
	return string(runes)
}

func writeCenterLayer(output *bytes.Buffer, layer Layer) {
	fmt.Fprintf(output, `<text x="%.2f" y="%.2f" text-anchor="middle" dominant-baseline="middle" font-family="%s" font-size="%.2f" letter-spacing="%.2f" font-weight="700" transform="%s">%s</text>`, 500+layer.OffsetX, 500+layer.OffsetY, fontFamily(layer), layer.FontSize, layer.LetterSpacing, layerTransform(layer, 500), html.EscapeString(layer.Content))
}

func writeTexture(output *bytes.Buffer, texture TextureConfig) {
	random := newRandom32(texture.Seed)
	count := int(math.Round(30 + float64(texture.Density)*2.2))
	scale := 0.55 + float64(texture.Intensity)/100
	for index := 0; index < count; index++ {
		a, b, size, alpha := random.Float64(), random.Float64(), random.Float64(), random.Float64()
		x, y := 90+a*820, 90+b*820
		if texture.Type == "edge" {
			angle := a * math.Pi * 2
			radius := 365 + b*(20+float64(texture.EdgeWear)*.45)
			x, y = 500+math.Cos(angle)*radius, 500+math.Sin(angle)*radius
		}
		multiplier := 1.7
		if texture.Type == "ink" {
			multiplier = 2.4
		}
		radius := (.8 + size*float64(texture.GrainSize)*multiplier) * scale
		opacity := math.Min(1, .3+alpha*.55+float64(texture.Fade)/500)
		fmt.Fprintf(output, `<circle cx="%.2f" cy="%.2f" r="%.2f" fill="black" opacity="%.2f"/>`, x, y, radius, opacity)
	}
	lineCount := texture.ScratchCount
	if texture.Type == "paper" {
		lineCount = max(lineCount, int(math.Round(float64(texture.Density)/3)))
	}
	for index := 0; index < lineCount; index++ {
		x, y := 120+random.Float64()*760, 130+random.Float64()*740
		length := 20 + random.Float64()*(45+float64(texture.Intensity)*1.3)
		angleRandom := random.Float64()
		angle := (angleRandom - .5) * 1.2
		if texture.Type == "paper" {
			angle = (angleRandom - .5) * .35
		}
		width := 1 + random.Float64()*float64(texture.GrainSize)*.55
		opacity := .4 + random.Float64()*.5
		fmt.Fprintf(output, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" stroke="black" stroke-width="%.2f" stroke-linecap="round" opacity="%.2f"/>`, x, y, x+math.Cos(angle)*length, y+math.Sin(angle)*length, width, opacity)
	}
}
