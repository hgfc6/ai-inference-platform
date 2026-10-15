# Session Handoff

## 当前目标

- 完成 AI Inference Platform v0：Gateway 转发本地 Mock Model Server 的聊天请求。

## 当前状态

- 已完成：项目约束、最小架构、Gateway、Mock Model Server 与自动化测试。
- 已验证：`go test ./...` 通过；已按默认端口 `9527 → 9528` 完成一次真实 HTTP 转发。
- 已完成：README 本地启动说明与配置说明。
- 未开始：流式 SSE、鉴权、指标、真实推理引擎、容器与 Kubernetes。

## 本轮关键改动

- 文件：`cmd/gateway/main.go`、`cmd/mock-model/main.go`、`internal/gateway/handler.go`。
- 影响：新增本地 HTTP Gateway 与 Mock 模型链路；默认端口为 `9527` 和 `9528`。
- 文件：`internal/gateway/handler_test.go`。
- 影响：覆盖成功转发、非法 JSON 与后端不可用三类行为。

## 已确认决策

- 决策：v0 使用原生 `net/http`，不引入 Gin、go-micro 或其他第三方 Go 依赖。
- 原因：当前目标是掌握 HTTP 转发、上下文、超时和错误边界；单接口场景不需要微服务框架。

## 已知风险 / 坑位

- 当前仅支持非流式 JSON 响应，后续接入真实模型时不能把长连接与 SSE 当成普通 JSON 响应处理。
- 原默认端口 `8081` 在本机曾发生占用；当前默认端口已调整为 `9527` 和 `9528`，仍可通过 `MOCK_MODEL_ADDR`、`LISTEN_ADDR` 和 `MODEL_BACKEND_URL` 覆盖。
- 本机 Git 会警告无法读取 `C:\Users\admin\.config\git\ignore`；这是用户级 Git 配置访问问题，不影响当前仓库状态或测试。

## 下一步

1. 为 Gateway 加入请求 ID、结构化访问日志与基础指标。
2. 接入真实推理后端前，先补充流式 SSE 的协议与测试。
