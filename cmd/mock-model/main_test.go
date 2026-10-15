package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleChatCompletionStreamsWhenRequested(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"stream":true}`))
	response := httptest.NewRecorder()

	handleChatCompletion(response, request)

	if contentType := response.Header().Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", contentType)
	}
	if !response.Flushed {
		t.Fatal("流式响应未调用 Flush")
	}
	body := response.Body.String()
	if !strings.Contains(body, "data: [DONE]\n\n") {
		t.Fatalf("流式响应缺少结束标记: %s", body)
	}
	if !strings.Contains(body, "chat.completion.chunk") {
		t.Fatalf("流式响应缺少聊天分片: %s", body)
	}
}
