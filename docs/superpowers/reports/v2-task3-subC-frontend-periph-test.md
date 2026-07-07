# v2 自检 Task3 子代理 C：前端外围 + 测试 Medium/Low 资产审查

> 生成日期：2026-07-08
> 范围：6 个 Medium/Low 资产（A-051 / A-052 / A-053 / A-054 / A-055 / A-059）
> 适用轴：前端外围 5 资产 = 正确性 + 安全 + 可维护性；A-059 fuzz = 正确性 + 可读性 + 可维护性 + 可观测性 + 文档一致性
> 纯诊断任务，未修改任何业务代码
> 发现 ID 区间：CRITICAL v2-C-25 起 / REQUIRED v2-R-107 起 / OPTIONAL v2-O-90 起 / FYI v2-F-73 起

---

## 资产 A-051: CSS

### 基本信息
- 路径: `frontend/src/styles/play.css`, `frontend/src/admin.css`, `frontend/src/index.css`, `frontend/src/style.css`
- 关键性: Medium
- 适用轴: 正确性 + 安全 + 可维护性
- 重点: XSS 风险（CSS injection）、可维护性（重复样式、命名规范）

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | CSS 语法正确；`prefers-reduced-motion` 仅覆盖 `countdown-pop`，未覆盖 `spin`/`fadeIn` 动画，无障碍一致性有缺口 |
| 安全 | 5 | 静态 CSS，无 `url()` 用户可控路径、无 `expression()`、无外部 `@import`；index.html favicon 用 `data:` SVG 受控。无 CSS injection 向量 |
| 可维护性 | 3 | 4 个文件硬编码同一套色板（#e94560/#0f3460/#16213e/#1a1a2e/#06d6a0）无 CSS 变量/设计令牌；z-index 散布魔法数字（9999/10000/10001/9989/10100/5/6/8/1）无层级令牌 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-O-90 | 可维护性 | OPTIONAL | 调色板（#e94560 红 / #0f3460 深蓝 / #16213e 卡片 / #1a1a2e 背景 / #06d6a0 绿 / #aaa/#888 灰阶）在 4 个 CSS 文件中硬编码重复 30+ 次，无 `:root` CSS 变量。换肤或维护需全量替换 | `style.css:3,37,43,63,89`、`admin.css:15,22,49,102,133`、`index.css:53,81,102,130`、`play.css:93,109,138` | 在 `style.css` 顶部定义 `:root { --color-accent: #e94560; --color-bg: #1a1a2e; ... }`，各文件改用变量 |
| v2-O-91 | 可维护性 | OPTIONAL | z-index 散布魔法数字无层级令牌：loading-overlay=9999、countdown=10000、app-toast=10001、reconnect=9989、entry-overlay=10100、hud=5、cooldown=6、nickname-inline=8、canvas=1。命名不统一易碰撞 | `play.css:8,34,118,152`、`style.css:28,83,187,207,260` | 定义 `--z-base/--z-hud/--z-overlay/--z-modal` 等层级令牌并统一引用 |
| v2-F-73 | 正确性 | FYI | `play.css:62-64` 为 `.countdown-pop` 添加了 `@media (prefers-reduced-motion: reduce)`，但 `spin`（loading-spinner:26、reconnect-spinner:140）和 `fadeIn`（overlay、reconnect-banner）动画未遵守同一规则，无障碍一致性缺口 | `frontend/src/styles/play.css:28,73,123`、`frontend/src/style.css:33` | 统一为所有装饰性动画加 `prefers-reduced-motion` 降级，或全局 `@media (prefers-reduced-motion: reduce){ *{animation-duration:0.01ms!important} }` |

### 整体健康度: 🟡 4.0/5

---

## 资产 A-052: HTML

