# Scripts

仓库辅助脚本，按用途分子目录。日常开发与 CI 仅依赖 `ci/` 与 `load/`。

| 目录 | 用途 |
|------|------|
| [`ci/`](ci/) | 覆盖率门禁、镜像 digest 校验、Alert rules 同步、仓库布局校验 |
| [`load/`](load/) | k6 负载与 soak 脚本（`make load-smoke` 等） |
| [`archive/`](archive/) | 一次性迁移/重构脚本，**不参与** CI |

## CI 脚本

- `ci/check-coverage.sh` — 分层覆盖率门禁（`make check-coverage`）
- `ci/check-docker-digests.sh` — Dockerfile digest 与 lockfile 一致性
- `ci/pin-digests.sh` — 解析并更新 digest lockfile
- `ci/sync-alert-rules.sh` — 从 `deploy/alertmanager/rules.yml` 生成 K8s ConfigMap
- `ci/check-repo-layout.sh` — ADR-021 目录布局白名单校验

## 负载测试

```bash
k6 run scripts/load/k6-smoke.js
k6 run scripts/load/k6-ws-soak.js
k6 run scripts/load/k6-single-room.js
```
