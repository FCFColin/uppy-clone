# 日志级别策略

> 统一日志级别规范防止生产环境 DEBUG 暴增（成本与检索性能），确保 on-call 在 WARN/ERROR 中看到可行动信息。

## 级别定义

| 级别 | 用途 | 示例 | 生产环境 |
|------|------|------|---------|
| **DEBUG** | 开发调试细节，对排障价值低 | 协议帧 hex dump、SQL 参数 | 关闭（`LOG_LEVEL=info`） |
| **INFO** | 正常业务事件、请求完成 | `request completed`、`room created` | 开启 |
| **WARN** | 可恢复异常、降级、安全信号 | 熔断器打开、限流触发、审计 channel 满 | 开启 |
| **ERROR** | 需人工介入的失败 | DB 写入失败、Worker 消费失败 | 开启，必须含 `error` + `caller` |
| **FATAL** | 进程无法继续（Go 中少用，多用 `os.Exit`） | 启动配置缺失 | 极少使用 |

## 结构化字段规范

每条 HTTP 请求日志应包含（中间件自动注入）：`request_id`（chi RequestID）、`trace_id`（OpenTelemetry，若启用）、`latency_ms`（访问日志）、`user_id` / `role`（认证后注入）。

## 禁止事项

1. **禁止**记录密码、JWT 原文、Magic Link token、API Key、Refresh Token
2. **禁止**记录完整 email（可 hash 或截断：`u***@example.com`）
3. **禁止**在 INFO 级别记录每次 DB 查询（用 OTel span 代替）
4. **禁止**在 ERROR 中输出完整 stack trace 到 stdout（用 `slogctx.Error` 的 `caller` 字段）

## 本地开发

```bash
LOG_LEVEL=debug LOG_FORMAT=text go run ./cmd/server
```

## 生产环境

```bash
LOG_LEVEL=info LOG_FORMAT=json
OTEL_EXPORTER_OTLP_ENDPOINT=tempo:4317  # 可选，启用链路追踪
```

## 审计日志

敏感操作使用 `audit.Log()`，独立于应用日志流，写入 `audit_logs` 表（HMAC 链防篡改）。详见 ADR-008。
