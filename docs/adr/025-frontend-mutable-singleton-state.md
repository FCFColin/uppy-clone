# ADR-025: 前端可变单例状态管理

## 状态: 提议中（审计草稿，2026-06-26）

## 上下文

前端游戏客户端需要管理：
- 服务端快照数据（玩家位置、分数、阶段）
- 客户端插值状态（平滑渲染用的 lerped 位置）
- 连接状态（WS readyState、重连计数、认证状态）
- UI 状态（倒计时、错误覆盖层、昵称输入）

评估了 Redux、Zustand、Jotai 等状态管理库。前端零生产依赖是 ADR-018 的核心决策。

## 决策

采用 **可变单例 + 模块私有状态** 模式：

1. **全局游戏状态**：`export const state` 可变对象（`frontend/src/game/state.ts:71-92`）
   - WS handler（`websocket.ts`）直接变异 `state`
   - Renderer（`renderer.ts`）只读 `state` 渲染
   - UI（`ui.ts`）读取 `state` 更新 DOM

2. **插值私有状态**：`state.ts` 内部闭包变量（`:105-250`），不暴露给外部

3. **调试暴露**：开发模式将 `state`、`__ws` 挂到 `window`（`main.ts:19-24`）

4. **跨页面认证**：Cookie-based session（`shared/auth.ts`），`localStorage` 仅存储 `uppy-player-id`

## 后果

**正面**
- 零依赖，无 action/reducer/dispatch 样板
- 游戏循环性能最优（直接属性访问，无 immutable 拷贝）
- 与 Canvas 命令式渲染模型自然匹配

**负面**
- 无单向数据流，状态变更来源难以追踪（~30 处 `console.log` 弥补）
- 多模块并发写入 `state` 无保护（单线程 JS 缓解但不保证逻辑一致性）
- `websocket.ts` 混合连接管理 + 状态变更 + UI 副作用，职责过重
- 无法使用时间旅行调试
- 测试需直接变异 `state` 对象（`state.test.ts` 做法）

**放弃的替代方案**
- Redux Toolkit：~10KB + boilerplate，对当前状态复杂度过度
- Zustand：轻量但仍引入依赖，收益有限
- Elm Architecture：与 imperative Canvas 渲染不匹配
