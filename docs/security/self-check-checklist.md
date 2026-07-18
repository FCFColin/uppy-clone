# 安全自检清单

> 与 [threat-model.md](./threat-model.md) 配套使用。Phase 0 与第一层每次 PR/发布前执行；第二至六层建议每月对照一次。

## Phase 0：发布阻塞项（生产部署前必过）

| 检查 | 命令 / 位置 | CI 自动化 |
|------|-------------|-----------|
| GKE `TRUSTED_PROXY_CIDRS` 已注入 | 各 overlay `infra/k8s/overlays/*/kustomization.yaml` → ConfigMap `trusted-proxy-cidrs`；deploy 用 `__TRUSTED_PROXY_CIDRS__` 替换（GitHub secret `TRUSTED_PROXY_CIDRS`） | - |
| 生产 env 校验 | `EnableHSTS=true` 时 `TRUSTED_PROXY_CIDRS` 必填且至少含一条可解析 CIDR（`env.Validate()` → `validateTrustedProxyCIDRs`） | - |
| Admin lockout 按客户端 IP 隔离 | 部署后从两个公网 IP 各 5 次错误密码，仅各自 lockout；单测 `TestExtractClientIP_IsolatesClientsBehindSharedProxy` | `go-ci.yml` → `Test` job |
| Docker digest 全阶段 pin | `bash scripts/ci/check-docker-digests.sh Dockerfile` | `go-ci.yml` → `Docker Image Pinning` job |
| 发布配置静态验收 | `powershell -File scripts/ci/verify-release-config.ps1`（含 `__IMAGE_TAG__`、`__TRUSTED_PROXY_CIDRS__`） | `ci-cd.yml` → `Release Config Verification` job + deploy step |

**根因说明：** Ingress 后若未配置 trusted proxy CIDR，所有请求共享 LB `RemoteAddr`，admin lockout 与 `admin:login` 限流会锁死全体管理员（Medium 风险）。

## 第一层：CI/CD 与供应链（阻塞项）

| 检查 | 命令 / 位置 |
|------|-------------|
| Secret scan (detect-secrets) | `detect-secrets scan --baseline .secrets.baseline` 或 CI job `Secret Scanning (detect-secrets)` |
| Secret scan (gitleaks) | `gitleaks detect --source .` 或 CI job `Secret Scanning (gitleaks)` |
| Docker digest 锁定 | `bash scripts/ci/check-docker-digests.sh` |
| 仓库布局 | `make check-repo-layout` |
| Go 测试 + race | `cd backend && go test ./... -race -short` |
| govulncheck（生产路径） | `cd backend && govulncheck -test=false ./cmd/... ./internal/...`；`GO-2026-5746` 仅经 testcontainers → docker client 传递依赖，生产镜像无此链 |
| 前端 audit | `cd frontend && npm audit --audit-level=high` |
| 前端测试 | `cd frontend && npm test` |
| Pre-commit | `pre-commit run --all-files` |
| 部署镜像 SHA pin | Kustomize `__IMAGE_TAG__` → commit SHA（见 `ci-cd.yml` deploy） |
| Cosign 验签 | deploy job `cosign verify` 步骤 |

**Makefile 快捷命令：** `make security-check`（第一层本地子集）

**自动化脚本（Windows PowerShell，已接入 CI）：**

| 脚本 | 用途 | CI Workflow |
|------|------|-------------|
| `scripts/ci/verify-required-checks.ps1` | 核对 `.github/settings.yml` 与 CI job 名称一致 | `repo-governance.yml`（每周一） |
| `scripts/ci/self-check-layers.ps1` | 第二至六层可自动化子集（auth、WS、validate、ratelimit、cooldown 契约） | `security-layer-checks.yml`（每月1日） |
| `scripts/ci/verify-release-config.ps1` | 发布前静态验收（SHA tag、cosign、kustomize `__IMAGE_TAG__`、`__TRUSTED_PROXY_CIDRS__`） | `ci-cd.yml` → `Release Config Verification` job |

## 测试命令速查

| 层级 | 聚合命令 | CI 自动化 |
|------|----------|-----------|
| Phase 0 | `go test ./internal/middleware/... -run ExtractClientIP -count=1` | `ci-cd.yml` → `Release Config Verification` job |
| 第一层 | `make security-check` | `go-ci.yml` + `ci-cd.yml` 多 job |
| 第二至六层 | 已在 CI 中自动化 | `security-layer-checks.yml`（每月1日） |
| 全量后端 | `cd backend && go test ./... -short -count=1` | `go-ci.yml` → `Test` job |
| 全量前端 | `cd frontend && npm test` | `ci-cd.yml` → `Quality Gate` job |
| E2E | `make e2e` | `ci-cd.yml` → `E2E Tests` job（3 browser × 10 spec） |

## 第二层：认证与会话

- [x] Magic Link 一次性验证（`backend/internal/auth/magiclink_test.go`）
- [x] Refresh token 在 HttpOnly `refresh` cookie，不在 localStorage
- [x] JWT 撤销 / refresh 轮换（`refresh_test.go`、`revoke_test.go`）
- [x] Admin 使用独立 `ADMIN_JWT_PRIVATE_KEY`（ES256 PEM；生产 `ENABLE_HSTS=true` 时必填）
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

- [x] 启动必填：`JWT_PRIVATE_KEY`（ES256 PEM）、`ENCRYPTION_KEY`、`DATABASE_URL`；生产另需 `ADMIN_JWT_PRIVATE_KEY`（ES256 PEM）、`TRUSTED_PROXY_CIDRS`
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
