# Balloon Game — 多人网页气球对战

一个实时多人网页气球对战游戏，使用 Go 后端 + TypeScript 前端。项目以游戏为载体，实践企业级架构、安全纵深、可观测性和运维自动化。

## 快速开始

```bash
# 安装依赖 & 启动开发服务
make dev

# 运行测试
make test         # 后端单元测试
make test-all     # 后端 + 前端 + 集成测试
make e2e          # Playwright E2E
make check        # lint + test + 覆盖率
```

## 项目架构

```
backend/           # Go 后端 (chi + pgx + gorilla-websocket)
frontend/          # TypeScript 前端 (Vite + Canvas 2D)
infra/             # 基础设施 (Terraform + K8s)
scripts/           # CI/负载脚本
docs/              # 文档
```

## 文档

| 位置 | 内容 |
|------|------|
| [docs/architecture/](docs/architecture/) | 系统架构说明 |
| [docs/adr/](docs/adr/) | 架构决策记录（29 份） |
| [docs/api/](docs/api/) | OpenAPI, AsyncAPI, WebSocket 协议 |
| [docs/security/](docs/security/) | 威胁模型、安全自检清单 |
| [docs/operations/](docs/operations/) | SLO、Runbook、环境说明 |
| [docs/development/](docs/development/) | 覆盖策略、基准测试 |
| [docs/README.md](docs/README.md) | 文档索引 |

## 技术栈

- **后端**: Go 1.26, chi, pgx, gorilla-websocket, go-redis, OpenTelemetry
- **前端**: TypeScript 5.6, Vite 6, Vitest, Canvas 2D
- **数据库**: PostgreSQL 16, Redis 7.4
- **部署**: Docker, Kubernetes (GKE), Terraform (GCP)

## 许可

MIT — 详见 [LICENSE](LICENSE)
