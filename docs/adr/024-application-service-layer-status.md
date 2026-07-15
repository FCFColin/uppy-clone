# ADR-024: Application Service 层 — 已删除

## 状态: 已完成（2026-06-26）

`internal/service` CQRS 脚手架（QueryService / CommandService / LobbyService /
AdminService）从未接线到生产代码，已全部删除。Auth 的两个已用方法迁移至 `auth` 包。

当前分层为 ADR-028 的 handler → domain(auth/game) → store，无 service 中间层。
