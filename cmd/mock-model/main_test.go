package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hgfc6/ai-inference-platform/internal/gateway"
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

func TestGatewayForwardsConcurrentMockStreams(t *testing.T) {
	const requestCount = 5

	mockServer := httptest.NewServer(http.HandlerFunc(handleChatCompletion))
	defer mockServer.Close()

	handler, err := gateway.NewHandler(mockServer.URL, mockServer.Client())
	if err != nil {
		t.Fatalf("创建 Gateway Handler 失败: %v", err)
	}
	gatewayServer := httptest.NewServer(handler)
	defer gatewayServer.Close()

	startedAt := time.Now()
	errors := make(chan error, requestCount)
	var waitGroup sync.WaitGroup
	for range requestCount {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			response, err := (&http.Client{Timeout: 4 * time.Second}).Post(
				gatewayServer.URL+"/v1/chat/completions",
				"application/json",
				strings.NewReader(`{"model":"mock-model","stream":true}`),
			)
			if err != nil {
				errors <- fmt.Errorf("发起流式请求失败: %w", err)
				return
			}
			defer response.Body.Close()

			if response.StatusCode != http.StatusOK {
				errors <- fmt.Errorf("状态码 = %d, want %d", response.StatusCode, http.StatusOK)
				return
			}
			body, err := io.ReadAll(response.Body)
			if err != nil {
				errors <- fmt.Errorf("读取流式响应失败: %w", err)
				return
			}
			if !strings.Contains(string(body), "data: [DONE]\n\n") {
				errors <- fmt.Errorf("流式响应缺少结束标记: %s", body)
			}
		}()
	}
	waitGroup.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
	elapsed := time.Since(startedAt)
	if elapsed > 4*time.Second {
		t.Fatalf("%d 个流式请求耗时 %s，疑似被串行处理", requestCount, elapsed)
	}
	t.Logf("%d 个并发流式请求耗时 %s", requestCount, elapsed)
}
