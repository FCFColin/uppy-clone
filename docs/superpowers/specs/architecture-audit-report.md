# 架构大方向体检报告

> 生成日期：2026-07-06 | 范围：第 1 层宏观大方向体检

---

## 总体评分

| 维度 | 评分 | 状态 |
|------|------|------|
| 1. 技术栈合理性 | 🟡 黄灯 | 整体合理，Redis 职责过载需关注 |
| 2. 模块化与内聚性 | 🟢 绿灯 | 良好，少数循环依赖可优化 |
| 3. 演进能力 | 🟡 黄灯 | 基础设施接口隔离良好，协议同步存在手动环节 |
| 4. 技术债务全景 | 🟢 绿灯 | 债务极少，文档一致性略有疏漏 |
| 5. 架构合规性 | 🟢 绿灯 | Clean Arch 规则严格执行，ADR 落地率 63% |

---

## 维度 1：技术栈合理性评估

### 1.1 Go 1.26 新特性利用

**发现**：293 个文件使用了 `interface{}` 空接口，代码库仍以 Go 1.18 前的惯用法为主。`slog` 已在 logging middleware 中使用，但泛型（尤其是 `cmp.Ordered`、类型集合）的利用几乎为零。

**评估**：🟡 黄灯 — 虽不影响运行，但存在可简化的模式：
- 多数 `interface{}` 出现在 `domain/events.go`（事件载荷）— **不适用泛型**
- 少量出现在 `store/` 的 `[]interface{}` 用于 QueryRow 参数 — **可替换为泛型切片**
- `slog` 采用率已足够，`errors.Join` 等标准库新特性未见使用

**建议**：
- 低优先级：泛型引入需谨慎，避免"为了泛型而泛型"
- 不需要系统性重构

### 1.2 pgx vs ORM 合理性

**发现**：`store/postgres.go` 约 954 行（ADR-019 已承认此问题），9 个 store 文件包含大量手写 SQL。查询复杂度中等，无 ORM 典型痛点（N+1、延迟加载）。

**评估**：🟢 绿灯 — ADR-019 决策仍然合理。ORM 在此场景无法提供足够收益，手写 SQL 对复杂游戏查询更可控。

### 1.3 Vanilla TS MPA 合理性

**发现**：前端 62 个源码文件 + 38 个测试文件（61.3% 测试覆盖率分布）。`game/` 模块 72 文件平铺，但最大文件仅 187 行。零生产依赖。

**评估**：🟢 绿灯 — ADR-018 决策仍然正确。Canvas 游戏 + 二进制 WebSocket 场景下，框架引入会带来 ~40KB runtime 开销却无实质收益。MPA 首屏加载优于 SPA。

**关注点**：前端 `game/` 存在 2 个循环依赖（`entry_flow ↔ entry_flow_dom` 和 `phase_sync → ui → ui_update → phase_sync`），均为低-中等严重度。

### 1.4 Chi Middleware 链复杂度

**发现**：典型受保护 API 路由经过 15 层中间件（10 全局 + 5 路由级）。两条重复中间件：

| 全局中间件 | 状态 | 建议 |
|-----------|------|------|
| `chi.Logger` | 🔴 冗余 | 与自定义 `RequestIDLogger` 功能重叠 |
| `chi.Recoverer` | 🔴 冗余 | 自定义 `Recovery`（slog 驱动）已覆盖 |
| `Tracing + Prometheus` | 🟡 可合并 | 两个都包装 ResponseWriter，可合并为单个 Obs 中间件 |

全局中间件从 10 → **8 层**可以安全缩减。

**评估**：🟡 黄灯 — 中间件层数本身不是问题（chi 的 O(1) 开销），但冗余和日志上下文多次变异增加了认知负担。

### 1.5 Redis 角色过载 🔴

**发现**：单一 Redis 实例承载了 10 个功能域：

