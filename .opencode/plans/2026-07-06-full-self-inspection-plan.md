# 全面自检计划 — 调研报告

> **性质：** 纯调研，不执行任何代码变更
> **日期：** 2026-07-06
> **状态：** ✅ 已完成

---

## 0. 项目画像（已有认知）

| 维度 | 现状 |
|------|------|
| **定位** | 多人网页气球对战游戏（Uppy 克隆） |
| **技术栈** | Go 1.26 / TypeScript 5.6 / Vite / Chi / pgx / gorilla-websocket / Canvas 2D |
| **后端包** | 28 个 internal 包，共 ~12,500 行代码 |
| **前端包** | ~50 个 .ts 文件（不含测试），Canvas 2D 渲染 |
| **数据库** | PostgreSQL 16 + Redis 7.4（状态/临时分离） |
| **部署** | Docker + K8s + Terraform (GCP) |
| **CI/CD** | GitHub Actions, 17 workflows |
| **文档** | 29 份 ADR, OpenAPI, AsyncAPI, 威胁模型 |

---

## 1. 架构自检（✅ 完成）

### 1.1 分层清晰度

| 检查项 | 结果 |
|--------|------|
| 后端包依赖图 | 28 个 internal 包构成有向无环图，无循环依赖 |
| `domain` 包依赖 | **零内部依赖** ✅ — 纯领域模型，符合 Clean Architecture |
| `handler` 包依赖 | 导入 audit, config, middleware, apierror, crypto, domain, metrics, protocol, telemetry, **game** |
| `auth` 包依赖 | 导入 domain, audit, config, validate, crypto, idgen, requestctx, apierror, metrics, slogctx, nicknames |
| `game` 包依赖 | 导入 domain, protocol, audit, config, metrics, nicknames, validate, idgen |
| `middleware` 包依赖 | 导入 domain, apierror, config, slogctx, metrics, requestctx, telemetry |
| `metrics` 包依赖 | 导入 `constants`（**已从 protocol 解耦** ✅） |
| `server` 是否为唯一组合根 | **是的** ✅ — `server_init.go` 中 `initHandlers()` / `initHub()` 完成所有依赖组装 |
| 分层违规 | **无严重违规** — 但 `handler_interfaces.go` 仍直接引用 `game.Room` / `game.RoomInfo` 类型 |

### 1.2 接口驱动程度（解耦计划执行情况）

#### 现状：**解耦计划已大部分执行**

| 计划任务 | 状态 | 证据 |
|----------|------|------|
| 1.6 拆分 config 为子接口 | ✅ 完成 | `config` 包已有多层配置结构 |
| 2.1 domain → validate 反转 | ✅ 完成 | `domain` 零依赖 `validate` |
| 2.2 metrics → protocol 常量抽取 | ✅ 完成 | `metrics` 导入 `constants` 非 `protocol` |
| 3.1 auth 定义接口 | ✅ 完成 | `auth` 包有 `UserDB`, `TokenStore`, `UserDataStore` 等接口 |
| 3.2 game 定义接口 | ✅ 完成 | `game` 包有 `Broadcaster`, `CacheStore`, `RoomRepository` 等接口 |
| 3.3 handler 全面依赖接口 | ✅ 完成 | `handler_interfaces.go` 定义 10 个接口 ✅ |
| 3.4 middleware → auth 解耦 | ✅ 完成 | middleware 不导入 auth 包 |
| 3.5 rbac → auth 解耦 | ✅ 完成 | `rbac.NewEnforcer()` 无参数 |

**Handler 接口清单（已存在）：**
- `UserStore`, `TokenStore`, `ConfigStore`, `AdminCache` — store 实现
- `JWTManager`, `RefreshTokenManager`, `AuthService` — auth 实现
- `GameService` — game/Hub 实现
- `LeaderboardStore` — store 实现

### 1.3 前端状态管理