### 基本信息
- 路径: `frontend/admin.html`, `frontend/index.html`, `frontend/leaderboard.html`, `frontend/play.html`, `frontend/verify.html`
- 关键性: Medium
- 适用轴: 正确性 + 安全 + 可维护性
- 重点: CSP meta 标签、可访问性（aria）、SEO meta

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | verify.html 引用 `/favicon.svg` 但文件不存在（404）；verify.html og:title 仍用旧品牌 "Tap Balloon"，其余页面已迁移到 "Uppy"，品牌/SEO 不一致 |
| 安全 | 2 | **5 个 HTML 文件均无 Content-Security-Policy meta 标签**，XSS 防御完全依赖 JS 层输出编码；admin 后台、昵称、排行榜等用户内容场景缺基础纵深防御 |
| 可维护性 | 3 | admin.html 与 verify.html 含内联样式（style 属性 + `<style>` 块），绕过 CSS 缓存且与 style.css 重复；admin.html 的 toggle 开关无 ARIA 角色 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-C-25 | 安全 | CRITICAL | 5 个 HTML 文件均无 CSP meta（`<meta http-equiv="Content-Security-Policy">`）。项目处理用户昵称、排行榜、管理员密码等敏感/用户内容，缺 CSP 意味着任何 JS 层 XSS（如 innerHTML 误用）可直接执行任意脚本窃取 admin token。Grep 确认全仓无 "Content-Security-Policy" | `frontend/*.html`（全部 5 个 head 段） | 至少添加 `default-src 'self'; script-src 'self'; connect-src 'self' ws: wss:; style-src 'self' 'unsafe-inline'; img-src 'self' data:`；Vite 构建可用 nonce 或 hash 收紧 'unsafe-inline' |
| v2-R-107 | 正确性 | REQUIRED | verify.html:8 引用 `<link rel="icon" type="image/svg+xml" href="/favicon.svg">`，但 `frontend/public/favicon.svg` 不存在（Glob 确认），部署后 404；index.html:11 已改用内联 `data:image/svg+xml` emoji favicon。同时 verify.html:6 `og:title` 仍为 "Tap Balloon"（旧品牌），其余页面统一为 "Uppy" | `frontend/verify.html:6,8` | 复用 index.html 的内联 data URI favicon；og:title 改为 "Uppy - 登录验证" |
| v2-O-92 | 可维护性 | OPTIONAL | admin.html:20,35 含内联 `style="..."`；verify.html:10-27 整个 `<style>` 块与 style.css:3 的 body 样式重复（background/font-family/min-height）；admin.html:30-33 的 toggle 开关 `<input type="checkbox">` 无 `role="switch"`/`aria-checked`，屏幕阅读器不识别开关语义 | `frontend/admin.html:20,35,30-33`、`frontend/verify.html:10-27` | 内联样式迁入 admin.css/verify.css；toggle 加 `role="switch" aria-checked` |

### 整体健康度: 🟡 2.7/5

---

## 资产 A-053: 构建配置

### 基本信息
- 路径: `frontend/vite.config.ts`, `frontend/vitest.config.ts`, `frontend/tsconfig.json`, `frontend/eslint.config.js`
- 关键性: Medium
- 适用轴: 正确性 + 安全 + 可维护性
- 重点: 供应链（依赖版本固定）、可维护性

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | vite.config.ts:5 `loadEnv(mode, resolve(__dirname, '..'), '')` 从父目录加载 env，耦合项目根布局；tsconfig `strict` + `noUncheckedIndexedAccess` 配置严谨 |
| 安全 | 3 | package.json 全部 devDependencies 用 `^` 范围；lockfileVersion 3 + integrity sha512 提供可复现性，但 `npm install`（非 `npm ci`）仍会漂移；CI 未在配置层强制 `npm ci` |
| 可维护性 | 3 | vitest.config.ts coverage `exclude` 达 22 项（admin/verify/leaderboard/state/websocket/lifecycle 等全文件排除），85% 阈值仅衡量未排除部分，真实覆盖率低于报告值；TODO 注释自承认 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-108 | 安全 | REQUIRED | package.json 所有依赖用 `^` 范围（`"vite": "^6.0.0"`、`"vitest": "^4.1.9"` 等），package-lock.json 虽 pin 到精确版本，但配置层未禁止 `npm install` 漂移。供应链上 `npm install` 在新环境可能引入与 lockfile 不一致的补丁版本 | `frontend/package.json:18-28` | CI 强制 `npm ci`；或对关键依赖用精确版本（去 `^`）；可加 `"engines"` 字段约束 Node 版本（见 A-054 v2-R-109） |
| v2-O-93 | 可维护性 | OPTIONAL | vitest.config.ts:12-39 coverage `exclude` 列表 22 项，排除 admin.ts/verify.ts/leaderboard.ts/state.ts/websocket.ts/lifecycle.ts/window_events.ts 等整文件。thresholds 注释自承 "Gradually improve"，但 85%/85%/80%/85% 仅覆盖剩余文件，报告的覆盖率指标误导维护者对真实测试覆盖的判断 | `frontend/vitest.config.ts:12-47` | 缩小 exclude 范围（改为行级 `/* istanbul ignore */`），或拆分 thresholds 按目录设定；至少在 README/注释中说明 "85% 为排除后值" |
| v2-F-74 | 正确性 | FYI | vite.config.ts:5 `loadEnv(mode, resolve(__dirname, '..'), '')` 从父目录（项目根）加载 env，而非 frontend/ 本目录。这是为了共享后端 .env（如 BACKEND_URL），但耦合 frontend 到父目录布局，若 frontend 被移动或单独克隆，build 静默失败（BACKEND_URL 回退 default） | `frontend/vite.config.ts:5` | 加注释说明依赖项目根 .env；或显式 `loadEnv(mode, resolve(__dirname), '')` + 单独读根 .env |