| 域 | 负载类型 | 延迟敏感度 |
|----|---------|-----------|
| Pub/Sub 广播 | 高吞吐量 | 🔴 高 |
| 房间注册表 | 中等 O(N) 含 KEYS | 🔴 高 |
| 流（邮件/结果/发件箱）| 积压可能 | 🟡 中 |
| JWT 撤销 | 每次 API 检查 | 🔴 高 |
| 速率限制 | EVAL 脚本 | 🟡 中 |
| 大厅读取缓存 | SET/GET | 🟡 中 |
| 魔法链接令牌 | 低频率 | 🟢 低 |

**关键风险**：
1. `KEYS` 命令（`room_registry_store.go:90`）是 O(N) 阻塞命令，会暂停所有 Redis 操作
2. 游戏结果流积压时可能填满 Redis 内存，触发驱逐策略影响 JWT 检查等关键路径
3. Pub/Sub 广播（实时、延迟敏感）与流消费者（可能阻塞）共享同一连接

**评估**：🔴 红灯 — 这是所有维度中最突出的架构风险。

---

## 维度 2：模块化与内聚性评估

### 2.1 Backend 包分布

| 包 | 文件数 | 角色 |
|----|--------|------|
| `game/` | 63 | 核心游戏引擎 |
| `store/` | 49 | 数据访问层 |
| `handler/` | 36 | HTTP/WS 控制器 |
| `auth/` | 20 | 认证与授权 |
| `domain/` | 17 | 领域模型 |
| `middleware/` | 15 | HTTP 中间件 |
| `server/` | 14 | 组合根 + 路由 |
| `worker/` | 9 | 后台工作者 |

**发现**：分布健康，无包完成所有工作。`store/`（49 文件）略大但合理（PG + Redis 双后端）。

**交叉引用检查**：Handler → store/auth 的导入在**生产代码中为零**（Clean Architecture 严格执行）。ADR-028 已落地。

### 2.2 Frontend 前端模块化

**发现**：`game/` 72 文件平铺，通过命名前缀约定分组（`ws_*`、`renderer_*`、`state_*`、`ui_*`）。最大文件 187 行，`main.ts` 仅 2 个导入。

**循环依赖**：
- `entry_flow ↔ entry_flow_dom`：仅导入类型 + 一个回调函数，低严重度
- `phase_sync → ui (barrel) → ui_update → phase_sync`：通过 barrel 文件掩盖，中严重度

**评估**：🟢 绿灯 — 平铺结构在 72 文件时仍可维护。子包拆分不紧迫，建议先修复两个循环依赖。

### 2.3 Server 包 / 共享内核

**发现**：`internal/server/` 14 文件 — 恰当的组合根，无 God Object 迹象。`shared/` 仅包含 `data/nicknames.json`，未出现"倾倒"倾向。

**评估**：🟢 绿灯

---

## 维度 3：演进能力评估

### 3.1 基础设施接口隔离

**发现**：38 个角色接口（role interfaces），按消费者划分：

| 接口族 | 数量 | 覆盖 |
|--------|------|------|
| Redis 抽象 | 6 | 每个消费者定义自己的窄接口 |
| PostgreSQL 抽象 | 13 | 同上 |
| JWT/跨切面 | 6 | handler/auth/middleware 各自定义 |
| 游戏引擎 | 8 | hub/room/outbound/persist 各司其职 |

**交换代价评估**：将 Redis 替换为 KeyDB 需要更新约 6 个角色接口 + 1 个 store 实现。将 PostgreSQL 替换为 CockroachDB 需要更新 13 个接口实现 + 迁移 SQL 方言。

**评估**：🟢 绿灯 — 接口隔离设计良好，基础设施交换成本可控。

### 3.2 协议同步可靠性

**发现**：
- **物理常量**（帧率、冷却时间）：`go generate` 自动生成前端代码 ✅
- **消息类型常量**（`0x10`-`0x21`）：**手动维护** — 前后端 `protocol.ts` vs `constants.go` 无 CI 校验 ⚠️
- **两份 `constants.ts`**：`frontend/src/shared/constants.ts` 和 `frontend/src/shared/game/constants.ts` — 内容相同但所有者不明确 ⚠️

