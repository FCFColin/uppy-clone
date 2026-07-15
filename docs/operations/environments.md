# 环境配置矩阵

本地、staging、生产环境配置对照。

| 配置项 | 本地 | Staging | 生产 |
|--------|------|---------|------|
| **密钥管理** | `.env` | Cloud Secret Manager | Secret Manager + KMS |
| **TLS** | 关闭（HTTP） | Let's Encrypt | 托管证书 |
| **CORS** | `localhost:*` | `staging.example.com` | 生产域名 |
| **日志级别** | `debug` | `info` | `warn` |
| **OpenTelemetry** | 关闭 | OTLP gRPC | OTLP gRPC |
| **限流** | 放宽（5×） | 正常 | 正常 + WAF |
| **DB 连接池** | 5 | 20 | 50 |
| **Redis 连接池** | 5 | 20 | 50 |
| **最大 WS 连接** | 100 | 500 | 1000+ |
| **pprof** | 开启（`:6060`） | 关闭 | 按需开启 |
| **Metrics 认证** | 无 | Basic Auth | Basic Auth + IP 白名单 |
| **Admin JWT TTL** | 30 分钟 | 30 分钟 | 30 分钟 |
| **优雅关闭超时** | 10s | 20s | 30s |
| **备份** | 手动 | 每日 | 连续 PITR |
| **错误预算策略** | N/A | 仅告警 | 告警 + 冻结发布 |

## REDIS_URL 格式

| 环境 | 示例 |
|------|------|
| 本地 | `localhost:6379` |
| Docker Compose | `redis://:password@redis:6379` |
| K8s | Secret 注入，格式同上 |

应用通过 `config.ParseRedisURL` 解析（见 `.env.example`）。

## 部署目标

- **当前**：单区域 GKE（ADR-014，已接受；单区域阶段已部署）
- **目标态**：多区域 + CockroachDB（ADR-014 多区域待激活 / ADR-015 提议中）
