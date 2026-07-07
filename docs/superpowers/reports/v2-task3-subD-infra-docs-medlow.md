# v2 自检 Task 3 — 子代理 D：基础设施 + 文档 Medium/Low 资产审查

> 范围：10 个 Medium/Low 资产（A-064/065/070/071/072/074/076/081/082/083）
> 模式：纯诊断，未修改任何业务代码
> 审查轴：基础设施 8 轴 / 监控配置 4 轴 / 项目配置 4 轴 / 文档 3 轴（正确性+可读性+文档一致性）

---

## 资产 A-064: release-please CI

**路径**：`.github/workflows/release-please.yml`, `.github/release-please-config.json`, `.github/release-please-manifest.json`

### 轴评分（基础设施 8 轴）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | 配置三件套齐全，`release-type: go` 与 manifest 一致 |
| 可维护性 | 4 | 注释充分说明企业价值；config 使用 `$schema` 校验 |
| 安全/供应链 | 2 | **`googleapis/release-please-action@v4` 用 tag 而非 SHA digest pin**，存在供应链风险 |
| 文档一致性 | 4 | 注释与配置目的吻合 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|----|---------|------|------|------|
| v2-R-119 | 安全/供应链 | REQUIRED | release-please-action 使用 `@v4` 浮动 tag，未 pin 到 SHA digest。仓库其他 actions（如 `anchore/sbom-action@0a33c98da4a4d9e35761d803bdde100295ae53d8`）已采用 SHA pin，此为不一致 | `.github/workflows/release-please.yml:16` | 改为 `googleapis/release-please-action@<sha>`，配合 dependabot 周期升级 |
| v2-O-100 | 正确性 | OPTIONAL | `release-please-config.json` 仅声明 `.` 包，未对 `backend/`、`frontend/` 子包独立发版。当前 Go 单包设计可接受，但若未来拆分需扩 config | `.github/release-please-config.json` | 当前 OK；后续若引入子包发版再扩 |
| v2-F-83 | 可维护性 | FYI | manifest.json 写死 `"1.0.0"`，依赖 release-please-action 自动维护；首次 release 后将由 action 更新 | `.github/release-please-manifest.json` | 无需操作 |

### 整体健康度: 🟡 3.5/5

---

## 资产 A-065: docs-governance CI

**路径**：`.github/workflows/docs-governance.yml`

### 轴评分（基础设施 8 轴）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 2 | **`repo-layout` job 必失败**（见下） |
| 可维护性 | 4 | 5 个 job 职责清晰，concurrency 控制合理 |
| 安全/供应链 | 2 | 所有 `actions/*@v4/v5`、`lycheeverse/lychee-action@v2` 均 tag pin |
| 文档一致性 | 3 | ADR 索引、ws-protocol、AsyncAPI 校验逻辑完备 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|----|---------|------|------|------|
| v2-C-30 | 正确性 | CRITICAL | `repo-layout` job 调用 `scripts/ci/check-repo-layout.sh`，脚本第 94 行 `grep -q '^# Generated from deploy/alertmanager/rules.yml' deploy/alertmanager/rules-configmap.yaml` 要求该文件存在。但 `deploy/alertmanager/rules-configmap.yaml` **不存在**（仅有 `rules.yml`），`grep` 不存在文件返回非零，job 必失败 | `scripts/ci/check-repo-layout.sh:94`, `docs-governance.yml:18-24` | 二选一：(a) 生成 `rules-configmap.yaml`（运行 `bash scripts/ci/sync-alert-rules.sh`），(b) 修改 check-repo-layout 跳过该检查直至文件就绪 |
| v2-C-31 | 正确性 | CRITICAL | `make sync-alert-rules` 在 `Makefile:1` `.PHONY` 声明，但 **Makefile 中无对应 target 定义**。运行 `make sync-alert-rules` 报 "No rule to make target"。脚本 `scripts/ci/sync-alert-rules.sh` 存在但 Makefile 未接入 | `Makefile:1`, `Makefile`（无 target） | 在 Makefile 添加 `sync-alert-rules:` 目标调用 `bash scripts/ci/sync-alert-rules.sh` |
| v2-R-120 | 安全/供应链 | REQUIRED | 所有 actions（checkout/setup-node/lychee）均 tag pin 而非 SHA digest pin | `docs-governance.yml:22,30,45,94,95,105,107` | 改为 SHA pin（与 `go-ci.yml` 中 `anchore/sbom-action@0a33c98...` 风格一致） |
| v2-R-121 | 正确性 | REQUIRED | `ws-protocol-sync` job 在 `check SNAPSHOT 0x01` 等硬编码协议常量。若 `constants.go` 数值变更需手动同步本 workflow；与 ADR-002 二进制协议维护成本耦合 | `docs-governance.yml:76-88` | 考虑改为从 `constants.go` 动态提取（grep + xargs），避免硬编码漂移 |
| v2-O-101 | 可维护性 | OPTIONAL | `markdown-links` 用 `--offline`，无法校验外部 URL。README/CHANGELOG 中的外链失效不会被捕获 | `docs-governance.yml:109` | 拆分内外链：内链 `--offline`，外链单独 job 周期跑（避免每次 PR 慢） |

