/**
 * Token 刷新工具
 *
 * Access token: 15 分钟有效，存储在 HttpOnly cookie（前端不可读）
 * Refresh token: 7 天有效，存储在 HttpOnly cookie（前端不可读）
 * 当 API 返回 401 时，自动尝试 refresh，成功则重试原请求
 *
 * RO-042: fetchWithRefresh 已合并到 apiFetch (api_fetch.ts)。
 * refreshAccessToken 和 logout 继续使用原生 fetch 以避免递归
 * (apiFetch → refreshAccessToken → apiFetch)。
 */

let isRefreshing = false;
let refreshPromise: Promise<boolean> | null = null;

function isAbortError(e: unknown): boolean {
  return e instanceof DOMException && e.name === 'AbortError';
}

async function authFetchWithTimeout(url: string, init: RequestInit, timeoutMs: number): Promise<Response> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, { ...init, signal: controller.signal });
  } finally {
    clearTimeout(timer);
  }
}

/**
 * 尝试用 HttpOnly refresh cookie 刷新 access token
 *
 * 返回 true 表示刷新成功，false 表示失败（需要重新登录）
 * 使用单例模式防止并发刷新
 */
export async function refreshAccessToken(): Promise<boolean> {
  if (isRefreshing && refreshPromise) {
    return refreshPromise;
  }

  isRefreshing = true;
  refreshPromise = (async () => {
    try {
      const res = await authFetchWithTimeout('/api/v1/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
      }, 10_000);

      if (!res.ok) {
        return false;
      }

      const data = (await res.json()) as { refreshed?: boolean };
      return data.refreshed === true;
    } catch (e) {
      if (isAbortError(e)) {
        return false;
      }
      return false;
    } finally {
      isRefreshing = false;
      refreshPromise = null;
    }
  })();

  return refreshPromise;
}

/**
 * 登出：调用后端 logout 端点（清除 HttpOnly cookies）
 */
export async function logout(): Promise<void> {
  try {
    await authFetchWithTimeout('/api/v1/auth/logout', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
    }, 5_000);
  } catch {
  }
  window.location.href = '/';
}