**评估**：🟡 黄灯 — 物理常量自动化做得很好，但消息类型的手动同步存在漂移风险。

### 3.3 新增上下文扩散半径预估

模拟新增"Chat"上下文：
1. `domain/chat.go` — 领域模型（1 文件）
2. `store/` — 数据持久化（2-3 文件）
3. `handler/chat.go` — HTTP/WS 控制器（1-2 文件）
4. `server/routes*.go` — 路由注册（修改 1 文件）
5. `frontend/` — 前端页面（1-2 文件）

**预估**：6-9 文件更改 — 合理的扩散半径，说明系统解耦良好。

---

## 维度 4：技术债务全景

### 4.1 死代码

**发现**：
- `TODO` 注释：2 处（极低）
- `FIXME` 注释：0 处
- `HACK` 注释：0 处
- `make deadcode` 二进制未构建，无法自动扫描
- 所有 handler 方法均在路由中被引用 — 无死 handler

**评估**：🟢 绿灯 — 代码库极其干净。

### 4.2 过度抽象

**发现**：
- 38 个接口对 25+ handler 函数，引入了一些"仅为测试存在"的接口
- `handler.TokenStore` 和 `auth.TokenStore` 接口方法几乎相同 — **存在重复**

**评估**：🟡 黄灯 — 接口数量偏多但非过度。`TokenStore` 重复可合并。

**值得注意**：`auth.redisClientProvider`（未导出接口）只在 `auth/middleware.go:27` 使用，仅定义 `Client()` 一个方法 — 属于"刚好够用"的窄接口，不视为过度。

### 4.3 测试债务

| 领域 | 测试覆盖评估 | 评分 |
|------|------------|------|
| Backend 单元测试 | 广泛（unit.out 覆盖所有包）| 🟢 |
| Backend 集成测试 | testcontainers + miniredis 混合模式 | 🟢 |
| Frontend 整体覆盖率 | 61.3%（38 test / 62 source） | 🟡 |
| Frontend game/ | ~30 测试文件 | 🟢 |
| Frontend shared/ | 8 测试文件覆盖 auth/cookie/toast | 🟡 |
| E2E | 11 spec 文件 × 6 并行矩阵 | 🟢 |

**评估**：🟢 绿灯 — ADR-018 承认的前端覆盖低问题已部分改善（61.3%），game/ 模块覆盖良好。shared/ 层（auth/fetch/session）可加强。

### 4.4 文档债务

| ADR | 状态 | 备注 |
|-----|------|------|
| ADR-013 (Cloud Run) | 🔄 已废弃 | 卸载到 GKE，但 ADR-000 承认仍有"过时 Cloud Run 措辞" |
| ADR-015 (CockroachDB) | ❌ 未实现 | 仍为 "提议中" 状态 |
| ADR-016 (多区域本地房间) | ❌ 未实现 | 仍为 "提议中" 状态 |
| ADR-022 (PII 加密) | ⚡ 部分 | `RotateKey()` 是 stub |
| ADR-020 (Embed Frontend) | ⚡ 部分 | Dockerfile 支持但无 `embed.FS` 引用 |

**评估**：🟢 绿灯 — ADR 文档总体与代码一致，少量漂移在可接受范围。**建议为 ADR-013 补充废弃标记**。

### 4.5 配置债务

**发现**：`.env.example` 中约 31 个环境变量。
- `ADMIN_JWT_SECRET` 出现 **两次**（重复定义）
- `TRUSTED_PROXY_CIDRS` 出现 **两次**
- 多数变量有文档注释和示例值
- Golangci-lint 无 env 相关检查

**评估**：🟢 绿灯 — 配置管理良好，两个重复问题易于清理。

---

## 维度 5：架构合规性评估

### 5.1 Clean Architecture 规则

