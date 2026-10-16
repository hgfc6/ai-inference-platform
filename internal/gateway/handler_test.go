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
	"sync/atomic"
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

func TestHandlerReleasesSlotWhenBackendRequestFails(t *testing.T) {
	handler, err := NewHandlerWithMaxInFlight("http://model.example", &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("后端连接失败")
	})}, 1)
	if err != nil {
		t.Fatalf("创建 Handler 失败: %v", err)
	}

	for range 2 {
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"mock-model"}`))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadGateway {
			t.Fatalf("后端失败后的状态码 = %d, want %d", response.Code, http.StatusBadGateway)
		}
	}
}

func TestHandlerRejectsWhenAtCapacityAndReleasesSlotAfterCompletion(t *testing.T) {
	backendStarted := make(chan struct{}, 1)
	releaseBackend := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case backendStarted <- struct{}{}:
		default:
		}
		<-releaseBackend
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer backend.Close()

	handler, err := NewHandlerWithMaxInFlight(backend.URL, backend.Client(), 1)
	if err != nil {
		t.Fatalf("创建 Handler 失败: %v", err)
	}
	gatewayServer := httptest.NewServer(handler)
	defer gatewayServer.Close()

	firstDone := make(chan error, 1)
	go func() {
		response, err := http.Post(gatewayServer.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"mock-model"}`))
		if err == nil {
			response.Body.Close()
		}
		firstDone <- err
	}()

	select {
	case <-backendStarted:
	case <-time.After(time.Second):
		t.Fatal("首个请求未到达模型后端")
	}

	startedAt := time.Now()
	response, err := http.Post(gatewayServer.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"mock-model"}`))
	if err != nil {
		t.Fatalf("发起满载请求失败: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("满载状态码 = %d, want %d", response.StatusCode, http.StatusServiceUnavailable)
	}
	if response.Header.Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q, want 1", response.Header.Get("Retry-After"))
	}
	if !strings.Contains(readBody(t, response), "gateway_overloaded") {
		t.Fatal("满载响应未包含 gateway_overloaded")
	}
	if elapsed := time.Since(startedAt); elapsed > 200*time.Millisecond {
		t.Fatalf("满载请求耗时 %s，未被快速拒绝", elapsed)
	}

	close(releaseBackend)
	if err := <-firstDone; err != nil {
		t.Fatalf("首个请求失败: %v", err)
	}

	response, err = http.Post(gatewayServer.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"mock-model"}`))
	if err != nil {
		t.Fatalf("释放名额后的请求失败: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("释放名额后的状态码 = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestHandlerReleasesSlotAfterClientCancelsStream(t *testing.T) {
	backendCancelled := make(chan struct{}, 1)
	var requestCount atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestCount.Add(1) > 1 {
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: first\n\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		backendCancelled <- struct{}{}
	}))
	defer backend.Close()

	handler, err := NewHandlerWithMaxInFlight(backend.URL, backend.Client(), 1)
	if err != nil {
		t.Fatalf("创建 Handler 失败: %v", err)
	}
	handlerReturned := make(chan struct{}, 1)
	gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
		handlerReturned <- struct{}{}
	}))
	defer gatewayServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, gatewayServer.URL+"/v1/chat/completions", strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatalf("创建客户端请求失败: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("发起流式请求失败: %v", err)
	}
	reader := bufio.NewReader(response.Body)
	if firstLine, err := reader.ReadString('\n'); err != nil || firstLine != "data: first\n" {
		response.Body.Close()
		t.Fatalf("第一段流式响应 = %q, err = %v", firstLine, err)
	}

	cancel()
	response.Body.Close()

	select {
	case <-backendCancelled:
	case <-time.After(time.Second):
		t.Fatal("客户端取消后，模型后端未收到取消信号")
	}
	select {
	case <-handlerReturned:
	case <-time.After(time.Second):
		t.Fatal("客户端取消后，Gateway Handler 未结束")
	}

	response, err = http.Post(gatewayServer.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"mock-model"}`))
	if err != nil {
		t.Fatalf("取消后的请求失败: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("取消后的状态码 = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestNewHandlerWithMaxInFlightRejectsNonPositiveLimit(t *testing.T) {
	_, err := NewHandlerWithMaxInFlight("http://model.example", http.DefaultClient, 0)
	if err == nil {
		t.Fatal("最大在途请求数为 0 时未返回错误")
	}
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}
	return string(body)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
