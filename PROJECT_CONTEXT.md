# 项目上下文

## 项目目标

本项目用于构建并掌握 LLM 推理平台的核心链路。学习目标是对请求路径、性能、稳定性与成本逐步建立技术所有权，而非只调用模型 API。

## 当前模块

- `cmd/gateway`：HTTP Gateway，接收 `POST /v1/chat/completions`，校验 JSON 后转发给模型后端。
- `cmd/mock-model`：本地开发用的 Mock Model Server，返回固定的 OpenAI 风格聊天响应。
- `internal/gateway`：Gateway 的请求转发与错误响应逻辑。

## 当前运行方式

1. 运行 `go run ./cmd/mock-model`，默认监听 `:9528`。
2. 运行 `go run ./cmd/gateway`，默认监听 `:9527`，并将请求转发到 `http://127.0.0.1:9528`。
3. 运行 `go test ./...` 验证请求转发、非法 JSON 和后端不可用三种情形。

## 当前边界

- Gateway 使用 Go 标准库 `net/http`，当前没有第三方 Go 依赖。
- v0 仅处理非流式 JSON 请求。
- 当前模型后端为本地 Mock；未接入 vLLM、GPU 或云服务。
- `created` 字段使用 Unix 时间戳，保持后端时间字段与时区无关。
