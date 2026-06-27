# uppy-clone

多人网页气球对战游戏 — Go 后端 + TypeScript 前端。

> **关于本项目的定位（请先读）**
> 这是一个**学习型工程项目**：以一个极简的多人网页小游戏作为**载体**，端到端实践
> **企业级多区域、多实例、水平高并发 SaaS 架构**（弹性、可观测、合规、多区域部署）。
> 游戏玩法只是练习的依托，不是目的。因此本仓库刻意包含远超"一个小游戏所需"的
> 重型组件（多区域 / CockroachDB / GDPR 合规 / OTel / owner 反向代理等），这些是
> **设计目标而非过度工程**。完整的目标、非目标与"刻意保留清单"见
> [docs/adr/000-project-charter.md](docs/adr/000-project-charter.md)。

## Quick Start（约 15 分钟）

### 前置条件

- Docker Desktop（PostgreSQL + Redis）
- Go 1.26+
- Node.js 20+

### 步骤

```bash
# 1. 克隆并进入项目
cd 多人网页游戏

# 2. 配置环境变量（开发模式）
cp .env.example .env
# 编辑 .env：设置 JWT_SECRET / ENCRYPTION_KEY / ADMIN_PASSWORD（见下方生成命令）
# 本地开发务必 ENABLE_HSTS=false（.env.example 已默认）

# 3. 生成密钥
export JWT_SECRET=$(openssl rand -hex 32)
export ENCRYPTION_KEY=$(openssl rand -hex 32)
export ADMIN_PASSWORD=$(openssl rand -base64 24)

# 4. 启动依赖 + 应用
make dev
# 或：docker compose up -d postgres redis && cd backend && go run ./cmd/server

# 5. 种子数据（可选）
make seed

# 6. 访问
# 前端: http://localhost:5173
# API:  http://localhost:8080/health
```

### 常用命令

| 命令 | 说明 |
|------|------|
| `make test` | 单元测试 |
| `make lint` | golangci-lint |
| `make bench` | 性能基准 |
| `make audit` | govulncheck + gitleaks |
| `make deadcode` | 死代码扫描 |
| `k6 run scripts/load/k6-smoke.js` | 负载冒烟（需服务运行） |

## Documentation

完整文档索引见 [docs/README.md](docs/README.md)，包括架构、Runbook、SLO、API 契约与 ADR。

## Environment Variables

The following secrets **must be explicitly provided** at deploy time. The
`docker-compose.yml` uses the `${VAR:?VAR required}` syntax, so the stack will
refuse to start when any of them is missing.

| Variable          | Required | Description                                      |
|-------------------|----------|--------------------------------------------------|
| `JWT_SECRET`      | Yes      | HMAC secret used to sign JWTs (>= 32 bytes).     |
| `ADMIN_PASSWORD`  | Yes      | Initial admin password (bcrypt-hashed at boot).  |
| `ENCRYPTION_KEY`  | Yes      | 32-byte hex key for AES-256-GCM field encryption.|
| `METRICS_USER`    | Prod     | Basic auth for `/metrics` and pprof.             |
| `METRICS_PASSWORD`| Prod     | Paired with METRICS_USER.                        |

### Generating secure values

```bash
# JWT_SECRET — 32 random bytes as hex
export JWT_SECRET=$(openssl rand -hex 32)

# ENCRYPTION_KEY — 32 random bytes as hex (must be exactly 64 hex chars)
export ENCRYPTION_KEY=$(openssl rand -hex 32)

# ADMIN_PASSWORD — a strong password
export ADMIN_PASSWORD=$(openssl rand -base64 24)
```

> **Security note:** The backend additionally rejects `JWT_SECRET` values
> containing `DEV_ONLY` or `change-in-production` when running in production
> mode (`ENABLE_HSTS != "false"`). Never reuse dev secrets in production.
> Production also requires `METRICS_USER` and `METRICS_PASSWORD`.
