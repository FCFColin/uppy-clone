# ADR-025: 前端受控状态管理 (GameStore)

## 状态: ✅ 仍合理且已落地 (2026-07-11)

## 上下文

前端游戏客户端需要管理：
- 服务端快照数据（玩家位置、分数、阶段）
- 客户端插值状态（平滑渲染用的 lerped 位置）
- 连接状态（WS readyState、重连计数、认证状态）
- UI 状态（倒计时、错误覆盖层、昵称输入）

评估了 Redux、Zustand、Jotai 等状态管理库。前端零生产依赖是 ADR-018 的核心决策。

## 决策

采用 **dispatch/reducer 受控状态管理** 模式 (`frontend/src/game/store.ts`)：

1. **GameStore**：仅暴露 `dispatch(action)` / `getState()`（`frontend/src/game/store.ts`）
   - 所有状态变更通过纯 reducer (`frontend/src/game/reducer.ts`) 计算 `newState`
   - 注意：`dispatch` 随后用 `Object.assign(state, newState)` 将新字段**原地合并**到共享单例 `state`，因此 `state` 仍是可变单例（与文件名一致），不是完全不可变。reducer 自身不变异入参，但 store 层为避免上层引用切换做了原地合并
2. **状态读取**：通过 `getState()` 读取共享单例；无 `select(selector)` 方法
3. **插值私有状态**：`state_interp.ts` 内部闭包变量，不暴露给外部
4. **调试暴露**：`vite-env.d.ts` 声明了 `state`、`__ws`、`__gamePhase` 等全局类型，但 `lifecycle.ts` 当前**未**将 `state`/`__ws` 挂到 `window`（仅 `phase_sync.ts` 写入 `window.__gamePhase`）
5. **跨页面认证**：Cookie-based session（`shared/network/auth.ts`）；`localStorage` 实际存储 `uppy-nickname`（昵称记忆，`lifecycle.ts` / `session.ts`）和 `uppy-game-url`（排行榜页跳回游戏入口，`lifecycle.ts` / `leaderboard.ts`），不存储 `uppy-player-id`

## 后果

**正面**
- 零依赖，dispatch/reducer 模式简单可控
- reducer 纯函数保证状态可回溯
- 与子模块解耦（renderer/input/ws 不直接写 state）

**负面**
- store 层 `Object.assign` 原地合并意味着旧引用仍指向同一对象，需通过 reducer 返回新对象来触发更新判断
- immutable 拷贝对性能有极小影响（不影响游戏循环）
- 模块级 `let` 变量散布在 10+ 文件中（tutorial/ui_wind/ui_update/renderer 等），`resetClientState()` 必须显式调用每个模块的 reset 函数以防止跨局状态残留（2026-07-09 审计修复：补齐 resetTutorial/resetWindHint/resetReconnectAttempts/resetUIUpdateCache 调用）

**放弃的替代方案**
- Redux Toolkit：~10KB + boilerplate，对当前状态复杂度过度
- Zustand：轻量但仍引入依赖，收益有限
- Elm Architecture：与 imperative Canvas 渲染不匹配
- 原可变单例模式：状态变更来源难以追踪，测试需直接变异全局对象