### 整体健康度: 🟡 3.3/5

---

## 资产 A-054: 前端依赖

### 基本信息
- 路径: `frontend/package.json`, `frontend/package-lock.json`
- 关键性: Medium
- 适用轴: 正确性 + 安全 + 可维护性
- 重点: 供应链安全（漏洞、过期依赖、版本固定）

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | 依赖与脚本声明完整；`test:frontend:gate` 脚本与 `test:frontend` 完全相同，"gate" 命名误导（无额外阈值强制） |
| 安全 | 4 | **无运行时依赖**（仅 devDependencies），攻击面小；lockfileVersion 3 + sha512 integrity 全量存在；有独立 `audit` 脚本（`npm audit --audit-level=high`）；但未纳入默认 `npm test` 流程 |
| 可维护性 | 3 | 缺 `engines` 字段约束 Node 版本；`^` 范围（见 A-053 v2-R-108）；`fast-check` devDep 存在用于属性测试 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-109 | 安全 | REQUIRED | package.json 无 `engines` 字段，Node 版本未约束。vite 6 / vitest 4 要求 Node 18+，但配置不声明，CI 或新开发者用旧 Node 会构建失败且错误信息难定位。结合 v2-R-108 的 `^` 范围，供应链可复现性双重弱化 | `frontend/package.json:1-29` | 添加 `"engines": { "node": ">=18.0.0" }` 并在 CI 用 `volta`/`.nvmrc` pin 精确版本 |
| v2-O-94 | 可维护性 | OPTIONAL | `"test:frontend:gate": "npm run test:frontend"`（package.json:16）与 `test:frontend` 完全相同，无额外阈值强制或失败门禁逻辑。"gate" 命名暗示有质量门，实际仅重复运行 coverage，误导维护者认为有独立门禁 | `frontend/package.json:16` | 要么删除 gate 脚本，要么改为 `vitest run --coverage --threshold` 加严格失败条件，或在 gate 中串联 `npm run lint` + `tsc --noEmit` |
| v2-F-75 | 可维护性 | FYI | `npm audit`（package.json:12）是独立脚本，未纳入 `npm test` 或 CI 默认流程；`--audit-level=high` 只报 high+，moderate 漏洞静默。无运行时依赖使风险较低，但 devDep 漏洞（如 jsdom/vite 历史 CVE）仍可能影响构建机 | `frontend/package.json:12` | CI 加 `npm audit --audit-level=moderate` 步骤，或纳入 pre-merge 检查 |

### 整体健康度: 🟡 3.7/5

---

## 资产 A-055: vite-env.d.ts

