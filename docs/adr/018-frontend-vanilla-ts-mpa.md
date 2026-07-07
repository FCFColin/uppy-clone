# ADR-018: 前端采用 Vanilla TypeScript 多页应用（MPA）

## 状态: 已接受

## 上下文

本项目前端是一个 Canvas 2D 实时多人游戏客户端，需要：
- 低首屏加载时间（无框架 runtime 开销）
- 精确的帧级渲染控制（`requestAnimationFrame` + 插值）
- 二进制 WebSocket 协议的手动编解码
- 与后端共享物理常量（`shared/game/constants.ts` ↔ `protocol/constants.go`）

团队评估了 React、Vue、Svelte 等 SPA 框架，以及 Phaser/PixiJS 等游戏引擎。

## 决策

采用 **Vanilla TypeScript + Vite 6 多页应用（MPA）**，零生产运行时依赖：
- 四个 HTML 入口：`index.html`（大厅）、`play.html`（游戏）、`admin.html`（管理）、`verify.html`（邮件验证）
- Vite Rollup 多入口构建（`frontend/vite.config.ts:14-20`）
- 严格 TypeScript：`strict: true`、`noUncheckedIndexedAccess`（`frontend/tsconfig.json:3-16`）
- 状态管理：可变单例 `state` 对象（`frontend/src/game/state_types.ts:76`），无 Redux/Zustand（**已被 ADR-025 取代为 dispatch/reducer 受控状态管理**）

## 后果

**正面**
- 生产 bundle 极小（零框架 runtime）
- Canvas 渲染路径无虚拟 DOM 干扰
- 编译器严格模式捕获类型错误
- 与后端 monorepo 共存，协议常量可同步维护

**负面**
- WebSocket 客户端已拆分为 `ws_connect.ts`、`ws_handlers_*.ts` 等模块（`websocket.ts` barrel 文件已移除），但连接/协议/重连/UI 仍跨模块协作，缺乏框架的组件边界
- 无声明式 UI，DOM 操作分散在 `ui.ts` 和各 entry 文件中
- 前端测试已覆盖核心模块（37 个 vitest 文件，覆盖 game/ 和 shared/ 目录）
- 若未来需要复杂管理后台，vanilla TS 可维护性下降

**放弃的替代方案**
- React/Vue SPA：引入 ~40KB+ runtime，对 Canvas 游戏无收益
- Phaser/PixiJS：过度抽象物理层（后端已做权威模拟）
- Svelte：编译时框架，但 MPA 场景收益有限