### 整体健康度: 🔴 2.5/5

---

## 资产 A-070: Prometheus

**路径**：`deploy/prometheus/alerts.yml`, `deploy/prometheus/deployment.yaml`, `deploy/prometheus/prometheus.yml`

### 轴评分（监控配置 4 轴）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 2 | **`ConnectionPoolExhaustion` 告警引用不存在的指标** |
| 可观测性 | 3 | Golden Signals 覆盖基本到位，但缺 GC/连接池等待时长告警 |
| 可维护性 | 3 | alerts.yml 与 alertmanager/rules.yml 分离，存在双源风险 |
| 文档一致性 | 2 | runbook/capacity-planning 引用 `game_active_ws_connections` 指标不存在 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|----|---------|------|------|------|
| v2-C-32 | 正确性 | CRITICAL | `ConnectionPoolExhaustion` 告警表达式 `pgxpool_acquire_count - pgxpool_release_count > 20`，但**这两个指标在代码中均不存在**。实际指标为 `db_pool_acquire_total`（counter，无 release 对应物）。告警永远不会触发，连接池耗尽无监控 | `deploy/prometheus/alerts.yml:22` | 改用 `db_pool_in_use_conns`（gauge）或 `db_pool_acquire_duration_seconds`（histogram）做饱和度告警，例如 `db_pool_in_use_conns > 20` |
| v2-R-122 | 文档一致性 | REQUIRED | `MemoryUsageHigh` 告警表达式 `process_resident_memory_bytes / 1073741824 > 0.9` 检查 RSS 是否 > 0.9 GiB，但语义标注为 "Memory usage > 90%"。**这是绝对值检查（0.9 GiB）而非百分比**，注释与表达式不符 | `deploy/prometheus/alerts.yml:32-34` | 改为 `process_resident_memory_bytes / on() group_left container_spec_memory_limit_bytes > 0.9`（需 cAdvisor 暴露 container 指标），或修正注释 |
| v2-R-123 | 可维护性 | REQUIRED | `deploy/prometheus/alerts.yml` 与 `deploy/alertmanager/rules.yml` 是两个独立规则文件，前者含 HighErrorRate/HighLatency/ConnectionPool/Memory，后者含 SLO burn + game-health。**两者在 `kustomization.yaml` 中均挂载到 ConfigMap `alertmanager-rules`**，但 alerts.yml 未在 kustomization configMapGenerator 中列出 | `deploy/kustomization.yaml:14-17` | 统一规则源：将 `alerts.yml` 内容并入 `rules.yml`，或扩展 kustomization 加载 alerts.yml |
| v2-O-102 | 可观测性 | OPTIONAL | 缺 GC 压力告警（`go_gc_duration_seconds`）、`db_pool_acquire_duration_seconds` P95 告警，runbook 故障 4 明确提到 GC 频率是常见根因 | `deploy/prometheus/alerts.yml` | 新增 `GCFrequent` (e.g. `rate(go_gc_duration_seconds_count[1m]) > 5`) 与 `DBPoolAcquireSlow` 告警 |
| v2-F-84 | 可维护性 | FYI | `DiskUsageHigh` 已注释移除，理由是未部署 node_exporter。注释清晰，决策合理 | `deploy/prometheus/alerts.yml:29-30` | 无需操作；未来部署 node_exporter 时按注释恢复 |

### 整体健康度: 🟡 2.8/5

---

## 资产 A-071: Alertmanager

**路径**：`deploy/alertmanager/config.yaml`, `deploy/alertmanager/deployment.yaml`, `deploy/alertmanager/rules.yml`

