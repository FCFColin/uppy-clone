# Scripts

仓库辅助脚本，按用途分子目录。日常开发与 CI 仅依赖 `ci/` 与 `load/`。

| 目录 | 用途 |
|------|------|
| [`ci/`](ci/) | 覆盖率门禁、镜像 digest 校验、Alert rules 同步、仓库布局校验 |
| [`load/`](load/) | k6 负载与 soak 脚本 |

安全自检清单见 [`docs/security/self-check-checklist.md`](../docs/security/self-check-checklist.md)；本地快捷命令：`make security-check`。

## CI 脚本

- `ci/check-coverage.sh` — 分层覆盖率门禁（`make test-cover`）
- `ci/check-docker-digests.sh` — Dockerfile digest 与 lockfile 一致性
- `ci/sync-alert-rules.sh` — 从 `deploy/alertmanager/rules.yml` 生成 K8s ConfigMap
- `ci/check-repo-layout.sh` — ADR-021 目录布局白名单校验（Windows：`check-repo-layout.ps1` 或 `make check-repo-layout`）

## 负载测试

```bash
make load-smoke
make load-ws-soak
make load-single-room
# 或: k6 run scripts/load/k6-smoke.js -e BASE_URL=http://localhost:8080
```
