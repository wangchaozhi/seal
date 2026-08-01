package seal

import (
	"errors"
	"fmt"
	"regexp"
	"unicode/utf8"
)

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var rendererVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func Validate(config Config) error {
	if config.SchemaVersion != 2 {
		return errors.New("schemaVersion must be 2")
	}
	if config.RendererVersion != "" && !rendererVersionPattern.MatchString(config.RendererVersion) {
		return errors.New("rendererVersion must use semantic version format")
	}
	if config.Shape != "circle" && config.Shape != "ellipse" && config.Shape != "square" {
		return errors.New("shape must be circle, ellipse or square")
	}
	if config.Canvas.LogicalWidth != 1000 || config.Canvas.LogicalHeight != 1000 {
		return errors.New("logical canvas must be 1000x1000")
	}
	if config.Canvas.ExportWidth < 300 || config.Canvas.ExportWidth > 5000 {
		return errors.New("canvas.exportWidth must be between 300 and 5000")
	}
	if !colorPattern.MatchString(config.Color) {
		return errors.New("color must be a six-digit HEX value")
	}
	if config.Border.Width < 1 || config.Border.Width > 20 || config.Border.InnerAdjust < -50 || config.Border.InnerAdjust > 50 {
		return errors.New("border values are outside the allowed range")
	}
	if len(config.Layers) < 1 || len(config.Layers) > 20 {
		return errors.New("layers must contain between 1 and 20 items")
	}
	ids := make(map[string]bool, len(config.Layers))
	for index, layer := range config.Layers {
		if err := validateLayer(layer); err != nil {
			return fmt.Errorf("layers[%d]: %w", index, err)
		}
		if ids[layer.ID] {
			return fmt.Errorf("layers[%d]: duplicate id", index)
		}
		ids[layer.ID] = true
	}
	if err := validateTexture(config.Texture); err != nil {
		return err
	}
	return nil
}

func validateLayer(layer Layer) error {
	if len(layer.ID) < 1 || len(layer.ID) > 64 {
		return errors.New("id length must be between 1 and 64")
	}
	validKind := layer.Kind == "arcText" || layer.Kind == "text" || layer.Kind == "centerText" || layer.Kind == "centerImage" || layer.Kind == "border" || layer.Kind == "innerRing"
	if !validKind {
		return errors.New("unsupported kind")
	}
	if layer.ZIndex < 0 || layer.ZIndex > 100 || utf8.RuneCountInString(layer.Content) > 100 || len(layer.FontID) > 64 || len(layer.AssetID) > 128 {
		return errors.New("metadata is outside the allowed range")
	}
	if layer.FontSize < 0 || layer.FontSize > 400 || layer.LetterSpacing < -20 || layer.LetterSpacing > 100 {
		return errors.New("typography is outside the allowed range")
	}
	if layer.ScaleX < .2 || layer.ScaleX > 3 || layer.ScaleY < .2 || layer.ScaleY > 3 {
		return errors.New("scale must be between 0.2 and 3")
	}
	if layer.Rotation < -360 || layer.Rotation > 360 || layer.OffsetX < -500 || layer.OffsetX > 500 || layer.OffsetY < -500 || layer.OffsetY > 500 {
		return errors.New("transform is outside the allowed range")
	}
	if layer.Kind == "arcText" && (layer.RadiusX < 10 || layer.RadiusX > 500 || layer.RadiusY < 10 || layer.RadiusY > 500) {
		return errors.New("arc radius must be between 10 and 500")
	}
	return nil
}

func validateTexture(texture TextureConfig) error {
	validType := texture.Type == "ink" || texture.Type == "grain" || texture.Type == "edge" || texture.Type == "scratch" || texture.Type == "paper"
	if !validType {
		return errors.New("texture.type is unsupported")
	}
	if texture.Intensity < 0 || texture.Intensity > 100 || texture.Density < 0 || texture.Density > 100 || texture.EdgeWear < 0 || texture.EdgeWear > 100 || texture.Fade < 0 || texture.Fade > 100 {
		return errors.New("texture percentage must be between 0 and 100")
	}
	if texture.GrainSize < 1 || texture.GrainSize > 24 || texture.ScratchCount < 0 || texture.ScratchCount > 40 || texture.Seed < 1 || texture.Seed > 2147483647 {
		return errors.New("texture values are outside the allowed range")
	}
	if len(texture.ApplyTo) < 1 || len(texture.ApplyTo) > 3 {
		return errors.New("texture.applyTo must contain between 1 and 3 targets")
	}
	seen := map[string]bool{}
	for _, target := range texture.ApplyTo {
		if target != "border" && target != "text" && target != "center" {
			return errors.New("texture.applyTo contains an unsupported target")
		}
		if seen[target] {
			return errors.New("texture.applyTo must contain unique targets")
		}
		seen[target] = true
	}
	return nil
}
