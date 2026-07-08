/**
 * fetch 超时工具
 *
 * 为关键路径 fetch 调用提供 8s 超时（与 lifecycle.ts 连接超时对齐）。
 * 网络挂起时主动 abort，避免 UI 永久等待（v2-R-14, v2-R-46, v2-R-50）。
 *
 * 用法：
 *   const { signal, cleanup } = createFetchTimeout();
 *   try {
 *     const res = await fetch(url, { signal });
 *     ...
 *   } catch (e) {
 *     // 超时或网络错误
 *   } finally {
 *     cleanup();
 *   }
 */
export const FETCH_TIMEOUT_MS = 8000;

export interface FetchTimeoutHandle {
  signal: AbortSignal;
  cleanup: () => void;
}

export function createFetchTimeout(timeoutMs: number = FETCH_TIMEOUT_MS): FetchTimeoutHandle {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  return {
    signal: controller.signal,
    cleanup: () => clearTimeout(timer),
  };
}