| 检查项 | 结果 |
|--------|------|
| `state.xxx = ...` 直接写入（生产代码） | 仅 `reducer.ts` 中 `resetRound()` 函数（13 处赋值） |
| `state.xxx = ...` 直接写入（测试代码） | 各 test 文件中共计约 100+ 处（测试 setup 可接受） |
| `dispatch`/`getState`/`select` 使用 | **25+ 生产文件已使用** ✅ |
| `renderer.ts` DOM 查询 | **无** `classList`, `getElementById`, `querySelector` ✅ |
| Store 设计 | 存在共享可变 `state` 单例 — `store.ts` 通过 `Object.assign` 扁平应用 |

**问题点：** `reducer.ts` 中 `resetRound()` 和 `gameReducer` 的 `ADD_RIPPLE`/`SET_END_REASON` 分支直接 mutate 入参 `state` 对象而非返回新对象。这违反了标准的纯 reducer 模式，虽然在当前 `store.ts` 的实现下能工作，但存在引用一致性问题。

### 1.4 前端模块边界

| 检查项 | 结果 |
|--------|------|
| `shared/` 子目录化 | ✅ 已完成 — network/, game/, ui/, data/, assets/ |
| `game/constants.ts` 桶文件 | ✅ **已消除**（文件不存在） |
| `state.ts` / `websocket.ts` 桶文件 | ✅ **已消除**（文件不存在） |
| `ui.ts` ↔ `ui_update.ts` 交叉导入 | ✅ **已消除** |
| `entry_flow_dom.ts` 是否独立 | ✅ **仍存在**（解耦计划 1.5 要求合并，但文件仍存在） |
| `lifecycle.ts` / `window_events.ts` | ✅ **已从 main.ts 提取** |

---

## 2. 技术栈自检（✅ 完成）

| 检查项 | 结果 |
|--------|------|
| Go 版本 | 1.26.4 — 最新稳定版 ✅ |
| Node 版本 | 22.x — 最新 LTS ✅ |
| 关键 Go 依赖 | pgx v5.10, redis v9.21, otel v1.44 — 均为最新大版本 ✅ |
| 前端依赖 | vite ^6.0.0, vitest ^4.1.9, typescript ^5.6.0 — 最新 ✅ |
| Docker 镜像 | node:20.18.0-alpine3.20, golang:1.26-alpine — 均为 pin digest ✅ |
| **直接依赖数** | Go: 23 个 direct / 248 个 indirect（含 linter/tools） |
| 工具依赖膨胀 | `tools.go` 引入 golangci-lint + air + migrate，产生约 240+ 间接依赖 ⚠️ |
| Docker base 镜像 | node 20 可以升级至 22; golang 1.26 是最新 |

---

## 3. 代码质量自检（✅ 完成）

### 3.1 错误处理

| 检查项 | 结果 |
|--------|------|
| `panic` 在生产代码 | 5 处 — 均为初始化/启动阶段（JWT, crypto, rand）— 可接受 ✅ |
| 吞错模式 | 经扫描 `err != nil` 后 `_` 模式不多，可进一步确认 ⚠️ |
| Error wrapping | 使用 `fmt.Errorf("%w")` 模式 ✅ |

### 3.2 并发安全

| 检查项 | 结果 |
|--------|------|
| `sync.Mutex` 使用 | 5 处（audit, hub, room, persist_manager）— 合理 ✅ |
| `sync.RWMutex` 使用 | 4 处（hub, room ×2, persist_manager）— 合理 ✅ |
| `context.Background` 生产代码 | 少量且合理（启动/初始化）✅ |
| `context.TODO` | 0 处 — 所有 context 已迁移 ✅ |

### 3.3 代码长度

| 最大的包 | 文件数 | 总行数 |
|----------|--------|--------|
| `game` | 36 | 3,121 |
| `store` | 35 | 2,876 |
| `handler` | 23 | 1,597 |
| `auth` | 12 | 1,066 |
| `server` | 10 | 858 |
| `middleware` | 12 | 788 |

