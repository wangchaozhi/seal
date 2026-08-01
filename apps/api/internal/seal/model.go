package seal

const RendererVersion = "2.0.0"

type Canvas struct {
	LogicalWidth  int  `json:"logicalWidth"`
	LogicalHeight int  `json:"logicalHeight"`
	ExportWidth   int  `json:"exportWidth"`
	Transparent   bool `json:"transparent"`
}

type BorderConfig struct {
	Width       float64 `json:"width"`
	DoubleLine  bool    `json:"doubleLine"`
	InnerRing   bool    `json:"innerRing"`
	InnerAdjust float64 `json:"innerAdjust"`
}

type Layer struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"`
	Visible       bool    `json:"visible"`
	Locked        bool    `json:"locked"`
	ZIndex        int     `json:"zIndex"`
	Content       string  `json:"content,omitempty"`
	FontID        string  `json:"fontId,omitempty"`
	FontSize      float64 `json:"fontSize,omitempty"`
	LetterSpacing float64 `json:"letterSpacing,omitempty"`
	ScaleX        float64 `json:"scaleX,omitempty"`
	ScaleY        float64 `json:"scaleY,omitempty"`
	Rotation      float64 `json:"rotation,omitempty"`
	OffsetX       float64 `json:"offsetX,omitempty"`
	OffsetY       float64 `json:"offsetY,omitempty"`
	RadiusX       float64 `json:"radiusX,omitempty"`
	RadiusY       float64 `json:"radiusY,omitempty"`
	StartAngle    float64 `json:"startAngle,omitempty"`
	SweepAngle    float64 `json:"sweepAngle,omitempty"`
	AssetID       string  `json:"assetId,omitempty"`
	AssetData     string  `json:"-"`
}

type TextureConfig struct {
	Enabled      bool     `json:"enabled"`
	Type         string   `json:"type"`
	Intensity    int      `json:"intensity"`
	Density      int      `json:"density"`
	GrainSize    int      `json:"grainSize"`
	EdgeWear     int      `json:"edgeWear"`
	ScratchCount int      `json:"scratchCount"`
	Fade         int      `json:"fade"`
	Seed         int64    `json:"seed"`
	ApplyTo      []string `json:"applyTo"`
}

type Config struct {
	SchemaVersion   int           `json:"schemaVersion"`
	RendererVersion string        `json:"rendererVersion,omitempty"`
	Shape           string        `json:"shape"`
	Canvas          Canvas        `json:"canvas"`
	Color           string        `json:"color"`
	Border          BorderConfig  `json:"border"`
	Layers          []Layer       `json:"layers"`
	Texture         TextureConfig `json:"texture"`
}

func (config *Config) ApplyDefaults() {
	if config.RendererVersion == "" {
		config.RendererVersion = RendererVersion
	}
	for index := range config.Layers {
		if config.Layers[index].ScaleX == 0 {
			config.Layers[index].ScaleX = 1
		}
		if config.Layers[index].ScaleY == 0 {
			config.Layers[index].ScaleY = 1
		}
	}
}

func (config Config) Layer(id string) (Layer, bool) {
	for _, layer := range config.Layers {
		if layer.ID == id {
			return layer, true
		}
	}
	return Layer{}, false
}
