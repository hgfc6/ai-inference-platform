package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/hgfc6/ai-inference-platform/internal/gateway"
)

func main() {
	listenAddr := envOrDefault("LISTEN_ADDR", ":9527")
	backendURL := envOrDefault("MODEL_BACKEND_URL", "http://127.0.0.1:9528")
	maxInFlight, err := envPositiveIntOrDefault("MAX_INFLIGHT_REQUESTS", gateway.DefaultMaxInFlight)
	if err != nil {
		slog.Error("读取 Gateway 配置失败", "error", err)
		os.Exit(1)
	}

	handler, err := gateway.NewHandlerWithMaxInFlight(backendURL, &http.Client{Timeout: 30 * time.Second}, maxInFlight)
	if err != nil {
		slog.Error("初始化 Gateway 失败", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /v1/chat/completions", handler)

	slog.Info("Gateway 已启动", "listen_addr", listenAddr, "model_backend_url", backendURL, "max_inflight_requests", maxInFlight)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		slog.Error("Gateway 已停止", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func envPositiveIntOrDefault(key string, defaultValue int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s 必须是大于 0 的整数: %q", key, value)
	}
	return parsed, nil
}
