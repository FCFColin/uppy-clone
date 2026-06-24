# ADR-007: Redis Stream 消息队列

> 状态: Accepted
> 日期: 2026-06-23

## 上下文

游戏结果当前同步写入 PostgreSQL（`EndGameAndRecordResults`）。在高并发场景下（100x 流量，~50,000 写 QPS），同步写入成为瓶颈：

- 每个 Room 结束时执行一次事务（UPDATE session + INSERT results），耗时 ~20ms
- PG 连接池上限 25，高并发写入时连接等待
- 游戏结束操作受 DB 延迟影响，用户体验下降

## 决策

引入 Redis Stream 作为消息队列，Worker 批量消费写入 PG。

### 实现细节

1. **生产者**: Room 结束时 `XADD game:results * payload_json`，立即返回，不等待 DB

2. **消费者组**: `XGROUP CREATE game:results result-writers $`，多个 Worker 竞争消费

3. **批量写入**: Worker `XREADGROUP` 消费消息，每 100 条或每 1s 批量 `INSERT` 到 PG

4. **容错**:
   - Worker 消费失败时消息留在 Pending 列表
   - 其他 Worker 通过 `XCLAIM` 接管超时消息（超时阈值 30s）
   - `XACK` 在 PG 写入成功后执行

5. **消息格式**:
   ```json
   {
     "session_id": "xxx",
     "ended_at": 1234567890,
     "final_score": 100,
     "results": [...]
   }
   ```

## 后果

### 好处

- **解耦**: 游戏结束不再受 DB 延迟影响，Room 可立即释放资源
- **批量写入提升吞吐**: 100 条/批 vs 1 条/次，PG 写入吞吐提升 10-50x
- **削峰**: 突发流量先入队列，Worker 按自身节奏消费，保护 PG

### 坏处

- **架构复杂度增加**: 引入消费者组、消息重试、死信队列等概念
- **需处理消息丢失**: Worker 崩溃时未 ACK 的消息需被其他 Worker 接管
- **最终一致性**: 游戏结果不是实时写入 PG，有 1-5s 延迟（排行榜更新延迟）
- **监控需求**: 需监控 Stream 长度、Consumer Lag、Dead Letter 数量
