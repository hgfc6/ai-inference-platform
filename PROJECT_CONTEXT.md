# 项目上下文

## 项目目标

本项目用于构建并掌握 LLM 推理平台的核心链路。学习目标是对请求路径、性能、稳定性与成本逐步建立技术所有权，而非只调用模型 API。

阶段路线、时间投入和验收标准见 [LEARNING_PLAN.md](LEARNING_PLAN.md)。

## 当前模块

- `cmd/gateway`：HTTP Gateway，接收 `POST /v1/chat/completions`，校验 JSON 后转发给模型后端。
- `cmd/mock-model`：本地开发用的 Mock Model Server，返回固定的 OpenAI 风格响应或 SSE 流式响应。
- `internal/gateway`：Gateway 的请求转发、SSE 透传与错误响应逻辑。

## 当前运行方式

1. 运行 `go run ./cmd/mock-model`，默认监听 `:9528`。
2. 运行 `go run ./cmd/gateway`，默认监听 `:9527`，并将请求转发到 `http://127.0.0.1:9528`。
3. 运行 `go test ./...` 验证请求转发、SSE 透传、客户端取消上游流、非法 JSON 和后端不可用情形。

## 当前边界

- Gateway 使用 Go 标准库 `net/http`，当前没有第三方 Go 依赖。
- v0 已支持 Mock SSE 透传；真实推理引擎的流式兼容性仍待验证。
- 当前模型后端为本地 Mock；未接入 vLLM、GPU 或云服务。
- `created` 字段使用 Unix 时间戳，保持后端时间字段与时区无关。
