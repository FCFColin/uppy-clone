# ADR-012: Chi 作为 HTTP 路由框架

## 状态

已接受

## 背景

Go 后端需要轻量 HTTP 路由、中间件链与 REST API 服务。

## 决策

采用 **go-chi/chi/v5**。

## 理由

- **标准库 net/http 兼容**：无自定义 Request/Response 抽象
- **中间件组合**：与 OTel、Prometheus、RBAC 中间件自然衔接
- **路由模式**：`RoutePattern()` 用于 Prometheus 低基数 path label
- **社区**：Cloudflare、Tailscale 等生产使用案例

## 备选方案

| 方案 | 放弃原因 |
|------|---------|
| Gin | 自定义 Context，与 stdlib 生态耦合度低 |
| Echo/Fiber | 非 stdlib Handler 签名 |
| net/http 裸路由 | 缺少路由组与中间件链 |

## 权衡

- 无内置 OpenAPI 生成，需维护独立 [`../api/openapi.yaml`](../api/openapi.yaml)
- 大型 monolith 路由注册集中在 `internal/server`，需 smoke test 保障
