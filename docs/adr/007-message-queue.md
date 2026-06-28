# ADR-007: Redis Stream 消息队列

## 状态

已接受

## 日期

2026-06-23

## 上下文

游戏结果当前同步写入 PostgreSQL（`EndGameAndRecordResults`）。高并发下同步写入成为瓶颈：每 Room 结束一次事务 ~20ms，连接池上限 25，游戏结束受 DB 延迟影响。

## 决策

引入 Redis Stream 作为消息队列，Worker 批量消费写入 PG：

1. Room 结束时 `XADD game:results`，立即返回
2. 消费者组 `result-writers` 竞争消费
3. 每 100 条或 1s 批量 INSERT
4. 失败消息 Pending + `XCLAIM` 接管；成功 `XACK`
5. Payload JSON：`session_id`、`ended_at`、`final_score`、`results`

## 后果

**正面**
- 游戏结束与 DB 解耦，Room 可立即释放
- 批量写入提升 PG 吞吐
- 削峰保护 PG

**负面**
- 消费者组、重试、监控复杂度增加
- 最终一致性：排行榜更新有 1–5s 延迟

## 关联

- ADR-009（Outbox）、ADR-011（PostgreSQL）
