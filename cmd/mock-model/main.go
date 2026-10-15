package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	listenAddr := envOrDefault("MOCK_MODEL_ADDR", ":9528")

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", handleChatCompletion)

	slog.Info("Mock Model Server 已启动", "listen_addr", listenAddr)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		slog.Error("Mock Model Server 已停止", "error", err)
		os.Exit(1)
	}
}

func handleChatCompletion(w http.ResponseWriter, r *http.Request) {
	var request json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || !json.Valid(request) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体必须是有效 JSON"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":      "chatcmpl-mock",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "mock-model",
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]string{
				"role":    "assistant",
				"content": "这是来自 Mock Model Server 的响应。",
			},
			"finish_reason": "stop",
		}},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("写入 JSON 响应失败", "error", err)
	}
}

func envOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