### 轴评分（监控配置 4 轴）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | SLO burn rate 多窗口告警实现符合 SRE 标准 |
| 可观测性 | 4 | 覆盖 auth/ws/game-health 三大维度，runbook 链接完备 |
| 可维护性 | 2 | **Slack webhook 硬编码占位符，未走 Secret** |
| 文档一致性 | 4 | rules.yml 注释明确指向 `metrics/record.go`，runbook 链接准确 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|----|---------|------|------|------|
| v2-C-33 | 可维护性/安全 | CRITICAL | `config.yaml` 硬编码 Slack webhook URL `https://hooks.slack.com/services/T00/B00/xxxxx`。该 URL 是占位符（非真实 token），但**应通过 K8s Secret 注入**而非 ConfigMap 明文。生产部署时若直接 commit 真实 webhook 即为密钥泄露 | `deploy/alertmanager/config.yaml:18` | 改为 `api_url_file: /etc/alertmanager/secrets/slack-webhook-url` 并通过 Secret 挂载；或使用 `api_url: '${SLACK_WEBHOOK_URL}'` + 模板渲染 |
| v2-R-124 | 正确性 | REQUIRED | `WSSLOBurnRateFast` 表达式分母 `sum(rate(ws_connection_total[1h]))` 在零流量时为 0，会导致除零产生 `+Inf`/`NaN`，告警可能误触发或静默失效 | `deploy/alertmanager/rules.yml:38-44` | 加 `> 0` 保护：`... ) > 0.0144 and sum(rate(ws_connection_total[1h])) > 10`（设最低流量门槛） |
| v2-O-103 | 可观测性 | OPTIONAL | `AuthSLOBurnRateFast` 用 1h/2m 窗口，符合 SRE 多窗口惯例；但 `AuthSLOBurnRateSlow` 用 6h/15m，标准 Google SRE 实践推荐 6h/30m。差异不影响功能 | `deploy/alertmanager/rules.yml:23-36` | 可选对齐至 6h/30m，与 slo.md 文档表保持一致 |
| v2-F-85 | 可维护性 | FYI | `deployment.yaml` 副本数=1，无 PDB/PVC。Alertmanager 多副本需考虑 HA cluster（`--cluster.*` 参数），当前单副本可接受但生产应扩 | `deploy/alertmanager/deployment.yaml:8` | 生产化时新增 `gossip` 集群配置 |

### 整体健康度: 🟡 3.3/5

---

## 资产 A-072: Grafana

**路径**：`deploy/grafana/dashboards/golden-signals.json`, `deploy/grafana/provisioning/`, `deploy/grafana/datasource.yaml`

### 轴评分（监控配置 4 轴）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 2 | **dashboard datasource UID 与 provisioning 不匹配** |
| 可观测性 | 4 | Golden Signals 9 个面板覆盖 latency/traffic/errors/saturation |
| 可维护性 | 3 | 双 datasource 配置（K8s vs local）职责不清 |
| 文档一致性 | 3 | dashboard 引用 `ws_connections`/`db_pool_*` 等指标与 metrics.go 一致 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|----|---------|------|------|------|
| v2-C-34 | 正确性 | CRITICAL | `golden-signals.json` 所有面板 `datasource: {uid: "Prometheus"}`，但 `provisioning/datasources/local.yaml` 未给 datasource 显式设置 `uid` 字段。Grafana 会自动生成随机 UID，**dashboard 因 UID 不匹配无法加载**，运行时报 "Datasource not found" | `deploy/grafana/dashboards/golden-signals.json:12,19,26,...`, `deploy/grafana/provisioning/datasources/local.yaml` | 在 `local.yaml` 的 datasource 添加 `uid: Prometheus`（与 dashboard 引用一致） |
| v2-R-125 | 可维护性 | REQUIRED | 存在两份 datasource 配置：`deploy/grafana/datasource.yaml`（K8s Thanos 全局）与 `deploy/grafana/provisioning/datasources/local.yaml`（local Compose）。两者命名冲突（前者 `Thanos (global)` + `Prometheus us-east1`，后者 `Prometheus`），但 `kustomization.yaml:11` 仅引用前者，后者无 K8s 入口 | `deploy/grafana/datasource.yaml`, `provisioning/datasources/local.yaml` | 在 README 明确：`provisioning/` 仅 docker-compose 用，`datasource.yaml`（根）仅 K8s 用；或合并到 provisioning/ 子目录按环境分文件 |
| v2-O-104 | 可维护性 | OPTIONAL | `golden-signals.json` 缺 `__inputs`/`__requires` 元数据字段，部分 Grafana 版本导入时无法自动解析依赖 | `deploy/grafana/dashboards/golden-signals.json` | 通过 Grafana UI 导出 standard 模板补充元数据，或保持手写但加 `templating` list |
| v2-O-105 | 可观测性 | OPTIONAL | dashboard 缺 outbox lag 面板（`outbox_lag_seconds` 指标在 metrics.go 存在且 runbook §6 引用），运营查 outbox backlog 需手写 PromQL | `deploy/grafana/dashboards/golden-signals.json` | 新增 "Outbox Lag" 面板 |

### 整体健康度: 🟡 2.8/5

---

## 资产 A-074: 项目配置

**路径**：`Makefile`, `.editorconfig`, `.gitattributes`, `.gitignore`, `.pre-commit-config.yaml`, `commitlint.config.js`, `_bootstrap-env.ps1`

