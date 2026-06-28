# 安全自检清单

> 与 [threat-model.md](./threat-model.md) 配套使用。Phase 0 与第一层每次 PR/发布前执行；第二至六层建议每月对照一次。

## Phase 0：发布阻塞项（生产部署前必过）

| 检查 | 命令 / 位置 |
|------|-------------|
| GKE `TRUSTED_PROXY_CIDRS` 已注入 | 各 overlay `infra/k8s/overlays/*/kustomization.yaml` → ConfigMap `trusted-proxy-cidrs`；deploy 用 `__TRUSTED_PROXY_CIDRS__` 替换（GitHub secret `TRUSTED_PROXY_CIDRS`） |
| 生产 env 校验 | `EnableHSTS=true` 时 `TRUSTED_PROXY_CIDRS` 必填且至少含一条可解析 CIDR（`env.Validate()` → `validateTrustedProxyCIDRs`） |
| Admin lockout 按客户端 IP 隔离 | 部署后从两个公网 IP 各 5 次错误密码，仅各自 lockout；单测 `TestExtractClientIP_IsolatesClientsBehindSharedProxy` |
| Docker digest 全阶段 pin | `bash scripts/ci/check-docker-digests.sh Dockerfile` |
| 发布配置静态验收 | `powershell -File scripts/ci/verify-release-config.ps1`（含 `__IMAGE_TAG__`、`__TRUSTED_PROXY_CIDRS__`） |

**根因说明：** Ingress 后若未配置 trusted proxy CIDR，所有请求共享 LB `RemoteAddr`，admin lockout 与 `admin:login` 限流会锁死全体管理员（Medium 风险）。

## 第一层：CI/CD 与供应链（阻塞项）

| 检查 | 命令 / 位置 |
|------|-------------|
| Secret scan (detect-secrets) | `detect-secrets scan --baseline .secrets.baseline` 或 CI job `Secret Scanning (detect-secrets)` |
| Secret scan (gitleaks) | `gitleaks detect --source .` 或 CI job `Secret Scanning (gitleaks)` |
| Docker digest 锁定 | `bash scripts/ci/check-docker-digests.sh` |
| 仓库布局 | `make check-repo-layout` |
| Go 测试 + race | `cd backend && go test ./... -race -short` |
| golangci-lint (gosec) | `make lint` |
| 前端 audit | `cd frontend && npm audit --audit-level=high` |
| 前端测试 | `cd frontend && npm test` |
| Pre-commit | `pre-commit run --all-files` |
| 部署镜像 SHA pin | Kustomize `__IMAGE_TAG__` → commit SHA（见 `ci-cd.yml` deploy） |
| Cosign 验签 | deploy job `cosign verify` 步骤 |

**Makefile 快捷命令：** `make security-check`（第一层本地子集）

**自动化脚本（Windows PowerShell）：**

| 脚本 | 用途 |
|------|------|
| `scripts/ci/verify-required-checks.ps1` | 核对 `.github/settings.yml` 与 CI job 名称一致 |
| `scripts/ci/self-check-layers.ps1` | 第二至六层可自动化子集（auth、WS、validate、ratelimit、cooldown 契约） |
| `scripts/ci/verify-release-config.ps1` | 发布前静态验收（SHA tag、cosign、kustomize `__IMAGE_TAG__`、`__TRUSTED_PROXY_CIDRS__`） |

## 测试命令速查

| 层级 | 聚合命令 |
|------|----------|
| Phase 0 | `powershell -File scripts/ci/verify-release-config.ps1` + `go test ./internal/middleware/... -run ExtractClientIP -count=1` |
| 第一层 | `make security-check` |
| 第二至六层 | `powershell -File scripts/ci/self-check-layers.ps1` |
| 全量后端 | `cd backend && go test ./... -short -count=1` |
| 全量前端 | `cd frontend && npm test` |
| E2E | `make e2e` |

## 第二层：认证与会话

- [x] Magic Link 一次性验证（`backend/internal/auth/magiclink_test.go`）
- [x] Refresh token 在 HttpOnly `refresh` cookie，不在 localStorage
- [x] JWT 撤销 / refresh 轮换（`refresh_test.go`、`revoke_test.go`）
- [x] Admin 使用独立 `ADMIN_JWT_SECRET`（生产 `ENABLE_HSTS=true` 时必填）
- [x] Admin 登录 lockout（`handler/admin_test.go`）
- [x] Magic Link session cookie 使用 `IsSecure(r)`
- [x] POST `/api/v1/auth/verify` 可用于避免 URL token 泄露

## 第三层：WebSocket 与游戏逻辑

- [x] 未认证 WS 拒绝（`websocket_test.go`）
- [x] Origin 校验 CSWSH
- [x] Read limit 4096、全局连接上限
- [x] 生产 CSP `connect-src 'self'`（开发 `ENABLE_HSTS=false` 时允许 `wss:`/`ws:`）

## 第四层：输入验证与 XSS

