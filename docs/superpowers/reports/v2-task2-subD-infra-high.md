# v2 自检 Task2 子代理 D：基础设施 High 关键性资产审查

> 生成日期：2026-07-08
> 审查范围：A-063 (security-scan CI) / A-069 (K8s global) / A-073 (Docker) / A-034 (集成测试)
> 适用轴：基础设施 8 轴（正确性+安全+架构+可维护性+供应链+弹性+可观测性+文档一致性）；A-073 为 4 轴；A-034 为 5 轴（测试轴）
> 审查模式：纯诊断，未修改任何业务代码
> 发现 ID 区间：CRITICAL v2-C-15+ / REQUIRED v2-R-60+ / OPTIONAL v2-O-45+ / FYI v2-F-38+

---

## 关键上下文（影响评分）

1. **扫描工具实际分布**：`security-scan.yml` (A-063) 仅为**补充性日扫描**（npm audit + govulncheck）。完整的 SAST/secret/容器/SBOM/cosign 链路在 `go-ci.yml` 中：gitleaks、detect-secrets、Trivy、CodeQL、govulncheck、golangci-lint、docker-pin-check、license-check、cosign sign、anchore SBOM。`ci-cd.yml` deploy 阶段做 cosign verify。因此 A-063 的"覆盖窄"是相对其自身职责的判断，并非项目整体缺工具。
2. **v1 基线报告状态**：`docs/superpowers/reports/2026-07-07-full-self-inspection-report.md` 经 Glob 实测**不存在**（reports/ 仅含 v2-asset-inventory.md 与 v2-task1-results.md）。task1 结果中 v2-C-04 标注"误报，报告实际存在"本身有误，见 v2-F-46。
3. **HPA 指标链路**：v2-task1 已识别 v2-C-03（prometheus-adapter 缺失），指派给本子代理 D，对应本报告 v2-C-15。

---

## 资产 A-063: security-scan CI

### 基本信息
- 路径: `.github/workflows/security-scan.yml`
- 关键性: High
- 适用轴: 正确性 + 安全 + 架构 + 可维护性 + 供应链 + 弹性 + 可观测性 + 文档一致性（8 轴）

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | 语法正确，govulncheck `continue-on-error: false` 真正阻塞；npm audit `--audit-level=high` 合理 |
| 安全 | 3 | 仅覆盖 npm/go CVE 两类；无 secret 扫描/SAST/容器扫描的定期调度（这些仅在 PR 触发的 go-ci.yml 中） |
| 架构 | 4 | schedule + workflow_dispatch 分离合理，与 go-ci.yml 的 PR 门控互补 |
| 可维护性 | 4 | 29 行极简，易读；但缺注释说明"为何只跑这两项" |
| 供应链 | 2 | 所有 GitHub Actions 用浮动标签（@v4/@v5/@v1），无 @sha256 digest pin |
| 弹性 | 4 | 每日 06:00 UTC 定期扫描提供持续覆盖 |
| 可观测性 | 3 | 无失败通知（Slack/issue/邮件），失败仅体现在 workflow run 状态 |
| 文档一致性 | 4 | 与 go-ci.yml 职责划分清晰 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-60 | 供应链 | REQUIRED | 4 个 GitHub Actions（checkout@v4/setup-go@v5/setup-node@v4/govulncheck-action@v1）均未 digest pin，存在供应链劫持风险（与 go-ci.yml 同样的项目级问题） | security-scan.yml:13,14,18,28 | 改为 `actions/checkout@<sha256>`，并用 Dependabot 维护；全仓 CI 统一 digest pin |
| v2-R-61 | 安全/可观测性 | REQUIRED | 计划扫描仅跑 npm audit + govulncheck，缺 gitleaks/detect-secrets/Trivy/CodeQL 的定期重扫。合并后新披露的容器镜像 CVE、历史泄露的密钥、SAST 新规则无法被日扫描捕获 | security-scan.yml:24-29 | 在本工作流或新增 daily 工作流中定期调度 gitleaks（全历史）、Trivy（镜像）、CodeQL；或为 go-ci.yml 增加 schedule 触发器 |
| v2-O-45 | 安全 | OPTIONAL | 未声明 `permissions:` 块，继承仓库默认权限（可能过宽） | security-scan.yml:8 | 显式 `permissions: { contents: read }` 最小权限 |
| v2-O-46 | 可观测性 | OPTIONAL | 扫描失败无通知渠道，依赖人工查看 run 状态 | security-scan.yml（全局） | 增加 Slack/issue 自动创建 on failure |
| v2-F-38 | 供应链 | FYI | SBOM 生成与 cosign 签名在 go-ci.yml build-push 中已实现（anchore/sbom-action + cosign sign），ci-cd.yml deploy 阶段做 cosign verify，本工作流为补充 CVE 扫描，职责清晰 | go-ci.yml:326-339, ci-cd.yml:179-187 | — |

