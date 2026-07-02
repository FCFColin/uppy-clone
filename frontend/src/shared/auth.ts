/**
 * Token 刷新工具
 *
 * Access token: 15 分钟有效，存储在 HttpOnly cookie（前端不可读）
 * Refresh token: 7 天有效，存储在 HttpOnly cookie（前端不可读）
 * 当 API 返回 401 时，自动尝试 refresh，成功则重试原请求
 */

let isRefreshing = false;
let refreshPromise: Promise<boolean> | null = null;

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
      const res = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
      });

      if (!res.ok) {
        return false;
      }

      const data = await res.json() as { refreshed?: boolean };
      return data.refreshed === true;
    } catch {
      return false;
    } finally {
      isRefreshing = false;
      refreshPromise = null;
    }
  })();

  return refreshPromise;
}

/**
 * 带自动刷新的 fetch
 *
 * 当请求返回 401 时，自动尝试 refresh，成功则重试原请求
 */
export async function fetchWithRefresh(url: string, options: RequestInit = {}): Promise<Response> {
  const res = await fetch(url, { ...options, credentials: 'include' });

  if (res.status === 401) {
    const refreshed = await refreshAccessToken();
    if (refreshed) {
      return fetch(url, { ...options, credentials: 'include' });
    }
    redirectToLogin();
  }

  return res;
}

/** 刷新失败时跳转到首页 */
function redirectToLogin(): void {
  window.location.href = '/';
}

/**
 * 登出：调用后端 logout 端点（清除 HttpOnly cookies）
 */
export async function logout(): Promise<void> {
  try {
    await fetch('/api/v1/auth/logout', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
    });
  } catch {
    // 忽略网络错误
  }
  window.location.href = '/';
}