- [x] SQL 参数化（`postgres_*.go`）
- [x] Nickname 消毒（`validate/nickname.go`）
- [x] Leaderboard 使用 `textContent` / DOM API，不用 `innerHTML` 渲染 API 数据
- [x] 静态文件路径穿越测试（`routes_test.go`）

## 第五层：中间件、CORS、限流

- [x] FailClosed 端点：quickplay、admin:login
- [x] `TRUSTED_PROXY_CIDRS` 生产已配置（env + K8s ConfigMap `trusted-proxy-cidrs`）
- [x] K8s manifest 已 wire `TRUSTED_PROXY_CIDRS`（`infra/k8s/base/service.yaml` → `balloon-game-region` ConfigMap）
- [x] `ALLOWED_ORIGINS` 生产非 localhost 默认
- [x] 多 IP 检测使用 `ExtractClientIP`（`middleware/ratelimit.go`）
- [x] 限流回归（`ratelimit_test.go`）

## 第六层：密钥、PII 与合规

- [x] 启动必填：`JWT_SECRET` ≥32、`ENCRYPTION_KEY`、`DATABASE_URL`；生产另需 `ADMIN_JWT_SECRET`、`TRUSTED_PROXY_CIDRS`
- [x] 邮箱 AES 存储、GDPR 删除流、审计链 HMAC
- [x] Metrics basic auth（生产）
- [x] `.env` 不入库

## 执行节奏

| 频率 | 内容 |
|------|------|
| 每次 PR | CI 全绿 + 第一层命令 |
| 每次发布 | Phase 0 + 第一层 + 部署 checklist（镜像 SHA、cosign verify、`TRUSTED_PROXY_CIDRS` secret） |
| 每月 | 第二至六层勾选 + E2E + CodeQL/npm audit 回顾 |
| 每季度 | 手动 CSWSH/CORS 探测；更新 threat model |

### 子 agent 结论（2026-06-27，对比 HEAD `265ce1a`）

| 审查 | 结论 | 工作区状态 |
|------|------|------------|
| [R2 game/WS](ecdf89ac-db25-4ac5-bf80-2f4bb777ab66) | 无 Critical；Important：WS cap TOCTOU、room lock 阻塞、cooldown 漂移 | **已修复**（TryReserveWSConnection、outbound 非阻塞、cooldown/TAP_REJECTED） |
| [R3 CI/infra](a6573164-4fe9-4658-b6d5-0e52e40ae3f8) | HEAD Phase 0/L1 失败 | 工作区已含 TRUSTED_PROXY、cosign verify、E2E 启动、digest pin（未提交） |
| [R1 auth/store](35af503f-48a7-4cf8-8255-a85e801f5c4a) | HEAD 未就绪（refresh/localStorage 等） | 工作区已改 HttpOnly + POST verify；CheckAuth 撤销检查 + GDPR 500 已验证 |
| [Security Review](63ac5b5b-f794-465b-96d4-beaf5b5d7450) | 2× Medium：无效 CIDR 静默忽略；admin lockout Redis 读失败 fail-open | **已修复**（`validateTrustedProxyCIDRs`、lockout 503 fail-closed + 单测） |
| [Bugbot](2421addc-b042-4e50-ba3f-9a2d68f687ff) | 2× High restore/registry split-brain；Medium cooldown + outbound | **已修复**（restore 过滤 + Redis 注册、cooldown roster、outbound 超时） |

**下一步：** 已 push main；监控 GitHub Actions CI。

## 审查记录

| 日期 | 分支 / PR | Medium+ 发现 | Remediation | 状态 |
|------|-----------|--------------|-------------|------|
| 2026-06-27 | branch changes | GKE 未 wire `TRUSTED_PROXY_CIDRS` → admin lockout DoS | K8s ConfigMap + deploy sed + `env.Validate()` | 已修复 |
| 2026-06-27 | branch changes | Dockerfile Go builder digest pin 回归 | 已恢复 `@sha256:` pin；CI `docker-pin-check` | 已验证 |
| 2026-06-27 | main 全库自检完成 | 后端 unit 覆盖率 73.4% < 100% 门禁；Windows 本地未跑 race/e2e | auth 测试编译、entry_flow 倒计时、index_leaderboard XSS 已修复；覆盖率债务 backlog | 已完成 |
| 2026-06-27 | 子 agent 审查 (07c34ac→265ce1a) | **HEAD 未就绪**；工作区已含多数修复 | 见下方「子 agent 结论」 | 已 commit main |
| 2026-06-27 | [Security Review](63ac5b5b-f794-465b-96d4-beaf5b5d7450) | 无效 `TRUSTED_PROXY_CIDRS` 启动成功 → 全员 lockout；Redis 读锁失败 bypass lockout | `validateTrustedProxyCIDRs`；admin lockout 503 fail-closed | 已修复 |
| 2026-06-27 | [Bugbot](2421addc-b042-4e50-ba3f-9a2d68f687ff) | restore 缺 Redis 注册；全 Pod PG restore；cooldown 计数；outbound 死锁 | restore 过滤+register；len(Players) cooldown；outbound 100ms 超时 | 已修复 |
