# ADR-006: Redis 读缓存层

## 状态

已接受

## 日期

2026-06-23

## 上下文

ListLobbies 和 CheckRoom 当前直接查询 PostgreSQL。在高 QPS 场景下（100x 流量，~200,000 读 QPS），PostgreSQL 成为读取瓶颈：

- `LoadAllActiveLobbies` 涉及 COUNT + 分页查询，深页查询性能差
- `CheckRoom` 每次加入房间前都要查询，频率极高
- PG 连接池上限 25，高并发读取会耗尽连接

## 决策

引入 Redis 读缓存层，TTL 30s，写穿透（Write-Through）策略。

### 实现细节

1. **缓存键设计**:
   - `lobby:list` — 大厅列表 JSON，TTL 30s
   - `lobby:check:{code}` — 单房间信息 JSON，TTL 30s

2. **写穿透**: 房间创建/删除/状态变更时，同步更新 Redis 缓存 + PG 持久化

3. **读路径**: 先查 Redis → miss 时回源 PG → 回填 Redis

4. **TTL 选择**: 30s 是一致性窗口和缓存命中率的权衡。游戏大厅列表 30s 延迟可接受。

## 后果

### 好处

- **降低 PG 读压力**: 热点数据命中缓存，PG 读 QPS 可降低 80%+
- **降低 P99 延迟**: Redis 读取 ~1ms vs PG ~10ms，P99 显著改善
- **支撑水平扩展**: 多实例共享 Redis 缓存，避免各实例重复查 PG

### 坏处

- **缓存一致性窗口**: 30s TTL 内可能读到旧数据（新创建的房间最多 30s 后才在列表可见）
- **Redis 内存增加**: 每个房间缓存约 1KB，10,000 房间约 10MB，可接受
- **代码复杂度**: 需要维护缓存失效逻辑，写穿透增加写路径复杂度
