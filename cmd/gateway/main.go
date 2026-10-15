package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/hgfc6/ai-inference-platform/internal/gateway"
)

func main() {
	listenAddr := envOrDefault("LISTEN_ADDR", ":9527")
	backendURL := envOrDefault("MODEL_BACKEND_URL", "http://127.0.0.1:9528")

	handler, err := gateway.NewHandler(backendURL, &http.Client{Timeout: 30 * time.Second})
	if err != nil {
		slog.Error("初始化 Gateway 失败", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /v1/chat/completions", handler)

	slog.Info("Gateway 已启动", "listen_addr", listenAddr, "model_backend_url", backendURL)
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
