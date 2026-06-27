# 维护者专用脚本（归档）

本目录存放**一次性迁移/重构**脚本，日常开发与 CI **不依赖**此处文件。

| 脚本 | 用途 |
|------|------|
| `merge_go_tests.py` | 合并同一 Go 包内碎片 `*_test.go` |
| `merge-package-tests.py` | 合并前端 Vitest 测试文件 |

负载测试请使用 [`../load/`](../load/)（`k6-smoke.js`、`k6-ws-soak.js`、`k6-single-room.js`）。
