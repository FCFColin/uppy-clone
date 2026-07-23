# SLI / SLO / SLA 定义与 Error Budget

> 最后更新: 2026-06-24 | 适用: 多人网页气球飞行对战游戏
> SLO 将用户体验量化为可度量指标（SLI），据此计算 Error Budget 驱动开发速度与稳定性权衡。SLA 是面向客户的合同承诺，必须低于 SLO 以留缓冲。

---

## 1. 概念定义

| 术语 | 定义 |
|------|------|
| **SLI** (Service Level Indicator) | 对用户体验的可度量量化（如成功率、延迟） |
| **SLO** (Service Level Objective) | 对 SLI 设定的目标值（如 99.9% 成功率） |
| **SLA** (Service Level Agreement) | 对外合同承诺，违约需赔偿，必须 < SLO |
| **Error Budget** | 错误预算 = 1 - SLO，允许的不可用/失败配额 |

---

## 2. 核心用户旅程 SLI/SLO 定义

### 2.1 认证 (Authentication)

| 项目 | 定义 |
|------|------|
| **用户旅程** | 用户通过 Magic Link / Quick Play 完成登录认证 |
| **SLI** | 成功认证请求数 / 总认证请求数；认证请求延迟分布 |
| **SLO - 成功率** | 99.9%（30 天窗口） |
| **SLO - 延迟** | p99 < 500ms |
| **Error Budget** | 43.2 分钟/月（见下方计算） |
| **Prometheus 指标** | `auth_requests_total{status}`、`auth_request_duration_seconds` |

> **故障排查**：认证 SLO 异常时，参见 [`runbook.md`](./runbook.md) **故障 6**（refresh token / Resend 熔断 / AES 密钥轮换三类场景）。

### 2.2 房间创建 (Room Creation)

| 项目 | 定义 |
|------|------|
| **用户旅程** | 用户创建新游戏房间并获得 Room Code |
| **SLI** | 成功创建房间数 / 总创建尝试数；创建延迟分布 |
| **SLO - 成功率** | 99.5%（30 天窗口） |
| **SLO - 延迟** | p99 < 1s |
| **Error Budget** | 3.6 小时/月（见下方计算） |
| **Prometheus 指标** | `room_creation_total{status}`、`room_creation_duration_seconds` |

### 2.3 WebSocket 连接 (WebSocket Connection)

| 项目 | 定义 |
|------|------|
| **用户旅程** | 用户建立 WebSocket 连接加入游戏房间 |
| **SLI** | 成功建立 WS 连接数 / 总连接尝试数；连接建立延迟分布 |
| **SLO - 成功率** | 99.5%（30 天窗口） |
| **SLO - 延迟** | p99 < 2s |
| **Error Budget** | 3.6 小时/月（见下方计算） |
| **Prometheus 指标** | `ws_connection_total{status}` |

### 2.4 游戏消息延迟 (Game Message Latency)

| 项目 | 定义 |
|------|------|
| **用户旅程** | 游戏内玩家操作（Tap）消息从发送到广播的端到端延迟 |
| **SLI** | WebSocket 消息处理延迟分布 |
| **SLO - 延迟** | p99 < 100ms |
| **Error Budget** | 不适用（延迟型 SLO，无独立预算；超阈值即消耗认证/WS 预算） |
| **Prometheus 指标** | `ws_message_duration_seconds{msg_type}` |

---

## 3. Error Budget 计算（30 天窗口）

公式：Error Budget = (1 - SLO) × 30 天（43,200 分钟 = 720 小时）

| SLO | 允许失败率 | 30 天 Error Budget | 适用 |
|-----|-----------|-------------------|------|
| 99.9% | 0.1% | **43.2 分钟/月** | 认证 |
| 99.5% | 0.5% | **3.6 小时/月** | 房间创建 / WebSocket |

### Error Budget 使用原则

1. **预算未耗尽**：可承担风险——发布新功能、激进迁移
2. **预算耗尽**：冻结风险变更——只允许修复类发布，优先补测试与稳定性建设
3. **预算持续超支**：触发 SLO 评审，调低 SLO 或投入可靠性工程

### Burn Rate（燃烧速率）

消耗速率用于多窗口告警（见 `deploy/prometheus/alerts.yml`）：

| 窗口 | 倍率 | 含义 |
|------|------|------|
| 1h × 14.4 | 2% budget/hour | 快速燃烧：2m 持续即告警 |
| 6h × 6 | 1.67% budget/hour | 慢速燃烧：15m 持续即告警 |

---

## 4. SLA 定义（合同承诺）

| 项目 | 值 |
|------|-----|
| **SLA - 整体可用性** | **99.5%** |
| **违约赔偿** | 服务积分退还（具体见客户合同） |
| **测量窗口** | 滚动 30 天 |
| **测量方式** | Prometheus SLI 指标聚合 |

### SLA < SLO 的缓冲设计

| 指标 | SLO | SLA | 缓冲 |
|------|----------------|----------------|------|
| 认证成功率 | 99.9% | 99.5% | 0.4%（约 2.9 小时/月） |
| 房间创建成功率 | 99.5% | 99.5% | 0%（与 SLA 持平，需关注） |
| WebSocket 成功率 | 99.5% | 99.5% | 0%（已与 SLA 对齐，见 ADR-005/027） |

---

## 5. SLO 评审与调整

- **频率**：每季度评审 SLO 目标是否合理
- **调整触发**：连续 3 个月预算耗尽 → 评估降 SLO 或投入可靠性建设；连续 3 个月剩余 >50% → 评估提升 SLO 或加速发布；用户反馈与 SLI 背离 → 重审 SLI 定义
- **变更流程**：SLO 调整需 SRE + 产品 + 工程负责人三方签字，记录 ADR
