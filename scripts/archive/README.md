# 维护者专用脚本（归档）

本目录存放**一次性迁移/重构**脚本，日常开发与 CI **不依赖**此处文件。

| 脚本 | 用途 |
|------|------|
| `merge_go_tests.py` | 合并 Go 测试文件 |
| `merge-package-tests.py` | 按包合并测试 |
| `fix_imports.py` | 批量修正 import |
| `cleanup-old-code.ps1` | 本地清理旧代码（Windows） |

负载测试请使用 [`../load/`](../load/)（`k6-smoke.js`、`k6-ws-soak.js`）。
