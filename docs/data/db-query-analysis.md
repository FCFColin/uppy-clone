# 数据库查询分析报告

基于当前 schema（000001_init_schema + 000002_add_indexes + 000004_add_composite_indexes + 000008_drop_redundant_indexes），对关键查询的预期执行计划分析。

> 注：migration 000008 已删除以下冗余索引（被复合索引覆盖）：
> `idx_users_email`、`idx_sessions_lobby`、`idx_results_session`、`idx_lobby_states_updated_at`。
> 以下分析均基于删除后的当前索引集。

## 1. 用户游戏历史查询

```sql
SELECT gr.* FROM game_results gr
JOIN game_sessions gs ON gr.session_id = gs.id
WHERE gr.user_id = $1
ORDER BY gs.created_at DESC;
```

**预期索引使用**：
- `idx_game_results_user_id` (单列) → 按 user_id 过滤
- `idx_game_results_session_user` (复合, session_id, user_id) → JOIN 加速（最左前缀匹配 session_id）

**预期计划**：
```
Nested Loop
  → Index Scan using idx_game_results_user_id on game_results (user_id = $1)
  → Index Scan using game_sessions_pkey on game_sessions (id = gr.session_id)
```

## 2. 活跃房间列表查询（cursor 分页）

```sql
SELECT * FROM lobby_states
WHERE (updated_at, code) < ($1, $2)
ORDER BY updated_at DESC, code DESC
LIMIT $3;
```

**预期索引使用**：
- `idx_lobby_states_updated_code` (复合, updated_at DESC, code) → keyset 分页

**预期计划**：
```
Index Scan using idx_lobby_states_updated_code on lobby_states
  Index Cond: ((updated_at, code) < ($1, $2))
```

> 注：实现见 `store/postgres_lobbies_list.go` `LoadAllActiveLobbies`（ADR-028 拆分后）；
> 已弃用 OFFSET 深页方案。

## 3. 房间状态按 lobby_code + status 查询

```sql
SELECT * FROM game_sessions
WHERE lobby_code = $1 AND status = 'active';
```

**预期索引使用**：
- `idx_game_sessions_lobby_status` (复合, lobby_code, status) → 最左前缀匹配

**预期计划**：
```
Index Scan using idx_game_sessions_lobby_status on game_sessions
  Index Cond: (lobby_code = $1 AND status = 'active')
```
对比已删除的单列索引 `idx_sessions_lobby`（migration 000008 删除，被此复合索引覆盖）：
单列索引需在 lobby_code 过滤后再对 status 做 Filter，复合索引直接定位。

## 4. 清理过期房间

```sql
DELETE FROM lobby_states
WHERE updated_at < $1;
```

**预期索引使用**：
- `idx_lobby_states_updated_code` (复合, updated_at DESC, code) → 最左前缀匹配 updated_at 条件
  （`idx_lobby_states_updated_at` 单列索引已被 migration 000008 删除，复合索引覆盖此查询）

**预期计划**：
```
Index Scan using idx_lobby_states_updated_code on lobby_states
  Index Cond: (updated_at < $1)
```

## 5. 会话结果聚合查询

```sql
SELECT session_id, SUM(score_contribution), COUNT(*)
FROM game_results
WHERE session_id = $1
GROUP BY session_id;
```

**预期索引使用**：
- `idx_game_results_session_user` (复合, session_id, user_id) → 最左前缀匹配 session_id 条件
  （`idx_results_session` 单列索引已被 migration 000008 删除，复合索引覆盖此查询）

**预期计划**：
```
Index Scan using idx_game_results_session_user on game_results
  Index Cond: (session_id = $1)
  → Aggregate
```

## 索引覆盖率总结

| 查询场景 | 使用的索引 | 类型 |
|---------|-----------|------|
| 用户游戏历史 | idx_game_results_user_id | 单列 |
| 活跃房间分页 | idx_lobby_states_updated_code | 复合 |
| 房间状态查询 | idx_game_sessions_lobby_status | 复合 |
| 过期房间清理 | idx_lobby_states_updated_code | 复合（最左前缀） |
| 会话结果聚合 | idx_game_results_session_user | 复合（最左前缀） |

## 复合索引最左前缀原则说明

- `idx_game_results_session_user(session_id, user_id)`：可优化 `WHERE session_id = ?` 和 `WHERE session_id = ? AND user_id = ?`，但不能优化 `WHERE user_id = ?`
- `idx_game_sessions_lobby_status(lobby_code, status)`：可优化 `WHERE lobby_code = ?` 和 `WHERE lobby_code = ? AND status = ?`，但不能优化 `WHERE status = ?`
- `idx_lobby_states_updated_code(updated_at DESC, code)`：可优化 `WHERE updated_at > ? ORDER BY updated_at DESC`，覆盖排序避免 filesort
