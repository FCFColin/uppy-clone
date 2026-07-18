# Baseline Before V2 — deep-arch-slim-v2 Pre-flight

> **测量时间**: 2026-07-18 (Asia/Shanghai)
> **测量方式**: PowerShell `Get-ChildItem -Recurse -File | Where-Object { Extension -in ... } | Get-Content | Measure-Object -Line`
> **对照基线**: 69,072 行 (slim-tier1-ef-and-materialize/baseline-final.md, 2026-07-18)
> **测量工具**: RunCommand on Windows PowerShell

## 总行数

| 指标 | 数值 |
|---|---|
| **本次测量总行数** | **72,896** |
| 对照基线 (slim-tier1-ef-and-materialize 最终) | 69,072 |
| **漂移** | **+3,824 行 (+5.54%)** |
| 漂移阈值 | 100 行 |
| 漂移状态 | ⚠️ **超出阈值，已记录但继续执行** |

## 按扩展名分布

| 扩展名 | 总行数 | 文件数 |
|---|---:|---:|
| .go | 49,685 | 309 |
| .ts | 10,663 | 114 |
| .md | 4,900 | 73 |
| .yaml | 3,138 | 28 |
| .yml | 1,491 | 17 |
| .css | 882 | 4 |
| .sh | 603 | 6 |
| .ps1 | 446 | 13 |
| .tf | 438 | 3 |
| .html | 420 | 6 |
| .js | 230 | 5 |
| **合计** | **72,896** | **568** |

## 排除目录

测量排除以下目录：`node_modules` / `coverage` / `vendor` / `.git` / `dist` / `build`

## 漂移分析

本次测量 72,896 行 vs 前轮锁定基线 69,072 行，漂移 +3,824 行 (+5.54%)，**远超 100 行阈值**。

可能原因分析（未深入调查，仅记录假设）：
1. **前轮基线测量口径不同**：前轮 `baseline-final.md` 的测量可能使用了不同的扩展名白名单或排除规则，导致基线偏低。
2. **前轮结束后有新增代码**：前轮 `slim-tier1-ef-and-materialize` 完成后至本 spec 启动前，可能有其他 spec 或手工提交新增了代码（如 ADR-030/031、配置文件等）。
3. **本次测量包含了所有 .md / .yaml / .tf / .ps1 / .sh 文件**：前轮基线可能未计入部分文档/配置文件类型。

### 建议处理方式

- **本 spec 不回滚漂移**：以本次实测 72,896 行作为 `deep-arch-slim-v2` 的真实起点基线。
- **最终验证 (Task 31) 重新测量**：与本文件同口径对比，计算实际减幅。
- **减幅目标重算**：spec 原文基于 69,072 行设定 -9.1% / -12.5% / -20.4% 目标。若以 72,896 为新基线，等价目标为：
  - 保守 -9.1%: 减 6,634 行 → 目标 66,262 行
  - 中等 -12.5%: 减 9,112 行 → 目标 63,784 行
  - 激进 -20.4%: 减 14,871 行 → 目标 58,025 行
- **向用户报告**：Task 0 完成报告中明确告知漂移，由用户决定是否调整 spec 目标百分比。

## 测量命令 (PowerShell)

```powershell
Get-ChildItem -Recurse -File |
  Where-Object { $_.Extension -in '.go','.ts','.html','.css','.md','.yaml','.yml','.tf','.ps1','.sh','.js' -and $_.FullName -notmatch '\\(node_modules|coverage|vendor|\.git|dist|build)\\' } |
  Get-Content |
  Measure-Object -Line
```

按扩展名分组的详细命令见 spec Task 0.3 描述。

## 备注

- 本文件由 `deep-arch-slim-v2` Task 0 SubTask 0.3 生成。
- 后续 Task 31 (最终验证) 将生成本 spec 的 `baseline-final.md`，与本文件对比计算净减幅。
- 覆盖率基线见同目录 `coverage-before-v2.txt`。
- grep 复核证据见同目录 `findings-recheck-v2.md`。
