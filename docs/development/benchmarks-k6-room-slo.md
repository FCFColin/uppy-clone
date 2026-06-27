# Frontend & Room Performance Baselines (v2)

> Phase 0 基线记录模板。改前/改后各跑一轮并更新数值。

## 后端 Prometheus（改后目标）

| 指标 | 正常条件 P95 目标 | 告警建议 |
|------|-------------------|----------|
| `room_lock_hold_seconds{operation="tick"}` | < 10ms | > 25ms 5m |
| `room_lock_hold_seconds{operation="message"}` | < 5ms | > 15ms 5m |
| `room_outbound_queue_depth` | < 32 | > 200 2m |
| `room_persist_lag_seconds` | < 0.5s | > 2s 5m |
| `outbox_lag_seconds` | < 1s | > 30s 10m |

## k6 基线命令

```bash
# 200 并发 WS soak
k6 run scripts/load/k6-ws-soak.js -e BASE_URL=http://localhost:8080 -e VUS=200

# 单房间 50 人
k6 run scripts/load/k6-single-room.js -e BASE_URL=http://localhost:8080 -e PLAYERS=50
```

## k6 结果（填实测）

| 场景 | 日期 | VUS/玩家 | first-snapshot p99 | checks rate | 备注 |
|------|------|----------|-------------------|-------------|------|
| ws-soak | _填_ | 200 | _填_ | _填_ | |
| single-room | _填_ | 50 | _填_ | _填_ | |

## Chrome Performance（前端）

| 项 | 改前 | 改后目标 |
|----|------|----------|
| snapshot 处理（主线程） | _填_ ms | < 4ms/帧（队列 drain ≤2 条） |
| render 帧时间 p95 | _填_ ms | < 16ms @ 60fps |
| 200ms RTT 模拟插值连续性 | _填_ | 无明显跳变 |

录制步骤：DevTools → Performance → 3 分钟 playing → 导出 trace 到 `docs/development/traces/`（可选）。
