# Session Handoff

## 当前目标

- 按 `LEARNING_PLAN.md` 从已完成的 v0 请求路径进入阶段 1：接入真实推理后端。

## 当前状态

- 已完成：项目约束、最小架构、Gateway、Mock Model Server 与自动化测试。
- 已验证：`go test ./...` 通过；已按默认端口 `9527 → 9528` 完成普通 HTTP 与 SSE 的真实转发。
- 已完成：README 本地启动说明与配置说明。
- 已完成：学习目标、12 周路线、时间投入和各阶段验收标准已沉淀到 `LEARNING_PLAN.md`。
- 已完成：Mock Model Server 在 `stream: true` 时发送 SSE；Gateway 透传响应并逐段刷新。
- 已完成：客户端取消 SSE 流时，Gateway 会取消上游请求的集成测试。
- 已完成：5 路 Mock SSE 并发流的 Gateway 端到端基线测试。
- 已完成：Gateway 全局在途请求数上限；满载时立即返回 `503` 与 `Retry-After: 1`，正常完成、后端失败和客户端取消后均会释放名额。
- 未开始：鉴权、指标、真实推理引擎、容器与 Kubernetes。

## 本轮关键改动

- 文件：`cmd/gateway/main.go`、`cmd/mock-model/main.go`、`internal/gateway/handler.go`。
- 影响：新增本地 HTTP Gateway 与 Mock 模型链路；默认端口为 `9527` 和 `9528`。
- 文件：`internal/gateway/handler_test.go`。
- 影响：覆盖成功转发、非法 JSON 与后端不可用三类行为。
- 文件：`cmd/mock-model/main_test.go`。
- 影响：覆盖 Mock SSE 响应和 Gateway 不等待流结束即转发首段数据的行为。
- 文件：`internal/gateway/handler_test.go`。
- 影响：覆盖客户端取消后，上游请求上下文被取消的行为。
- 文件：`cmd/mock-model/main_test.go`。
- 影响：覆盖 5 路并发流均完整返回，且不被 Gateway 串行处理的行为。
- 文件：`cmd/gateway/main.go`、`cmd/gateway/main_test.go`、`internal/gateway/handler.go`、`internal/gateway/handler_test.go`、`README.md`。
- 影响：新增 `MAX_INFLIGHT_REQUESTS`（默认 `8`）全局准入上限和满载 `503` 行为；覆盖满载拒绝、完成/失败/取消后的名额释放及非法配置。

## 已确认决策

- 决策：v0 使用原生 `net/http`，不引入 Gin、go-micro 或其他第三方 Go 依赖。
- 原因：当前目标是掌握 HTTP 转发、上下文、超时和错误边界；单接口场景不需要微服务框架。
- 决策：当前过载策略为立即拒绝，不引入等待队列。
- 原因：队列会额外引入排队超时、取消、公平性和内存上限问题；真实模型性能基线尚未建立。

## 已知风险 / 坑位

- 当前 SSE 仅由 Mock 后端验证；真实推理引擎的分片格式、取消语义和长连接行为仍需单独验证。
- Mock 在每次 SSE 刷新后等待 1 秒以便手工观察分段；该延迟仅用于模拟数据间隔，不能用于推断真实模型性能，并会增加相关测试耗时。
- `MAX_INFLIGHT_REQUESTS=8` 仅是 Mock 阶段的安全基线，不能直接用于真实模型；接入模型后需根据显存、TTFT、吞吐和错误率重新确定。
- 原默认端口 `8081` 在本机曾发生占用；当前默认端口已调整为 `9527` 和 `9528`，仍可通过 `MOCK_MODEL_ADDR`、`LISTEN_ADDR` 和 `MODEL_BACKEND_URL` 覆盖。
- 本机 Git 会警告无法读取 `C:\Users\admin\.config\git\ignore`；这是用户级 Git 配置访问问题，不影响当前仓库状态或测试。

## 下一步

1. 确认本机是否存在可用 NVIDIA GPU、显存规格和兼容的驱动/运行环境。
2. 根据该事实决定使用本地或远程 GPU；若需产生费用，先征得用户确认。
3. 接入真实推理引擎后，重新验证 SSE 的分片格式与取消语义，再处理请求 ID、结构化日志和基础指标。
