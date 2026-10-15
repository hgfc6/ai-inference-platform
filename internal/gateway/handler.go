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

// Handler 将聊天请求转发给模型后端。
type Handler struct {
	backendURL *url.URL
	client     *http.Client
}

// NewHandler 创建使用指定模型后端的 Gateway Handler。
func NewHandler(backendURL string, client *http.Client) (*Handler, error) {
	parsedURL, err := url.Parse(backendURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("模型后端地址无效: %q", backendURL)
	}
	if client == nil {
		client = http.DefaultClient
	}

	return &Handler{backendURL: parsedURL, client: client}, nil
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
	w.WriteHeader(backendResponse.StatusCode)
	if _, err := io.Copy(w, backendResponse.Body); err != nil {
		return
	}
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
