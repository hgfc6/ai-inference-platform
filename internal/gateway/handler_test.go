package gateway

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHandlerForwardsChatCompletion(t *testing.T) {
	requestBody := `{"model":"mock-model","messages":[{"role":"user","content":"你好"}]}`
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("请求方法 = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("请求路径 = %s, want /v1/chat/completions", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("读取后端请求体失败: %v", err)
		}
		if string(body) != requestBody {
			t.Fatalf("请求体 = %s, want %s", body, requestBody)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test"}`))
	}))
	defer backend.Close()

	handler, err := NewHandler(backend.URL, backend.Client())
	if err != nil {
		t.Fatalf("创建 Handler 失败: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(requestBody))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("状态码 = %d, want %d", response.Code, http.StatusCreated)
	}
	if response.Body.String() != `{"id":"chatcmpl-test"}` {
		t.Fatalf("响应体 = %s", response.Body.String())
	}
}

func TestHandlerRejectsInvalidJSON(t *testing.T) {
	handler, err := NewHandler("http://model.example", http.DefaultClient)
	if err != nil {
		t.Fatalf("创建 Handler 失败: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "invalid_request") {
		t.Fatalf("响应体未包含 invalid_request: %s", response.Body.String())
	}
}

func TestHandlerForwardsEventStreamWithoutWaitingForCompletion(t *testing.T) {
	secondChunk := make(chan struct{})
	var closeSecondChunk sync.Once
	defer closeSecondChunk.Do(func() { close(secondChunk) })

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte("data: first\n\n"))
		w.(http.Flusher).Flush()
		<-secondChunk
		_, _ = w.Write([]byte("data: second\n\ndata: [DONE]\n\n"))
	}))
	defer backend.Close()

	handler, err := NewHandler(backend.URL, backend.Client())
	if err != nil {
		t.Fatalf("创建 Handler 失败: %v", err)
	}
	gatewayServer := httptest.NewServer(handler)
	defer gatewayServer.Close()

	client := &http.Client{Timeout: time.Second}
	response, err := client.Post(gatewayServer.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatalf("发起流式请求失败: %v", err)
	}
	defer response.Body.Close()

	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", contentType)
	}
	if response.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", response.Header.Get("Cache-Control"))
	}

	reader := bufio.NewReader(response.Body)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("读取第一段流式响应失败: %v", err)
	}
	if firstLine != "data: first\n" {
		t.Fatalf("第一段流式响应 = %q", firstLine)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("读取第一段流式响应分隔行失败: %v", err)
	}

	closeSecondChunk.Do(func() { close(secondChunk) })
	remaining, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("读取剩余流式响应失败: %v", err)
	}
	if string(remaining) != "data: second\n\ndata: [DONE]\n\n" {
		t.Fatalf("剩余流式响应 = %q", remaining)
	}
}

func TestHandlerCancelsUpstreamStreamWhenClientCancels(t *testing.T) {
	backendCancelled := make(chan struct{}, 1)
	releaseBackend := make(chan struct{})
	defer close(releaseBackend)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: first\n\n"))
		w.(http.Flusher).Flush()

		select {
		case <-r.Context().Done():
			backendCancelled <- struct{}{}
		case <-releaseBackend:
		}
	}))
	defer backend.Close()

	handler, err := NewHandler(backend.URL, backend.Client())
	if err != nil {
		t.Fatalf("创建 Handler 失败: %v", err)
	}
	gatewayServer := httptest.NewServer(handler)
	defer gatewayServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, gatewayServer.URL+"/v1/chat/completions", strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatalf("创建客户端请求失败: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := (&http.Client{Timeout: time.Second}).Do(request)
	if err != nil {
		t.Fatalf("发起流式请求失败: %v", err)
	}
	reader := bufio.NewReader(response.Body)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		response.Body.Close()
		t.Fatalf("读取第一段流式响应失败: %v", err)
	}
	if firstLine != "data: first\n" {
		response.Body.Close()
		t.Fatalf("第一段流式响应 = %q", firstLine)
	}

	cancel()
	response.Body.Close()

	select {
	case <-backendCancelled:
	case <-time.After(time.Second):
		t.Fatal("客户端取消后，Gateway 未取消上游流式请求")
	}
}

func TestHandlerReturnsBadGatewayWhenBackendUnavailable(t *testing.T) {
	handler, err := NewHandler("http://model.example", &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("后端连接失败")
	})})
	if err != nil {
		t.Fatalf("创建 Handler 失败: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"mock-model"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("状态码 = %d, want %d", response.Code, http.StatusBadGateway)
	}
	if !strings.Contains(response.Body.String(), "backend_unavailable") {
		t.Fatalf("响应体未包含 backend_unavailable: %s", response.Body.String())
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