### 其他

| 检查项 | 结果 |
|--------|------|
| 后端 lint | golangci-lint 配置 50+ linter ✅ |
| 前端 lint | ESLint + typescript-eslint ✅ |
| TypeScript strict mode | **需确认** — tsconfig 未检查，可能有 `any` 隐式使用 ⚠️ |

---

## 4. 安全自检（✅ 完成）

| 检查项 | 结果 |
|--------|------|
| JWT 实现 | ECDSA (P256) 签名 + cT 声明 ✅ |
| Magic Link | 一次性令牌 + HMAC + 过期时间 ✅ |
| RBAC | 轻量策略表（ADR-026）✅ |
| Admin 鉴权 | 独立 JWT + LoginLock + bcrypt 密码 ✅ |
| 昵称验证 | 正则校验 + 拒绝列表 ✅ |
| WebSocket read limit | 4KB (WSReadLimit) ✅ |
| SQL 注入防护 | pgx 参数化查询 ✅ |
| CORS | 白名单校验，已覆盖 preflight ✅ |
| 安全头 | CSP, HSTS, X-Content-Type-Options, X-Frame-Options ✅ |
| Rate limiting | 按 endpoint + user + IP 分层限流 ✅ |
| `.env.example` | 全部敏感字段已占位，无硬编码密码 ✅ |
| Docker Compose 凭据 | dev-redis-secret 等多处 dev 凭据 — ⚠️ 仅用于本地开发 |
| 供应链安全 | cosign + SBOM + digest pin + 六重扫描 ✅ |

**安全缺口（与已有报告一致）：**
- `RotateKey()` 是 stub（无 AES 密钥轮换路径）
- `AUDIT_SECRET` 可回退到 `JWT_SECRET`（审计完整性耦合）

---

## 5. 测试自检（✅ 完成）

| 检查项 | 结果 |
|--------|------|
| 后端单元测试覆盖率 | 各包覆盖度文件存在（.out 文件 60+）— 需 CI 解析确认具体百分比 |
| 后端集成测试 | testcontainers 覆盖 PostgreSQL + Redis |
| 前端测试 | Vitest + jsdom |
| Property-based tests | `fast-check` + Go `testing/quick` ✅ |
| E2E 测试 | Playwright 覆盖 auth, gameplay, admin, security, reconnection, cross-page |
| 测试金字塔 | 单元 < 集成 < E2E 有良好分层 ✅ |

---

## 6. CI/CD 自检（✅ 完成）

| 检查项 | 结果 |
|--------|------|
| Workflow 数量 | 17 个（可能有约一半是零散未清理的）⚠️ |
| 关键 CI | ci.yml, go-ci.yml, cd.yml, security-scan.yml |
| CD 实现 | `build-and-deploy` job — 部署步骤仍是 `echo "placeholder"` 🔴 |
| Docker 镜像 | cosign 签名 + SBOM 生成 ✅ |
| 多环境部署 | 无 dev/staging/prod 分离 🔴 |
| 依赖自动更新 | Dependabot 已配置 ✅ |

---

## 7. 基础设施自检（✅ 完成）

| 检查项 | 结果 |
|--------|------|
| Terraform | GCP: Cloud SQL, Memorystore Redis, Secret Manager, GKE Workload Identity SA ✅ |
| K8s Kustomize | base/ (hpa, pdb, service, redis ×2, region-config) + overlays/us-east1 + global/ |
| 多区域部署 | 三个 region overlay 存在但 **从未验证** 🔴 |
| 可观测性 | Prometheus + Grafana + Alertmanager + OTel + 持续 profiling ✅ |
| 健康检查 | Docker compose healthcheck ✅ |
| 资源限制 | Docker compose resources: 0.25-1.0 CPU, 128-512MB RAM ✅ |

---

## 8. 运维自检（✅ 完成）