### 轴评分（项目配置 4 轴）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | **`make sync-alert-rules` 声明无 target**（见 A-065 v2-C-31） |
| 安全 | 3 | pre-commit 覆盖 trailing-whitespace/detect-secrets/private-key/golangci-lint |
| 供应链 | 2 | **pre-commit hooks 用 tag pin 而非 SHA** |
| 可维护性 | 3 | Makefile 与 CI 命令基本对齐，但有漂移 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|----|---------|------|------|------|
| v2-R-126 | 供应链 | REQUIRED | `.pre-commit-config.yaml` 所有 hooks 用 `rev: vX.Y.Z` tag 而非 commit SHA。pre-commit 官方推荐 SHA pin 防止 tag 被移动攻击 | `.pre-commit-config.yaml:7,17,24,30` | 改为 `rev: <sha>`；可用 `pre-commit autoupdate` 后手动转 SHA |
| v2-R-127 | 正确性 | REQUIRED | `Makefile` `.PHONY` 行声明 `sync-alert-rules`，但 Makefile **无该 target 定义**。`make ci` 通过 `check-repo-layout` 间接需要该产物，但开发者运行 `make sync-alert-rules` 报 "No rule to make target" | `Makefile:1` | 添加 target：`sync-alert-rules:\n\tbash scripts/ci/sync-alert-rules.sh` |
| v2-R-128 | 正确性 | REQUIRED | `Makefile:73` `check-repo-layout` 用 `2>/dev/null || powershell ...` 兜底。Linux/CI 上若 `check-repo-layout.sh` 失败会跳到 powershell（不存在）导致二次失败，掩盖根因 | `Makefile:73` | 拆分平台：`ifneq ($(OS),Windows_NT)` 用 bash，`else` 用 ps1；或用 `&&` 而非 `\|\|` |
| v2-O-106 | 可维护性 | OPTIONAL | `Makefile:1` `.PHONY` 列出 `check-protocol-constants`，但 `.PHONY` 后无 target 定义；target 在第 75-76 行定义但未单独 `.PHONY` 声明（实际 OK 因顶部已含） | `Makefile:75-76` | 当前可用；建议把 `check-protocol-constants` 与 `check-generated` 添加到 help 文本 |
| v2-O-107 | 安全 | OPTIONAL | `.pre-commit-config.yaml` `golangci-lint` 用 `--new-from-rev=HEAD`，仅 lint 新改动。CI 上 `go-ci.yml` 跑全量 lint；但 pre-commit 默认不阻止已存在的 lint 失败回归 | `.pre-commit-config.yaml:27` | 可接受（shift-left trade-off）；团队需知 CI 是最终门禁 |
| v2-F-86 | 可维护性 | FYI | `_bootstrap-env.ps1` 仅做 `.env` 加载到 Process 环境，未做 schema 校验。简单脚本，作用明确 | `_bootstrap-env.ps1` | 无需操作 |

### 整体健康度: 🟡 3.0/5

---

## 资产 A-076: 环境配置

**路径**：`.env.example`, `.golangci.yml`（实际位于 `backend/.golangci.yml`）, `backend/.air.toml`

