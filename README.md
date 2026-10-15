# ai-inference-platform

一个从零构建的 LLM 推理平台学习项目。当前 v0 只实现最小链路：Go Gateway 将聊天请求转发给本地 Mock Model Server。

## 当前能力

- `POST /v1/chat/completions` 请求转发
- 请求体 JSON 校验
- 后端不可用时返回 `502`
- Mock Model Server 返回 OpenAI 风格的聊天响应
- `stream: true` 时透传 Server-Sent Events（SSE）流式响应

当前不包含鉴权、GPU、vLLM、Docker、Kubernetes 和可观测性；这些能力会在后续阶段按真实问题逐项加入。

## 目录

```text
cmd/gateway/       Gateway 进程入口
cmd/mock-model/    本地 Mock Model Server 入口
internal/gateway/  请求转发与错误处理
```

## 本地运行

前置条件：Go 1.25 或更高版本。

终端一，启动 Mock Model Server：

```powershell
go run ./cmd/mock-model
```

终端二，启动 Gateway：

```powershell
go run ./cmd/gateway
```

终端三，发起请求：

```powershell
Invoke-RestMethod -Method Post `
  -Uri http://127.0.0.1:9527/v1/chat/completions `
  -ContentType 'application/json' `
  -Body '{"model":"mock-model","messages":[{"role":"user","content":"你好"}]}'
```

发送流式请求时，使用 `curl.exe -N` 禁用客户端缓冲：

```powershell
curl.exe -N http://127.0.0.1:9527/v1/chat/completions `
  -H "Content-Type: application/json" `
  -d '{"model":"mock-model","stream":true,"messages":[{"role":"user","content":"你好"}]}'
```

## 配置

| 环境变量 | 默认值 | 作用 |
| --- | --- | --- |
| `LISTEN_ADDR` | `:9527` | Gateway 监听地址 |
| `MODEL_BACKEND_URL` | `http://127.0.0.1:9528` | 模型后端基础地址 |
| `MOCK_MODEL_ADDR` | `:9528` | Mock Model Server 监听地址 |

## 验证

```powershell
go test ./...
```