| 检查项 | 结果 |
|--------|------|
| `make` 命令 | dev/test/build/run/migrate/seed/bench/audit/clean/e2e 全部 ✅ |
| Graceful Shutdown | SIGTERM → 关闭房间 → drain broadcast → 60s timeout ✅ |
| PostgreSQL 备份 | Cloud SQL 自动备份已开启 ✅ |
| Redis 持久化 | 状态 Redis: AOF + RDB; 临时 Redis: allkeys-lru 无持久化 ✅ |
| 迁移策略 | golang-migrate, up/down 幂等性 CI 已验证 ✅ |

---

## 9. 文档自检（✅ 完成）

| 检查项 | 结果 |
|--------|------|
| ADR 数量 | 29 份（000-029），结构完整，格式统一 ✅ |
| ADR 引用网络 | 形成跨文档引用链 ✅ |
| API 文档 | OpenAPI + AsyncAPI + ws-protocol ✅ |
| 安全文档 | 威胁模型 + 自检清单 + 日志策略 ✅ |
| 运维文档 | SLO, Runbook, 环境说明, 容量规划 ✅ |
| 开发文档 | 覆盖策略, 基准测试 ✅ |
| 数据文档 | DB 查询分析, CockroachDB 迁移 ✅ |
| 架构文档 | 架构说明 + 多区域拓扑 ✅ |
| **README** | **不存在 — 根目录无 README.md** 🔴 |
| CONTRIBUTING | 存在 ✅ |
| CHANGELOG | 存在 ✅ |

---

## 10. 汇总报告

### 总体评分

| # | 维度 | 评分 | 关键结论 |
|---|------|------|---------|
| 1 | 架构设计 | 🟢 良好 | 解耦计划已大部分完成，Clean Architecture 形式完备 |
| 2 | 技术栈 | 🟢 良好 | 版本前沿，选型合理，间接依赖偏多 |
| 3 | 代码质量 | 🟢 良好 | Go 实践规范，前端 reducer 有轻微反模式 |
| 4 | 安全性 | 🟢 良好 | 纵深防御完善，密钥轮换缺少实现 |
| 5 | 测试 | 🟢 良好 | 覆盖率文件存在，金字塔结构好 |
| 6 | CI/CD | 🟡 需行动 | CD 流水线是占位符，17 个 workflow 需清理 |
| 7 | 基础设施 | 🟢 良好 | Terraform + K8s 清单完整，多区域未验证 |
| 8 | 运维 | 🟢 良好 | Graceful Shutdown、备份、迁移流程完整 |
| 9 | 文档 | 🟢 良好 | ADR 完整，但根目录 README 缺失 |

### 优先级行动项（从已有报告 + 本次调查）

| 优先级 | 项 | 影响 |
|--------|----|------|
| 🔴 P0 | CD 流水线实现（当前是 placeholder） | 无法自动部署到生产 |
| 🔴 P0 | 多区域 GKE 部署验证 | 核心架构承诺未验证 |
| 🟡 P1 | `RotateKey()` 实现 | 密钥轮换缺失 |
| 🟡 P1 | Workflow 清理（17 个 → 精简） | CI 维护成本 |
| 🟡 P1 | 根目录 README 缺失 | 新开发者入手困难 |
| 🟡 P1 | Redis 职责过载（10 功能域） | 单一故障点 |
| 🟢 P2 | `reducer.ts` 纯函数改造 | 可维护性 |
| 🟢 P2 | indriect go deps 优化 | 构建速度 |
| 🟢 P2 | `entry_flow_dom.ts` 合并入 `entry_flow.ts` | 解耦遗留项 |

### 现有参考

本次调研的宏观战略判断与 `docs/superpowers/reports/2026-07-06-comprehensive-self-audit.md`（841 行）一致。本文档聚焦于**代码级颗粒度发现**，互补而非替代。推荐阅读该报告获取 12 维度的详细战略分析。