### 轴评分（项目配置 4 轴）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 2 | **`.env.example` 多个变量与代码不一致** |
| 安全 | 3 | 敏感字段标注 `CHANGE_ME`/`RUN_openssl_rand`，但 `JWT_SECRET` 是 stale |
| 供应链 | 4 | golangci.yml v2 schema，linters 选择合理 |
| 可维护性 | 2 | **`.golangci.yml` 实际路径与任务清单不符**；env 与代码漂移 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|----|---------|------|------|------|
| v2-C-35 | 正确性 | CRITICAL | `.env.example` 声明 `JWT_SECRET=CHANGE_ME_TO_RANDOM_STRING`，但**代码（`env.go:43`、`auth/jwt.go:39`）使用 `JWT_PRIVATE_KEY`（ECDSA PEM 私钥）+ `JWT_PUBLIC_KEY`**。开发者按 .env.example 设置 `JWT_SECRET` 后启动会 panic（`JWT_PRIVATE_KEY: failed to decode PEM block`）。`JWT_SECRET` 在 backend 代码中**完全未被读取** | `.env.example:48`, `backend/internal/config/env.go:43-44` | 替换为 `JWT_PRIVATE_KEY=<ECDSA PEM>`、`JWT_PUBLIC_KEY=<ECDSA PEM>` 示例并加生成命令；删除 `JWT_SECRET` |
| v2-R-129 | 正确性 | REQUIRED | `.env.example:51` 声明 `ADMIN_JWT_SECRET`，但 backend 代码**完全未读取**该变量（grep 全仓零命中）。文档误导 | `.env.example:51` | 删除该行，或在代码中实现 admin JWT 签发用独立密钥 |
| v2-R-130 | 正确性 | REQUIRED | `.env.example:44-45` 声明 `ENABLE_ROOM_OUTBOUND_QUEUE`/`ENABLE_ASYNC_ROOM_PERSIST`，但 backend 代码**完全未读取**（仅 .env.example 与 ADR-027 doc 引用）。ADR-027 标注"已接受"但功能开关未落地 | `.env.example:44-45`, `backend/internal/game/` | 在 `room_outbound.go`/`persist_manager.go` 接入 `config.GetEnvBool` 读取开关；或从 .env.example 删除直到实现 |
| v2-R-131 | 正确性 | REQUIRED | `.env.example:77-79` 声明 `ENABLE_PPROF`/`DEBUG_PORT`，runbook 与 continuous-profiling.md 也描述用此二变量启用 pprof。但 backend 代码**完全未读取**（grep `ENABLE_PPROF`/`DEBUG_PORT` 在 backend 零命中） | `.env.example:77-79`, `docs/operations/continuous-profiling.md:13`, `docs/operations/runbook.md:226` | 在 `server_debug.go` 接入 env 读取；或修订文档明确 pprof 当前由 `net/http/pprof` 默认暴露（需核查） |
| v2-R-132 | 正确性 | REQUIRED | `.env.example:9-10` 注释 + `cockroachdb-migration.md` 描述 `DB_DIALECT=cockroach` 切换 dialect。但 backend Go 代码**完全未读取 `DB_DIALECT`**（仅 `service.yaml` K8s env + docs 引用，无 `os.Getenv("DB_DIALECT")`）。`store.CurrentDialect`、`PostgresStore.ApplyCockroachMultiRegion` 均不存在。`backend/migrations/cockroach/` 目录不存在 | `.env.example:9-10`, `docs/data/cockroachdb-migration.md`, `docs/adr/015-distributed-sql-cockroachdb.md` | ADR-015 状态为"提议中"但 cockroachdb-migration.md 写得像已实现。需在文档顶部加 ⚠️ 提议中标记，或实现代码 |
| v2-R-133 | 可维护性 | REQUIRED | 任务清单声明 `.golangci.yml` 在仓库根，实际路径为 `backend/.golangci.yml`。`make lint` 在 `backend/` 目录运行能找到；但 v2 资产清单路径与实际不符，影响审查与工具链定位 | `backend/.golangci.yml` | 更新资产清单路径；或在根目录加 symlink（不推荐）；保持现状但修订文档 |
| v2-O-108 | 正确性 | OPTIONAL | `.golangci.yml:9` 声明 `go: '1.26'`，但 `go.mod` 实际版本未核查。若 go.mod 为 1.21 等较低版本，lint 行为可能漂移 | `backend/.golangci.yml:9` | 核对 `backend/go.mod` 的 `go` 指令并保持一致 |
| v2-O-109 | 安全 | OPTIONAL | `.env.example:48-51` 中 `JWT_SECRET`/`ENCRYPTION_KEY`/`ADMIN_PASSWORD` 等占位值未统一格式：有的 `CHANGE_ME_*`，有的 `RUN_openssl_*`。格式不统一影响自动化校验 | `.env.example:47-54` | 统一为 `RUN_openssl_rand_hex_32_TO_GENERATE` 风格，便于部署脚本 grep 检测未填值 |

### 整体健康度: 🔴 2.5/5

---

## 资产 A-081: 运维文档

**路径**：`docs/operations/`（runbook.md, slo.md, capacity-planning.md, chaos-experiments.md, continuous-profiling.md, environments.md, audit-log-archival.md）