### 基本信息
- 路径: `frontend/src/vite-env.d.ts`
- 关键性: Low
- 适用轴: 正确性 + 安全 + 可维护性

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | `state: unknown` / `__interp: unknown` 类型为 unknown，访问需 cast，丧失类型安全；9 个全局变量声明反映前端架构存在全局可变状态 |
| 安全 | 3 | 9 个 `declare global { var ... }` 将可变状态挂到 window，理论上可被浏览器控制台/扩展篡改；非直接 XSS 向量，但削弱信任边界 |
| 可维护性 | 2 | 生产全局（state/requestRestart/generateRandomNickname/submitSetupNickname/__ws/_restartCountdownTimer）与测试/调试全局（__gamePhase/__seenSeqs/__interp）混在同一文件，污染生产类型空间；全局可变状态散布难以追踪 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-110 | 可维护性 | REQUIRED | 文件声明 9 个 `declare global { var ... }` 全局可变状态（state、__gamePhase、requestRestart、generateRandomNickname、__seenSeqs、__interp、submitSetupNickname、__ws、_restartCountdownTimer）。这是前端架构欠债的标志——连接对象、游戏状态、倒计时定时器等本应封装在模块/类中，现散落于 window 全局。任何模块均可读写，变更影响半径极大，测试需 mock 全局 | `frontend/src/vite-env.d.ts:6-15` | 将 `__ws` 收敛到 connection 模块单例；`state`/`__interp`/`__seenSeqs` 移入 store 模块；`_restartCountdownTimer` 移入 restart_vote_ui 模块；最终目标删除全部 global 声明 |
| v2-O-95 | 正确性 | OPTIONAL | `state: unknown`（:7）和 `__interp: unknown`（:11）用 unknown 类型，等于无类型。任何 `window.state` 访问都需 `as GameState` cast，cast 错误不被 TS 捕获，类型安全形同虚设 | `frontend/src/vite-env.d.ts:7,11` | 若全局必须保留，改为 `state: GameState`（import 类型）；或在迁移期间用 `state: GameState \| undefined` 强制调用方校验 |
| v2-F-76 | 可维护性 | FYI | `__` 前缀全局（__gamePhase、__seenSeqs、__interp）为测试/调试暴露，但与生产全局（state、requestRestart...）混在同一声明文件，无分离。生产构建仍包含这些测试钩子的类型，且 `__gamePhase: string` 等暴露内部相位枚举为字符串，易写错 | `frontend/src/vite-env.d.ts:8,11,12` | 拆分为 `vite-env.d.ts`（生产）+ `vite-env.test.d.ts`（测试全局）；`__gamePhase` 改为具体联合类型而非 `string` |

### 整体健康度: 🔴 2.7/5

---

## 资产 A-059: 后端 fuzz 测试

