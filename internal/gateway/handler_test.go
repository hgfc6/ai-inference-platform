package gateway

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