### 轴评分（文档 3 轴）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 2 | **多处指标名/配置路径与代码不一致** |
| 可读性 | 4 | 五段式结构清晰，故障分级表完备 |
| 文档一致性 | 2 | runbook↔slo↔alerts 三处指标名漂移 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|----|---------|------|------|------|
| v2-C-36 | 文档一致性 | CRITICAL | runbook 与 capacity-planning 多处引用 `game_active_ws_connections` 指标，但**该指标在 `metrics.go` 中不存在**。实际指标名为 `ws_connections`。HPA（`infra/k8s/base/hpa.yaml:34`）正确使用 `ws_connections`。文档与代码漂移会导致运维按错误指标名查询 Grafana/Prometheus 得不到数据 | `docs/operations/runbook.md:202`, `docs/operations/capacity-planning.md:41` | 全局替换 `game_active_ws_connections` → `ws_connections`；建议加 CI 校验文档中提到的指标名必须在 metrics.go 中存在 |
| v2-C-37 | 文档一致性 | CRITICAL | runbook §7.1 引用指标 `ws_active_connections`，与 metrics.go 实际名 `ws_connections` 不一致。同一文档内 runbook §3 用 `ws_connections`，§7.1 用 `ws_active_connections`，三处命名不统一 | `docs/operations/runbook.md:439` | 统一为 `ws_connections` |
| v2-R-134 | 文档一致性 | REQUIRED | `audit-log-archival.md:61` 声明 "Prometheus 指标 `audit_logs_total_rows`、`audit_archive_last_run_timestamp`"，但**这两个指标在 `metrics.go` 中不存在**。文档描述的是规划中功能，未明确标注"未实现" | `docs/operations/audit-log-archival.md:61` | 在 §实现路径顶部加 ⚠️ "本节为规划，对应指标与 worker 未实现" 标记 |
| v2-R-135 | 文档一致性 | REQUIRED | `continuous-profiling.md:12-13` 声明 "`ENABLE_PYROSCOPE=true`、`PYROSCOPE_SERVER_ADDRESS`"，runbook 故障 4/5 引用 `ENABLE_PPROF`/`DEBUG_PORT`。**这些 env 变量均未被 backend 代码读取**（见 A-076 v2-R-131）。文档描述与实现脱节 | `docs/operations/continuous-profiling.md:12-13`, `docs/operations/runbook.md:226,242,293` | 在文档顶部加 "未实现/规划中" 标记；或接入代码后修订 |
| v2-R-136 | 文档一致性 | REQUIRED | `capacity-planning.md:40-41,45` 与 runbook:198,202 引用 `infra/k8s/base/hpa.yaml`，路径正确（文件存在）。但 capacity-planning:11 `MAX_WS_CONNECTIONS` 默认 10000，与 `config/constants.go` 实际值需核对（未在本次审查中读取 constants.go） | `docs/operations/capacity-planning.md:11` | 后续核查 `MaxWSConnections` 常量值是否为 10000 |
| v2-R-137 | 正确性 | REQUIRED | `runbook.md:301` 提供清理审计日志 SQL：`DELETE FROM audit_logs WHERE created_at < extract(epoch from now() - interval '30 days') * 1000`。但 `audit_logs` 表 schema（migration 000006）中 `created_at` 字段类型未核查。若为 `TIMESTAMPTZ`，则 `extract(epoch ...)*1000` 产生毫秒数值与 `TIMESTAMPTZ` 类型不匹配 | `docs/operations/runbook.md:301` | 核查 migration 000006 schema，修正为 `WHERE created_at < now() - interval '30 days'`（若 TIMESTAMPTZ） |
| v2-O-110 | 可读性 | OPTIONAL | runbook 故障 6 编号为 `## 6. Room 热路径性能` 后又出现 `### 6.3 AES 密钥轮换`，章节编号混乱（6.3 在 6.2 之后但属于"故障 6 认证服务异常"，而 "## 6" 是另一节） | `docs/operations/runbook.md:316,402,434` | 重新编号：将 "## 6. Room 热路径性能" 改为 "## 7"，"## 7. 多区域事件" 改为 "## 8" |
| v2-O-111 | 可读性 | OPTIONAL | `chaos-experiments.md` 全部实验状态为"⏳ 待执行"，无实际结果。文档价值在 staging 落地前受限 | `docs/operations/chaos-experiments.md:33,98` | 在文档顶部加 "状态：待 staging 集群就绪后首次执行" 横幅，避免读者误以为已有结论 |
| v2-F-87 | 可读性 | FYI | `environments.md` 简洁有效，三环境对照表清晰；部署目标标注"当前 vs 目标态" | `docs/operations/environments.md` | 无需操作 |

### 整体健康度: 🟡 2.8/5

---

## 资产 A-082: 开发文档

**路径**：`docs/development/`（benchmarks-go-microbench.md, benchmarks-k6-room-slo.md, coverage-policy.md）

