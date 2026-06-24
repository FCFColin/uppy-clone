# ADR 索引 (Architecture Decision Records)

本目录记录项目所有重要架构决策。每个 ADR 包含：上下文、决策、后果。

## ADR 列表

| 编号 | 标题 | 状态 | 日期 |
|------|------|------|------|
| 001 | 从 Cloudflare Workers 迁移到 Go + PostgreSQL | 已接受 | 2025-01 |
| 002 | WebSocket 使用二进制协议而非 JSON | 已接受 | 2025-01 |
| 003 | Magic Link 认证而非传统密码 | 已接受 | 2025-01 |
| 004 | 引入熔断器保护下游依赖 | 已接受 | 2025-02 |
| 005 | Hub 无状态化与房间状态外置 | 已接受（部分实施） | 2025-02 |
| 006 | Redis 读缓存层 | 提议中 | 2026-06 |
| 007 | Redis Stream 消息队列 | 已接受 | 2026-06 |
| 008 | 审计日志防篡改持久化 | 已接受 | 2026-06 |
| 009 | Transactional Outbox 模式 | 已接受 | 2026-06 |
| 010 | 异步邮件发送 | 已接受 | 2026-06 |

## ADR 流程

1. **提议**: 创建新 ADR 文件，状态为"提议中"
2. **讨论**: 团队评审，收集反馈
3. **决策**: 达成共识后状态改为"已接受"或"已否决"
4. **归档**: 不再适用的决策标记为"已废弃"

命名规范: `NNN-简短标题.md`，NNN 为三位数字序号。

## 索引一致性校验

CI 通过以下脚本校验本索引与实际 ADR 文件一致：

```bash
# 校验 README 表格行数 = docs/adr/ 下 ADR 文件数（排除 README.md）
adr_count=$(find docs/adr -name '0*.md' | wc -l)
table_rows=$(grep -c '^\| 0' docs/adr/README.md)
[ "$adr_count" -eq "$table_rows" ] || { echo "ADR index mismatch"; exit 1; }
```
