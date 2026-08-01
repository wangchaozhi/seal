package raster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Rasterizer interface {
	PNG(ctx context.Context, svg []byte, width int) ([]byte, error)
}

type ImageProcessor interface {
	Reencode(ctx context.Context, input []byte, contentType string) ([]byte, int, int, error)
}

type HTTPClient struct {
	BaseURL          string
	Client           *http.Client
	MaxResponseBytes int64
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{BaseURL: strings.TrimRight(baseURL, "/"), Client: &http.Client{Timeout: 30 * time.Second}, MaxResponseBytes: 64 << 20}
}

func (client *HTTPClient) PNG(ctx context.Context, svg []byte, width int) ([]byte, error) {
	payload, err := json.Marshal(map[string]any{"svg": string(svg), "width": width})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/render", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	reader := io.LimitReader(response.Body, client.MaxResponseBytes+1)
	result, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if int64(len(result)) > client.MaxResponseBytes {
		return nil, errors.New("rasterizer response too large")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rasterizer returned %d", response.StatusCode)
	}
	if len(result) < 8 || !bytes.Equal(result[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return nil, errors.New("rasterizer returned invalid PNG")
	}
	return result, nil
}

func (client *HTTPClient) Reencode(ctx context.Context, input []byte, contentType string) ([]byte, int, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/images/reencode", bytes.NewReader(input))
	if err != nil {
		return nil, 0, 0, err
	}
	request.Header.Set("Content-Type", contentType)
	response, err := client.Client.Do(request)
	if err != nil {
		return nil, 0, 0, err
	}
	defer response.Body.Close()
	result, err := io.ReadAll(io.LimitReader(response.Body, client.MaxResponseBytes+1))
	if err != nil {
		return nil, 0, 0, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, 0, 0, fmt.Errorf("image processor returned %d", response.StatusCode)
	}
	if len(result) < 8 || !bytes.Equal(result[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return nil, 0, 0, errors.New("image processor returned invalid PNG")
	}
	width, err := strconv.Atoi(response.Header.Get("X-Image-Width"))
	if err != nil {
		return nil, 0, 0, errors.New("image processor omitted width")
	}
	height, err := strconv.Atoi(response.Header.Get("X-Image-Height"))
	if err != nil {
		return nil, 0, 0, errors.New("image processor omitted height")
	}
	return result, width, height, nil
}