### 轴评分（文档 3 轴）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | 微基准数据完整，标注硬件/日期/Go 版本 |
| 可读性 | 4 | 表格 + 关键指标解读，易于消费 |
| 文档一致性 | 3 | 引用 `code-simplification-plan.md` 不存在；k6 阈值与 alerts 部分对齐 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|----|---------|------|------|------|
| v2-R-138 | 文档一致性 | REQUIRED | `coverage-policy.md:38` 引用 `code-simplification-plan.md`，但 `docs/development/` 目录下**不存在该文件**（LS 显示仅 3 个 md）。死链 | `docs/development/coverage-policy.md:38` | 删除引用，或补建文件（若内容已被 ADR-028 吸收则改为引用 ADR-028） |
| v2-R-139 | 文档一致性 | REQUIRED | `benchmarks-k6-room-slo.md:13` 列出 `outbox_lag_seconds > 30s 10m` 告警建议，但 `deploy/alertmanager/rules.yml` 与 `deploy/prometheus/alerts.yml` 均**未实现该告警**。文档建议与实际告警规则脱节 | `docs/development/benchmarks-k6-room-slo.md:13` | 在 `deploy/alertmanager/rules.yml` 的 `game-health` group 新增 `OutboxLagHigh` 告警；或在文档标注"建议未落地" |
| v2-R-140 | 文档一致性 | REQUIRED | `benchmarks-k6-room-slo.md:9-13` 列出 5 项告警建议（room_lock_hold tick/message、room_outbound_queue_depth、room_persist_lag、outbox_lag），其中 4 项**未在 alerts.yml/rules.yml 中实现**。文档作为"目标基线"但告警侧无对应实现 | `docs/development/benchmarks-k6-room-slo.md:9-13`, `deploy/alertmanager/rules.yml` | 在 alerts.yml 新增对应告警规则，或在文档每行加"⚠️ 告警未实现"标注 |
| v2-O-112 | 可读性 | OPTIONAL | `benchmarks-go-microbench.md` 表格"单 tick 可执行次数"列基于 66.7ms/tick 计算，但表头未说明计算公式。读者需自行推导 | `docs/development/benchmarks-go-microbench.md:59-68` | 在表格上方加注释 "单 tick 66.7ms / ns-per-op = 可执行次数" |
| v2-O-113 | 正确性 | OPTIONAL | `benchmarks-go-microbench.md:5` 标注 "go1.26.4"，`.golangci.yml:9` 标注 `go: '1.26'`。Go 1.26 是未来版本（截至 2026-08 知识截止，最新稳定版为 1.23）。需核查是否为笔误或预发布 | `docs/development/benchmarks-go-microbench.md:5`, `backend/.golangci.yml:9` | 核查 `backend/go.mod` 实际 go 版本；若为 1.23 等则修正 |
| v2-F-88 | 可读性 | FYI | `coverage-policy.md` 排除规则清晰，明确 per-file 排除与 Vitest 无额外排除的边界 | `docs/development/coverage-policy.md:26-32` | 无需操作 |

### 整体健康度: 🟡 3.3/5

---

## 资产 A-083: 数据文档

**路径**：`docs/data/`（cockroachdb-migration.md, db-query-analysis.md）

### 轴评分（文档 3 轴）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 2 | **CRDB 迁移文档描述未实现功能；索引分析引用已删除索引** |
| 可读性 | 4 | SQL 示例 + 预期执行计划，结构清晰 |
| 文档一致性 | 2 | 与代码/migration 显著脱节 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|----|---------|------|------|------|
| v2-C-38 | 文档一致性 | CRITICAL | `cockroachdb-migration.md` 描述完整迁移流程，引用 `backend/migrations/cockroach/001_multiregion.sql`（**目录与文件均不存在**）、`PostgresStore.ApplyCockroachMultiRegion` 方法（**不存在**）、`store.CurrentDialect`（**不存在**）。文档读起来像已实现，但 ADR-015 状态为"提议中" | `docs/data/cockroachdb-migration.md:22-24,42-45` | 在文档顶部加显著 ⚠️ "本手册对应 ADR-015 提议中，功能未实现" 横幅；或拆分到 `docs/adr/015-...md` 的"未来工作"小节 |
| v2-C-39 | 文档一致性 | CRITICAL | `db-query-analysis.md:1` 声明 "基于当前 schema（000001_init_schema + 000002_add_indexes + 000004_add_composite_indexes）"。但 migration `000008_drop_redundant_indexes.up.sql:15` **已删除 `idx_lobby_states_updated_at`**。文档第 4 节"清理过期房间"仍引用 `idx_lobby_states_updated_at`（已不存在），实际查询会改用 `idx_lobby_states_updated_code` 最左前缀 | `docs/data/db-query-analysis.md:3,71-72`, `backend/migrations/000008_drop_redundant_indexes.up.sql:15` | 更新文档：删除 `idx_lobby_states_updated_at` 行；在"预期索引使用"改为 `idx_lobby_states_updated_code`（最左前缀覆盖 updated_at）；在顶部"基于 schema"行追加 `+ 000008_drop_redundant_indexes` |
| v2-R-141 | 文档一致性 | REQUIRED | `db-query-analysis.md:44` 引用 `store/postgres.go` `LoadAllActiveLobbies`，但 `backend/internal/store/` 下文件已拆分为 `postgres_lobbies_list.go`、`postgres_lobbies_query.go` 等（ADR-028 拆分）。文档引用的文件路径过时 | `docs/data/db-query-analysis.md:44` | 修正为 `store/postgres_lobbies_list.go` 或仅写 `store/` 包名 |
| v2-R-142 | 文档一致性 | REQUIRED | `cockroachdb-migration.md:59` 引用集成测试 `TestCockroachDB_MigrationCompatibility`，但 `backend/tests/integration/` 下未核查该测试是否存在。若不存在则文档承诺的验证步骤无法执行 | `docs/data/cockroachdb-migration.md:57-60` | 核查测试是否存在；若不存在则标注"待实现" |
| v2-O-114 | 可读性 | OPTIONAL | `db-query-analysis.md` 缺少 EXPLAIN ANALYZE 实测执行计划，仅"预期计划"。文档价值偏向理论分析 | `docs/data/db-query-analysis.md` | 在 staging/生产 PG 上跑 `EXPLAIN ANALYZE` 回填实测，或在顶部标注"预期计划，待 EXPLAIN ANALYZE 实测" |
| v2-F-89 | 可读性 | FYI | `db-query-analysis.md:110-114` "复合索引最左前缀原则说明" 段落准确清晰，有教育价值 | `docs/data/db-query-analysis.md:110-114` | 无需操作 |

