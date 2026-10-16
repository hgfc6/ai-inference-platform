package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// DefaultMaxInFlight 是 Gateway 默认允许的最大在途请求数。
const DefaultMaxInFlight = 8

// Handler 将聊天请求转发给模型后端。
type Handler struct {
	backendURL *url.URL
	client     *http.Client
	inFlight   chan struct{}
}

// NewHandler 创建使用指定模型后端的 Gateway Handler。
func NewHandler(backendURL string, client *http.Client) (*Handler, error) {
	return NewHandlerWithMaxInFlight(backendURL, client, DefaultMaxInFlight)
}

// NewHandlerWithMaxInFlight 创建带全局并发上限的 Gateway Handler。
func NewHandlerWithMaxInFlight(backendURL string, client *http.Client, maxInFlight int) (*Handler, error) {
	parsedURL, err := url.Parse(backendURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("模型后端地址无效: %q", backendURL)
	}
	if maxInFlight <= 0 {
		return nil, fmt.Errorf("最大在途请求数必须大于 0: %d", maxInFlight)
	}
	if client == nil {
		client = http.DefaultClient
	}

	return &Handler{
		backendURL: parsedURL,
		client:     client,
		inFlight:   make(chan struct{}, maxInFlight),
	}, nil
}

// ServeHTTP 校验 JSON 请求并将其转发给模型后端。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "仅支持 POST 请求")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "读取请求体失败")
		return
	}
	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求体必须是有效 JSON")
		return
	}
	if !h.tryAcquire() {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, "gateway_overloaded", "Gateway 正忙，请稍后重试")
		return
	}
	defer h.release()

	targetURL := *h.backendURL
	targetURL.Path = strings.TrimRight(targetURL.Path, "/") + r.URL.Path
	targetURL.RawQuery = r.URL.RawQuery

	backendRequest, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL.String(), bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "gateway_error", "创建后端请求失败")
		return
	}
	backendRequest.Header.Set("Content-Type", "application/json")

	backendResponse, err := h.client.Do(backendRequest)
	if err != nil {
		writeError(w, http.StatusBadGateway, "backend_unavailable", "模型后端不可用")
		return
	}
	defer backendResponse.Body.Close()

	if contentType := backendResponse.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if cacheControl := backendResponse.Header.Get("Cache-Control"); cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}
	w.WriteHeader(backendResponse.StatusCode)
	if isEventStream(backendResponse.Header.Get("Content-Type")) {
		if _, err := io.Copy(flushingWriter{ResponseWriter: w}, backendResponse.Body); err != nil {
			return
		}
		return
	}
	if _, err := io.Copy(w, backendResponse.Body); err != nil {
		return
	}
}

func (h *Handler) tryAcquire() bool {
	select {
	case h.inFlight <- struct{}{}:
		return true
	default:
		return false
	}
}

func (h *Handler) release() {
	<-h.inFlight
}

func isEventStream(contentType string) bool {
	mediaType, _, err := strings.Cut(contentType, ";")
	if err {
		contentType = mediaType
	}
	return strings.EqualFold(strings.TrimSpace(contentType), "text/event-stream")
}

type flushingWriter struct {
	http.ResponseWriter
}

func (w flushingWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if err != nil {
		return n, err
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
	return n, nil
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