**验证结果**：生产代码（非 test 文件）中，`handler/` 导入 `store` 或 `auth` 的次数 = **0**。所有依赖通过接口注入。ADR-028 声明成立。

**组合根**：`internal/server/` 是唯一的依赖注入点。`auth_service.go` 适配器将 auth 包函数式 API 转为 `handler.AuthService` 接口。

**评估**：🟢 绿灯 — 规则严格执行。

### 5.2 ADR 采纳率仪表盘

| 状态 | 数量 | 占比 |
|------|------|------|
| ✅ 已落地 | 17 | 63% |
| ⚡ 部分落地 | 8 | 30% |
| 🔄 已废弃/提议中 | 2 | 7% |
| ❌ 未执行 | 0 | 0% |

**核心发现**：
- 多区域相关（ADR-014/015/016）是唯一的"未落地"集群 — 属于目标架构、非当前实现
- ADR-028（Clean Architecture）是最新（2026-07-03）且已落地
- 无 ADR 被完全忽视或违反

### 5.3 环境一致性

**发现**：`.env.example` 包含所有 31 个变量并附有说明。Docker Compose 中 6 个服务镜像版本均固定。`infra/k8s/` 中 per-region overlay diff 为区域-specific 配置。

**评估**：🟢 绿灯

---

## 关键发现汇总 & 行动建议

### 🔴 高优先级

| # | 发现 | 维度 | 建议 |
|---|------|------|------|
| 1 | **Redis 单实例承载 10 个功能域** | 技术栈 | 优先将 Pub/Sub 广播隔离到独立 Redis 实例；用 `SCAN` 替换 `KEYS`；为流设置 `MAXLEN` |
| 2 | **消息类型常量手动同步** | 演进 | 为 `protocol.ts` 添加 CI 校验（如 diff 检查 Go 常量 vs TS 常量），或扩展 codegen 脚本 |

### 🟡 中优先级

| # | 发现 | 维度 | 建议 |
|---|------|------|------|
| 3 | Chi 两条冗余中间件 | 技术栈 | 删除 `chi.Logger` 和 `chi.Recoverer`，全局中间件从 10 层减至 8 层 |
| 4 | 前端 `game/` 两个循环依赖 | 模块化 | 抽取 `entry_flow_types.ts` 和直接引用 `ui_update` 修复 |
| 5 | ADR-013 废弃未标注 | 债务 | 给 ADR-013 文件添加 "已废弃" 标记和替代方案索引 |
| 6 | `interfaces{}` 未利用泛型 | 技术栈 | 仅在 store 的 Query 参数处考虑泛型化，不全局推广 |

### 🟢 低优先级

| # | 发现 | 维度 | 建议 |
|---|------|------|------|
| 7 | `ADMIN_JWT_SECRET` 和 `TRUSTED_PROXY_CIDRS` 重复 | 债务 | 清理 `.env.example` |
| 8 | `handler.TokenStore` 与 `auth.TokenStore` 方法重复 | 债务 | 考虑抽取公共接口或精简 -->
| 9 | `TracingMiddleware` + `PrometheusMiddleware` 可合并 | 技术栈 | 合为一个 ObservabilityMiddleware |
| 10 | `frontend/src/shared/game/constants.ts` 是重复 | 债务 | 确认是否可删除 |

---

## 附录：调查方法

| 方式 | 覆盖内容 |
|------|---------|
| 读 28 份 ADR | 逐份验证决策落地状态 |
| 读 6 个路由/中间件文件 | 中间件链完整追踪 |
| 读 12+ store 文件 | Redis 跨域使用分析 |
| 读 31 个前端 game/ 文件 | 模块熵分析 |
| 读 go.mod、golangci.yml、Dockerfile | 技术栈版本与工具链 |
| 读 Makefile、CI workflows | 工程基线 |
| grep import 分析 | Clean Architecture 合规验证 |
| `.env.example` 扫描 | 配置债务 |