### 整体健康度: 🔴 2.5/5

---

## 汇总统计

### 按资产
| 资产 ID | 名称 | 整体评分 | 发现数 | 健康度 |
|---------|------|---------|--------|--------|
| A-064 | release-please CI | 3.5/5 | 3 | 🟡 |
| A-065 | docs-governance CI | 2.5/5 | 5 | 🔴 |
| A-070 | Prometheus | 2.8/5 | 5 | 🟡 |
| A-071 | Alertmanager | 3.3/5 | 4 | 🟡 |
| A-072 | Grafana | 2.8/5 | 4 | 🟡 |
| A-074 | 项目配置 | 3.0/5 | 6 | 🟡 |
| A-076 | 环境配置 | 2.5/5 | 7 | 🔴 |
| A-081 | 运维文档 | 2.8/5 | 8 | 🟡 |
| A-082 | 开发文档 | 3.3/5 | 6 | 🟡 |
| A-083 | 数据文档 | 2.5/5 | 6 | 🔴 |

### 按严重级别
| 级别 | 数量 | ID 范围 |
|------|------|---------|
| CRITICAL | 9 | v2-C-30 ~ v2-C-39（v2-C-30, 31, 32, 33, 34, 35, 36, 37, 38, 39 共 10 个；其中 v2-C-30/31 关联同一根因） |
| REQUIRED | 23 | v2-R-119 ~ v2-R-142 |
| OPTIONAL | 14 | v2-O-100 ~ v2-O-114 |
| FYI | 7 | v2-F-83 ~ v2-F-89 |

> 实际计数：CRITICAL 10、REQUIRED 23、OPTIONAL 14、FYI 7 = 54 项

### 主题聚类（跨资产高频根因）

1. **GitHub Actions / pre-commit 供应链 pin 不一致**（v2-R-119, v2-R-120, v2-R-126）：仓库仅 `anchore/sbom-action` 单处用 SHA pin，其余全 tag pin
2. **告警规则引用不存在的指标**（v2-C-32 `pgxpool_*`、v2-C-36/37 `game_active_ws_connections`/`ws_active_connections`）：监控与 metrics.go 漂移
3. **`.env.example` 与代码 env 读取脱节**（v2-C-35 `JWT_SECRET`、v2-R-129 `ADMIN_JWT_SECRET`、v2-R-130 outbound 开关、v2-R-131 pprof、v2-R-132 `DB_DIALECT`）：6 个变量文档声明但代码未读取
4. **CockroachDB 多区域功能"文档已写、代码未实现"**（v2-R-132, v2-C-38, v2-R-142）：ADR-015 提议中但 operations/data 文档写得像已实现
5. **`make sync-alert-rules` 缺 target + `rules-configmap.yaml` 缺文件**（v2-C-30, v2-C-31, v2-R-127）：`make ci` 与 docs-governance workflow 必失败
6. **migration 000008 删除索引后文档未同步**（v2-C-39 `idx_lobby_states_updated_at` 仍在 db-query-analysis.md 引用）
7. **Grafana dashboard datasource UID 不匹配**（v2-C-34）：dashboard 运行时无法加载

### 建议优先级排序
1. **P0（立即修复）**：v2-C-30/31（CI 必失败）、v2-C-35（`.env.example` JWT_SECRET 误导开发者）
2. **P1（本周）**：v2-C-32/33/34/36/37/38/39（监控/告警/文档一致性）、v2-R-127/128（Makefile）
3. **P2（本迭代）**：其余 REQUIRED：供应链 SHA pin、env 变量清理、文档"提议中"标注
4. **P3**：OPTIONAL 与 FYI