### 基本信息
- 路径: `backend/internal/protocol/decode_fuzz_test.go`
- 关键性: Medium
- 适用轴: 正确性 + 可读性 + 可维护性 + 可观测性 + 文档一致性
- 重点: 覆盖率、可观测性
- 关联源码: `backend/internal/protocol/decode.go`（6 个 Decode* 函数：DecodeMessage / DecodeTap / DecodeNicknamePayload / DecodeSetNickname / DecodeRestartVote / DecodePing）

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | `DecodeNicknamePayload`（decode.go:30）未被直接 fuzz，仅经 `DecodeSetNickname` 间接覆盖，且后者 seed 固定以 MsgSetNickname 开头，arbitrary 长度字节未直接压测；5 个 fuzz 函数仅丢弃返回值，oracle 弱（仅测 "不 panic"） |
| 可读性 | 4 | 结构清晰，一函数一 fuzz；seed corpus 无注释但自解释（empty/valid/truncated/64KB）；命名规范 |
| 可维护性 | 4 | 每个 Decode* 函数对应独立 Fuzz 函数，增删解耦；seed 覆盖边界（空、恰好长度、超长 64KB） |
| 可观测性 | 2 | 无 `f.Logf` 日志、无覆盖率追踪、无 panic 时的上下文输出（依赖 Go 默认）；fuzzer 命中哪些分支不可见，难以评估有效性 |
| 文档一致性 | 3 | decode.go 每个函数有 doc 注释，但无 fuzz 策略说明、无运行方式文档（`go test -fuzz=...`）；与 property_test.go 的职责边界未说明 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-111 | 正确性 | REQUIRED | `DecodeNicknamePayload`（decode.go:30-42）是 6 个 Decode 函数中唯一未被直接 fuzz 的。它经 `DecodeSetNickname` 间接覆盖，但 `FuzzDecodeSetNickname` 的 seed 均以 `MsgSetNickname` 字节开头（:29-32），fuzzer 难以探索 `DecodeNicknamePayload` 的 `nickLen` 边界（如 nickLen=255 但 payload 不足、nickLen=0 的 `<=0` 分支）。同时所有 fuzz 函数仅 `_, _ = Decode...(data)` 丢弃返回值，oracle 仅 "不 panic"，无法捕获逻辑回归（如返回错误值） | `backend/internal/protocol/decode_fuzz_test.go:28-37`、`decode.go:30` | 新增 `FuzzDecodeNicknamePayload` 直接 fuzz；对有 `ok bool` 返回值的函数，加 round-trip oracle：`EncodeX(DecodeX(data))` 应等价，或至少断言 `ok=false` 时输入确实非法 |
| v2-O-96 | 可观测性 | OPTIONAL | 5 个 fuzz 函数无任何日志或覆盖率埋点。fuzzer 运行时无法知道命中了哪些分支（如 `nickLen > 255` 分支是否被探索）、累计执行次数、平均输入大小。panic 时仅依赖 Go 默认输出，无输入十六进制转储 | `backend/internal/protocol/decode_fuzz_test.go`（全文） | 关键分支加 `f.Logf`；用 `go test -fuzz -coverprofile` 生成覆盖率；可加 `testing.Main` 注册自定义 reporter 记录 fuzz 统计 |
| v2-F-77 | 文档一致性 | FYI | decode.go 函数注释完整，但无 fuzz 策略文档：未说明如何运行（`go test -fuzz=FuzzDecodeMessage -fuzztime=30s`）、与 `property_test.go` 的职责边界（fuzz 测不 panic，property 测语义？）、seed corpus 选择依据。新人难以接手 fuzz 维护 | `backend/internal/protocol/decode_fuzz_test.go:1` | 文件顶部加 package 注释说明 fuzz 策略与运行方式；或在 `protocol/doc.go` 集中说明测试体系 |

### 整体健康度: 🟡 3.2/5

---

## 汇总

### 评分总览
| 资产 ID | 名称 | 整体健康度 | v2 评分 |
|---------|------|-----------|---------|
| A-051 | CSS | 🟡 | 4.0/5 |
| A-052 | HTML | 🟡 | 2.7/5 |
| A-053 | 构建配置 | 🟡 | 3.3/5 |
| A-054 | 前端依赖 | 🟡 | 3.7/5 |
| A-055 | vite-env.d.ts | 🔴 | 2.7/5 |
| A-059 | 后端 fuzz 测试 | 🟡 | 3.2/5 |

### 发现数量统计
| 严重级别 | 数量 | 发现 ID |
|---------|------|---------|
| CRITICAL | 1 | v2-C-25 |
| REQUIRED | 5 | v2-R-107, v2-R-108, v2-R-109, v2-R-110, v2-R-111 |
| OPTIONAL | 7 | v2-O-90, v2-O-91, v2-O-92, v2-O-93, v2-O-94, v2-O-95, v2-O-96 |
| FYI | 5 | v2-F-73, v2-F-74, v2-F-75, v2-F-76, v2-F-77 |

**总计 18 个发现。**

### 优先处理建议
1. **v2-C-25（CRITICAL）**：5 个 HTML 全缺 CSP meta，admin 后台 + 用户昵称场景风险最高，优先补齐
2. **v2-R-107（REQUIRED）**：verify.html favicon 404 + 品牌不一致，修复成本极低
3. **v2-R-110（REQUIRED）**：vite-env.d.ts 9 个全局变量是前端架构欠债信号，建议结合 A-040 ws_connection 重构一并推进
4. **v2-R-108 + v2-R-109（REQUIRED）**：依赖版本固定 + engines 字段，供应链可复现性基线
5. **v2-R-111（REQUIRED）**：补 `FuzzDecodeNicknamePayload` + 强化 fuzz oracle，成本低收益高
