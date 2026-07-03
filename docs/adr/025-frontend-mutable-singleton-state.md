# ADR-025: 前端受控状态管理 (GameStore)

## 状态: 已更新 (2026-07-03)

## 上下文

前端游戏客户端需要管理：
- 服务端快照数据（玩家位置、分数、阶段）
- 客户端插值状态（平滑渲染用的 lerped 位置）
- 连接状态（WS readyState、重连计数、认证状态）
- UI 状态（倒计时、错误覆盖层、昵称输入）

评估了 Redux、Zustand、Jotai 等状态管理库。前端零生产依赖是 ADR-018 的核心决策。

## 决策

采用 **dispatch/reducer 受控状态管理** 模式 (`frontend/src/game/store.ts`)：

1. **GameStore**：`dispatch(action)` / `getState()` / `select(selector)`（`frontend/src/game/store.ts`）
   - 所有状态变更通过纯 reducer (`frontend/src/game/reducer.ts`) 不可变更新
2. **状态读取**：通过 `store.select()` 或 `store.getState()`，不再直接变异全局对象
3. **插值私有状态**：`state_interp.ts` 内部闭包变量，不暴露给外部
4. **调试暴露**：开发模式将 `state`、`__ws` 挂到 `window`（`lifecycle.ts`）
5. **跨页面认证**：Cookie-based session（`shared/network/auth.ts`），`localStorage` 仅存储 `uppy-player-id`

## 后果

**正面**
- 零依赖，dispatch/reducer 模式简单可控
- 不可变更新保证状态可回溯
- 与子模块解耦（renderer/input/ws 不直接写 state）

**负面**
- 需要迁移所有直接状态变异到 dispatch 调用
- immutable 拷贝对性能有极小影响（不影响游戏循环）

**放弃的替代方案**
- Redux Toolkit：~10KB + boilerplate，对当前状态复杂度过度
- Zustand：轻量但仍引入依赖，收益有限
- Elm Architecture：与 imperative Canvas 渲染不匹配
- 原可变单例模式：状态变更来源难以追踪，测试需直接变异全局对象
