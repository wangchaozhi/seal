package seal

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type Handler struct {
	Renderer     Renderer
	MaxBodyBytes int64
}

func (handler Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/seals/render", handler.render)
}

func (handler Handler) render(writer http.ResponseWriter, request *http.Request) {
	config, err := handler.decode(request)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	if config.Canvas.ExportWidth > 1200 {
		config.Canvas.ExportWidth = 1200
	}
	result, err := handler.Renderer.SVG(config, true)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	writer.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Disposition", `attachment; filename="seal.svg"`)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(result)
}

func (handler Handler) decode(request *http.Request) (Config, error) {
	var config Config
	reader := io.Reader(request.Body)
	if handler.MaxBodyBytes > 0 {
		reader = io.LimitReader(request.Body, handler.MaxBodyBytes+1)
	}

	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, errors.New("invalid JSON body")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("request must contain a single JSON object")
	}
	config.ApplyDefaults()
	return config, nil
}
