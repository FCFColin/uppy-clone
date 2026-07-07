# v2 资产清单与适用轴映射

> 生成日期：2026-07-08
> 基于：v1 资产清单（A-001~A-083）+ v2 校验修正

---

## 1. v1 → v2 资产清单修正

| 资产 ID | v1 状态 | v2 修正 | 修正原因 |
|---------|--------|---------|---------|
| A-005 constants | Low | **High** | 跨前后端协议关键性 |
| A-032 gen-frontend-constants | Low | **Medium** | 代码生成一致性 |
| A-045 shard/game | "目录不存在，已移除" | **存在**，路径为 `frontend/src/shared/game/` | v1 拼写错误（shard vs shared），实际目录存在 |
| ~~A-045 永久移除~~ | — | **恢复**，实际 83 资产 | v1 盲区修正 |

**v2 实际审查资产数：83 个**（v1 误报为 82）

---

## 2. v1 路径命名修正

v1 资产清单中所有 `shard/*` 路径实际为 `shared/*`：
- A-044: `frontend/src/shard/network/` → `frontend/src/shared/network/`
- A-045: `frontend/src/shard/game/` → `frontend/src/shared/game/`（v1 误判不存在）
- A-046: `frontend/src/shard/ui/` → `frontend/src/shared/ui/`
- A-047: `frontend/src/shard/data/` → `frontend/src/shared/data/`
- A-048: `frontend/src/shard/assets/` → `frontend/src/shared/assets/`

**v1 报告中的所有 "shard" 引用应理解为 "shared"。**

---

## 3. 新增资产（v1 漏审）

| 资产 | 路径 | 说明 |
|------|------|------|
| A-045a | `frontend/src/shared/ui/toast.ts` + `toast.test.ts` | v1 漏审，应纳入 A-046 shard/ui 范围 |

---

## 4. 适用轴映射（详略得当）

| 资产类别 | 适用轴 | 轴数 |
|---------|-------|------|
| 后端核心 Critical | 正确性 + 可读性 + 架构 + 安全 + 性能 + 可观测性 + 可维护性 + 供应链 + 弹性 + 文档一致性 | 10 |
| 后端核心非 Critical | 正确性 + 可读性 + 架构 + 安全 + 性能 + 可维护性 + 可观测性 | 7 |
| 后端外围 cmd/* | 正确性 + 安全 + 可维护性 + 弹性 | 4 |
| 后端外围 migrations | 正确性 + 架构 + 可维护性 + 文档一致性 | 4 |
| 前端核心 Critical | 正确性 + 可读性 + 架构 + 安全 + 性能 + 可维护性 + 弹性 + 文档一致性 | 8 |
| 前端核心非 Critical | 正确性 + 可读性 + 架构 + 性能 + 可维护性 | 5 |
| 前端外围 | 正确性 + 安全 + 可维护性 | 3 |
| 测试资产 | 正确性 + 可读性 + 可维护性 + 可观测性 + 文档一致性 | 5 |
| 基础设施（K8s/Terraform/CI） | 正确性 + 安全 + 架构 + 可维护性 + 供应链 + 弹性 + 可观测性 + 文档一致性 | 8 |
| 监控配置 | 正确性 + 可观测性 + 可维护性 + 文档一致性 | 4 |
| Docker / 项目配置 | 正确性 + 安全 + 供应链 + 可维护性 | 4 |
| 文档资产 | 正确性 + 可读性 + 文档一致性 | 3 |

---

## 5. 子代理产出统一模板

```
## 资产 A-NNN: [名称]

### 基本信息
- 路径: [相对路径]
- 关键性: [Critical/High/Medium/Low]
- 适用轴: [列举]

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | | |
| 可读性 | | |
| 架构 | | |
| 安全 | | |
| 性能 | | |
| 可观测性 | | |
| 可维护性 | | |
| 供应链 | | |
| 弹性 | | |
| 文档一致性 | | |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|

### 整体健康度: 🟢/🟡/🔴 X.X/5
```

---

## 6. 5 个新轴审查 Checklist

### 轴 6: 可观测性（Observability）
- [ ] metrics 暴露：是否暴露 Prometheus metrics？
- [ ] 业务指标覆盖：关键业务指标是否覆盖？
- [ ] tracing：是否使用 OpenTelemetry？
- [ ] 跨边界 trace 传播：跨服务/goroutine 是否传播？
- [ ] 结构化日志：是否使用 slog？
- [ ] 日志级别：级别是否合理？
- [ ] 敏感字段脱敏：email/token 是否脱敏？
- [ ] 告警规则：是否有告警？基于 SLO？有通知渠道？

### 轴 7: 可维护性（Maintainability）
- [ ] 变更影响半径：修改功能需触及多少文件？
- [ ] 测试稳定性：是否 flaky？依赖外部状态？
- [ ] 技术债标记：TODO/FIXME/HACK 数量与跟踪
- [ ] 注释质量：解释"为什么"而非"做什么"？
- [ ] 误导性注释：是否存在？
- [ ] 公共 API 稳定性：是否有 deprecation 流程？

### 轴 8: 供应链安全（Supply Chain）
- [ ] 依赖固定：go.sum/package-lock.json 锁定？
- [ ] Go module 版本：是否使用特定版本？
- [ ] 镜像 digest pinning：Docker/K8s 是否用 @sha256:？
- [ ] 镜像签名：cosign 签名 + 验证？
- [ ] SBOM 生成：是否生成 SBOM？CI 校验？
- [ ] 依赖审计：govulncheck/npm audit 定期运行？CI 阻塞？

### 轴 9: 弹性（Resilience）
- [ ] 超时：HTTP/DB/Redis 调用是否设置超时？
- [ ] 重试：是否使用 internal/resilience/retry.go？
- [ ] 熔断：circuit breaker 覆盖关键依赖？
- [ ] 降级：Redis/DB 故障时优雅降级？
- [ ] 限流：是否有限流？区分用户/IP/全局？
- [ ] 背压：WS/goroutine 是否有背压？worker pool？

### 轴 10: 文档-代码一致性（Doc-Code Alignment）
- [ ] ADR 一致性：ADR 决策在代码中体现？
- [ ] OpenAPI 一致性：openapi.yaml 与实际 API 一致？
- [ ] 架构图一致性：architecture.md 与代码一致？
- [ ] README 一致性：README 与 Makefile/CI 一致？
- [ ] 注释 vs 实现：函数注释与实现一致？
- [ ] 误导性文档：是否存在？

---

## 7. 严重级别定义

| 级别 | 含义 | 处理要求 |
|------|------|---------|
| CRITICAL | 安全漏洞、数据丢失、功能完全损坏 | 必须立即修复 |
| REQUIRED | 逻辑错误、架构偏离、可读性严重问题 | 必须在合并前修复 |
| OPTIONAL | 风格偏好、轻微改进建议 | 可忽略 |
| FYI | 信息性备注 | 无需操作 |
