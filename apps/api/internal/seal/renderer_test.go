package seal

import (
	"bytes"
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{
		SchemaVersion: 2, RendererVersion: RendererVersion, Shape: "circle", Color: "#d92626",
		Canvas: Canvas{LogicalWidth: 1000, LogicalHeight: 1000, ExportWidth: 1200, Transparent: true},
		Border: BorderConfig{Width: 6},
		Layers: []Layer{
			{ID: "border", Kind: "border", Visible: true, Locked: true, ZIndex: 0, ScaleX: 1, ScaleY: 1},
			{ID: "main-text", Kind: "arcText", Visible: true, ZIndex: 10, Content: "示例科技有限公司", FontID: "system-serif", FontSize: 72, LetterSpacing: 6, ScaleX: 1, ScaleY: 1, OffsetY: 20, RadiusX: 335, RadiusY: 335},
			{ID: "inner-text", Kind: "arcText", ZIndex: 11, FontSize: 34, ScaleX: 1, ScaleY: 1, OffsetY: 35, RadiusX: 270, RadiusY: 270},
			{ID: "bottom-text", Kind: "arcText", Visible: true, ZIndex: 12, Content: "合同专用章", FontID: "system-sans", FontSize: 38, LetterSpacing: 4, ScaleX: 1, ScaleY: 1, RadiusX: 260, RadiusY: 260},
			{ID: "center", Kind: "centerText", Visible: true, ZIndex: 20, Content: "★", FontSize: 220, ScaleX: 1, ScaleY: 1},
		},
		Texture: TextureConfig{Enabled: true, Type: "ink", Intensity: 34, Density: 42, GrainSize: 6, EdgeWear: 25, ScratchCount: 8, Fade: 22, Seed: 928341, ApplyTo: []string{"border", "text", "center"}},
	}
}

func TestRendererDeterministicTexture(t *testing.T) {
	config := validConfig()
	first, err := (Renderer{}).SVG(config, true)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	second, err := (Renderer{}).SVG(config, true)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("same seed must produce identical SVG")
	}
	if !strings.Contains(string(first), `mask="url(#wearMask)"`) {
		t.Fatal("expected configured texture mask")
	}
}

func TestRendererEscapesUserText(t *testing.T) {
	config := validConfig()
	config.Layers[1].Content = `<script>alert(1)</script>`
	result, err := (Renderer{}).SVG(config, true)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(result, []byte("<script>")) || !bytes.Contains(result, []byte("&lt;script&gt;")) {
		t.Fatal("user text must be escaped")
	}
}

func TestRendererBottomArcKeepsTextUprightAndInsideRing(t *testing.T) {
	result, err := (Renderer{}).SVG(validConfig(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result, []byte(`<path id="bottomArc" d="M760.00 500.00 A260.00 260.00 0 0 1 240.00 500.00"/>`)) {
		t.Fatal("bottom arc must run right-to-left on the lower half")
	}
	if !bytes.Contains(result, []byte(`>章用专同合</textPath>`)) {
		t.Fatal("bottom arc content must be reversed along its right-to-left path")
	}
}

func TestValidateRejectsInvalidShape(t *testing.T) {
	config := validConfig()
	config.Shape = "triangle"
	if err := Validate(config); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateRejectsDuplicateLayerID(t *testing.T) {
	config := validConfig()
	config.Layers = append(config.Layers, config.Layers[0])
	if err := Validate(config); err == nil {
		t.Fatal("expected duplicate layer error")
	}
}