### 整体健康度: 🟡 3.6/5
窄而专的补充扫描器，自身实现正确但供应链 pinning 与覆盖广度不足。

---

## 资产 A-069: K8s global

### 基本信息
- 路径: `infra/k8s/base/` (hpa.yaml, kustomization.yaml, pod-disruption-budget.yaml, service.yaml, redis.yaml, redis-ephemeral.yaml, region-config.yaml) + `infra/k8s/global/` (network-policy.yaml, multicluster-ingress.yaml) + `infra/k8s/overlays/{us-east1,europe-west1,asia-southeast1}/kustomization.yaml`
- 关键性: High
- 适用轴: 正确性 + 安全 + 架构 + 可维护性 + 供应链 + 弹性 + 可观测性 + 文档一致性（8 轴）

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | HPA 自定义指标 ws_connections 链路断裂（无 prometheus-adapter），按 WS 连接数自动伸缩实际不生效 |
| 安全 | 3 | balloon-game 容器安全上下文完整（nonroot/65532/readOnlyRootFilesystem/cap drop ALL）；但两个 Redis StatefulSet 缺 securityContext；命名空间未声明 PSS restricted |
| 架构 | 4 | StatefulSet+headless Service 支持 owner 反向代理寻址；base/overlays/global 三层分离清晰；多区域 MCI 拓扑合理 |
| 可维护性 | 4 | 注释充分解释"为什么"（ADR 引用、权衡）；overlays 用 JSON Patch 较清晰 |
| 供应链 | 2 | balloon-game 镜像在 base 与 3 个 overlay 中均为占位符 `__IMAGE_TAG__`（ci-cd 用 sed 替换为 git sha），无 digest pin；Redis 镜像已 digest pin（例外） |
| 弹性 | 4 | HPA 行为调优（scaleDown 300s 稳定窗口、Min 策略）、PDB maxUnavailable:1、startup/liveness/readiness 三探针、terminationGracePeriodSeconds:60 优雅排空 |
| 可观测性 | 4 | balloon-game 有 prometheus scrape 注解 + region 标签；Redis 无 metrics 暴露（可观测盲区） |
| 文档一致性 | 4 | 与 ADR-005/013/014/015/016 引用一致；hpa.yaml 注释明确指出需 prometheus-adapter 但配置缺失 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-C-15 | 弹性/正确性 | CRITICAL | HPA 依赖自定义指标 `ws_connections`（ Pods 类型指标），但全仓库无 prometheus-adapter / APIService 配置将该自定义指标暴露给 HPA。当前 HPA 仅能基于 CPU 伸缩，WS 连接数维度失效，可能导致单实例连接超限（舱壁上限 10000）或不必要扩容 | hpa.yaml:31-38（注释 L10 已自述依赖 prometheus-adapter） | 新增 `infra/k8s/base/prometheus-adapter.yaml`（APIService + Service + adapter 规则映射 ws_connections），或在告警/runbook 中明确标注该指标链路未就绪；与 v2-task1 v2-C-03 同一问题 |
| v2-R-62 | 供应链 | REQUIRED | balloon-game 镜像 `gcr.io/PROJECT_ID/balloon-game` 在 base 中无 tag/digest，3 个 overlay 用 `__IMAGE_TAG__` 占位符，ci-cd.yml 用 sed 替换为 git sha（标签而非 digest）。ci-cd.yml 注释自身承认应改为 digest pinning | service.yaml:91, overlays/*/kustomization.yaml:13, ci-cd.yml:200-207 | CI 改用 `kustomize edit set image ...@sha256:<digest>`，overlay 的 images 段注入 digest；cosign verify 已就位，配合 digest pin 形成完整 SLSA L3 |
| v2-R-63 | 安全 | REQUIRED | redis 与 redis-ephemeral 两个 StatefulSet 的容器均缺 `securityContext`（无 runAsNonRoot/readOnlyRootFilesystem/capabilities drop ALL）。虽然 redis 镜像默认非 root，但未显式声明不满足纵深防御与 K8s restricted profile | redis.yaml:42-43, redis-ephemeral.yaml:42-43 | 为 redis 容器补 `securityContext: { runAsNonRoot: true, runAsUser: 999, readOnlyRootFilesystem: true, capabilities: { drop: [ALL] } }`（注意 redis 写 /data 需挂载 emptyDir） |
| v2-R-64 | 安全 | REQUIRED | namespace `balloon-game` 未声明 Pod Security Standards admission 标签。Dockerfile 注释提到"K8s restricted profile"，但 namespace 缺 `pod-security.kubernetes.io/enforce: restricted` 注解，restricted 约束未强制 | network-policy.yaml:13（namespace 定义处）, overlays（namespace: balloon-game） | 在 namespace 定义上加 `pod-security.kubernetes.io/enforce: restricted`、`audit: restricted`、`warn: restricted` 注解；需新增 namespace manifest 或在 overlay 中补 |
| v2-O-47 | 可观测性 | OPTIONAL | Redis（stateful + ephemeral）无 prometheus.io/scrape 注解，也无 exporter sidecar，Redis 指标（内存/连接/命中率）不可观测 | redis.yaml:36-39, redis-ephemeral.yaml:36-39 | 部署 redis-exporter sidecar 或用 Memorystore 自带监控；至少暴露基础指标 |
| v2-F-39 | 安全 | FYI | network-policy.yaml 实现良好：default-deny-egress + allow-dns + allow-db-redis-egress(5432/6379) + allow-health-check-probes + internal-proxy 仅同命名空间 Pod 互访，纵深防御到位 | network-policy.yaml（全文） | — |
| v2-F-40 | 弹性 | FYI | PDB maxUnavailable:1 与 HPA minReplicas:3 协同，保证至少 2 副本可用；HPA behavior 双策略（Percent+Pods）+ 长稳定窗口适配 WS 长连接排空，弹性设计成熟 | pod-disruption-budget.yaml:8, hpa.yaml:39-57 | — |
| v2-F-41 | 供应链 | FYI | Redis 镜像 `redis:7.4.0-alpine3.20@sha256:c35af3...` 在 redis.yaml 与 redis-ephemeral.yaml 中均已 digest pin，与 Dockerfile 风格一致 | redis.yaml:44, redis-ephemeral.yaml:44 | — |

### 整体健康度: 🔴 3.0/5
架构与弹性设计成熟，但 HPA 指标链路断裂（CRITICAL）与镜像 digest pin 缺失构成生产风险。

---

## 资产 A-073: Docker

### 基本信息
- 路径: `Dockerfile`, `docker-compose.yml`, `.dockerignore`
- 关键性: High
- 适用轴: 正确性 + 安全 + 供应链 + 可维护性（4 轴）

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | Dockerfile 三阶段构建正确，npm ci 可复现，CGO_ENABLED=0 静态二进制，USER nonroot；compose depends_on 条件 + healthcheck + profiles 正确 |
| 安全 | 4 | 基础镜像全 digest pin（distroless nonroot），readOnlyRootFilesystem（K8s 层），127.0.0.1 绑定 Redis；弱项为 compose 默认 dev 密码与 cockroach --insecure（均 dev/可选 profile） |
| 供应链 | 3 | Dockerfile 优秀（3 个基础镜像全 digest pin）；docker-compose.yml 7 个镜像全未 digest pin（dev 用途缓解严重性） |
| 可维护性 | 3 | docker-compose.yml `name: uppy-clone` 为模板残留，与项目名 balloon-game 不一致，误导性 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-65 | 供应链 | REQUIRED | docker-compose.yml 全部 7 个镜像未 digest pin：postgres:16.4-alpine3.20、redis:7.4.0-alpine3.20(×2)、prom/prometheus:v2.53.0、prom/alertmanager:v0.27.0、grafana/grafana:11.1.0、cockroachdb/cockroach:v23.2.6。go-ci.yml 的 docker-pin-check 仅强制 Dockerfile digest + 拒绝 :latest，未要求 compose digest | docker-compose.yml:47,65,79,89,99,108,125 | 为 compose 镜像补 `@sha256:` digest；或扩展 `scripts/ci/check-docker-digests.sh` 覆盖 compose（dev 上下文下可降级为 OPTIONAL，但任务明确要求检查） |
| v2-R-66 | 可维护性/文档一致性 | REQUIRED | docker-compose.yml 顶层 `name: uppy-clone` 为模板残留，项目实际为 balloon-game（多人网页游戏）。误导维护者，与 K8s/Dockerfile 命名不一致 | docker-compose.yml:1 | 改为 `name: balloon-game`；全仓检查是否还有 uppy-clone 残留（backend go.mod module 路径亦为 `github.com/uppy-clone/backend`，属历史遗留，本审查不改业务代码） |
| v2-O-48 | 可维护性 | OPTIONAL | Dockerfile 无 HEALTHCHECK 指令。K8s 层有 startup/liveness/readiness 探针覆盖，但 docker-compose 单独运行 app 服务时无健康检查，depends_on 无法用 service_healthy | Dockerfile（缺），docker-compose.yml:12-13（app 无 healthcheck） | 可在 Dockerfile 加 `HEALTHCHECK` 或在 compose app 服务加 healthcheck（curl /health/live） |
| v2-O-49 | 供应链 | OPTIONAL | Dockerfile 构建阶段未内联生成 SBOM；SBOM 在 go-ci.yml build-push 推送后用 anchore/sbom-action 生成。本地 `docker build` 产出的镜像无 SBOM 附件 | Dockerfile, go-ci.yml:330-335 | 可接受现状（CI 已产 SBOM）；如需本地 SBOM 可加 `syft` 步骤 |
| v2-F-42 | 供应链 | FYI | Dockerfile 实现优秀：node/golang/distroless 三基础镜像全 digest pin，多阶段构建，npm ci 可复现，CGO_ENABLED=0 静态，USER nonroot:nonroot，COPY --chown 保留所有权。达到 SLSA L2 基线 | Dockerfile:3,11,20 | — |
| v2-F-43 | 可维护性 | FYI | .dockerignore 覆盖完善：排除 .git/.env/docs/tests/infra/.github/scripts，减少 context 与攻击面；保留 backend/migrations/README.md 例外 | .dockerignore（全文） | — |
| v2-F-44 | 安全 | FYI | compose 中 Redis 端口绑定 127.0.0.1（6379/6380）、cockroach --insecure 仅在 `cockroach` profile 下启用、app 资源 limits/reservations 配置齐全，dev 安全实践合理 | docker-compose.yml:68,82,125-127,38-45 | — |

### 整体健康度: 🟡 3.5/5
Dockerfile 质量优秀，docker-compose.yml 供应链 pin 与命名残留是主要扣分项。

---

## 资产 A-034: 集成测试

### 基本信息
- 路径: `backend/tests/integration/` (admin_api_test.go, auth_full_flow_test.go, game_room_lifecycle_test.go, outbox_test.go, postgres_gdpr_lobby_test.go, postgres_store_test.go, rate_limiter_test.go, redis_redis_store_test.go, ws_handler_test.go)
- 关键性: High
- 适用轴: 正确性 + 可读性 + 可维护性 + 可观测性 + 文档一致性（5 轴，测试轴）

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | 覆盖广，但 TestPostgresStore_AnonymizeUser 断言逻辑错误（&& 应为 ||），GDPR 匿名化验证形同虚设；TestRateLimiter_ConcurrentRequests 设计偏弱 |
| 可读性 | 4 | 命名清晰（TestXxx_Scenario），表驱动 + 子测试组织良好；helper（newAdminTokenString/wsDial）抽取合理 |
| 可维护性 | 4 | testcontainers + miniredis 混合策略平衡真实性与速度；graceful skip（testing.Short + 容器不可用 skipf）保障稳定性 |
| 可观测性 | 3 | 测试内无 metrics/tracing 断言；失败信息基本够用但部分用 %+v 笼统打印 |
| 文档一致性 | 4 | postgres_store_test.go 顶部注释解释 testcontainers 取舍；与 ADR 一致；module 名 uppy-clone 为历史遗留（非测试本身问题） |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-67 | 正确性 | REQUIRED | `TestPostgresStore_AnonymizeUser` 断言 `if got.Email != "" && got.Nickname == "GDPRUser"` 用 `&&`，仅当 Email 非空**且** Nickname 仍为原值时才失败。若 AnonymizeUser 只清空 Email 而漏改 Nickname（或反之），测试会误通过，GDPR 匿名化验证形同虚设 | postgres_gdpr_lobby_test.go:37 | 改为 `if got.Email != "" \|\| got.Nickname == "GDPRUser"`（任一字段未匿名即失败）；并断言 Nickname 为匿名占位值 |
| v2-O-50 | 供应链 | OPTIONAL | testcontainers 使用的镜像 `postgres:16-alpine`、`redis:7-alpine`（testutil）未 digest pin，可能因上游标签漂移导致测试不稳定 | testutil/postgres.go:82,123,173, testutil/redis.go:22, testutil/miniredis.go:24 | 测试镜像补 digest pin（testcontainers 支持 `postgres:16-alpine@sha256:...`）；与 Dockerfile/K8s 风格统一 |
| v2-O-51 | 正确性/可维护性 | OPTIONAL | `TestRateLimiter_ConcurrentRequests` 用 limit=20 启动 10 goroutine，全部应被允许；且 `if !allowed { errCh <- nil }` 对被拒请求发 nil，循环 `if err != nil` 不捕获被拒。该测试无法验证并发限流边界行为 | rate_limiter_test.go:99-131 | 改为 limit<goroutine 数，断言被拒数量符合预期（如 limit=5, 10 goroutine, 期望 5 允许 5 拒绝） |
| v2-O-52 | 可观测性 | OPTIONAL | 无任何测试验证 metrics 暴露或 trace 传播；outbox 测试仅验证插入不验证消费/发布 | postgres_gdpr_lobby_test.go（全局）, outbox_test.go（仅 Insert） | 增补 metrics 端点测试与 outbox 消费/重试路径测试 |
| v2-F-45 | 可维护性 | FYI | 测试覆盖广：auth（JWT 签发/验签/篡改/撤销/过期/none-alg/并发 quickplay）、admin（7 种 token 拒绝场景）、game room（生命周期/并发/冲突钩子/nil store）、outbox（并发/大 payload/特殊字符）、postgres（CRUD/lobby/GDPR/游标分页）、rate limiter（边界/并发/多窗口）、redis（房间注册）、ws（连接/拒绝/源/连接上限）。testcontainers 真实 DB + miniredis 快路径混合策略成熟 | 全目录 | — |
| v2-F-46 | 可维护性 | FYI | 测试稳定性设计良好：`skipIfShort`（testing.Short skip）、容器不可用 `t.Skipf`（不 Fatal）、t.Cleanup 统一清理、ws 测试设 5s handshake + 3s read deadline 超时 | testutil/postgres.go:205-209, testutil/redis.go:28, ws_handler_test.go:29,64 | — |
| v2-F-47 | 文档一致性 | FYI | postgres_store_test.go:16-21 顶部注释明确解释 testcontainers 取舍（"catches bugs mocks cannot... ~5s per container"），文档与实现一致 | postgres_store_test.go:16-21 | — |

### 整体健康度: 🟡 3.6/5
覆盖广度与稳定性设计优秀，但 GDPR 断言 bug 削弱正确性信心。

---

## 跨资产备注

| 发现 ID | 严重级别 | 描述 |
|---------|---------|------|
| v2-F-48 | FYI | v1 基线报告 `docs/superpowers/reports/2026-07-07-full-self-inspection-report.md` 经 Glob 实测**不存在**（reports/ 仅 v2-asset-inventory.md + v2-task1-results.md）。v2-task1-results.md 中 v2-C-04 标注"误报，报告实际存在"本身有误，建议主代理复核 A-075 文档一致性结论 |

---

## 汇总统计

| 资产 | 整体评分 | 健康度 | CRITICAL | REQUIRED | OPTIONAL | FYI |
|------|---------|--------|----------|----------|----------|-----|
| A-063 security-scan CI | 3.6/5 | 🟡 | 0 | 2 (R-60, R-61) | 2 (O-45, O-46) | 1 (F-38) |
| A-069 K8s global | 3.0/5 | 🔴 | 1 (C-15) | 3 (R-62~R-64) | 1 (O-47) | 3 (F-39~F-41) |
| A-073 Docker | 3.5/5 | 🟡 | 0 | 2 (R-65, R-66) | 2 (O-48, O-49) | 3 (F-42~F-44) |
| A-034 集成测试 | 3.6/5 | 🟡 | 0 | 1 (R-67) | 3 (O-50~O-52) | 3 (F-45~F-47) |
| **合计** | — | — | **1** | **8** | **8** | **10** |

**最优先修复**：
1. v2-C-15（A-069）：补 prometheus-adapter 配置，恢复 HPA ws_connections 指标链路
2. v2-R-67（A-034）：修正 GDPR 匿名化断言（&&→||），避免合规验证空转
3. v2-R-62（A-069）：balloon-game 镜像 digest pin + CI kustomize set image
4. v2-R-63/R-64（A-069）：Redis securityContext + namespace PSS restricted 标签
5. v2-R-60/R-61（A-063）：GitHub Actions digest pin + 扩展定期扫描覆盖
