/**
 * Token 刷新工具
 *
 * 实现 JWT 双 token 机制的前端逻辑：
 * - Access token: 15 分钟有效，存储在 HttpOnly cookie（前端不可读）
 * - Refresh token: 7 天有效，存储在 localStorage
 * - 当 API 返回 401 时，自动尝试用 refresh token 刷新
 * - 刷新失败则跳转到首页
 */

const REFRESH_TOKEN_KEY = 'uppy-refresh-token';

/** 存储 refresh token 到 localStorage */
export function storeRefreshToken(token: string): void {
  localStorage.setItem(REFRESH_TOKEN_KEY, token);
}

/** 读取 localStorage 中的 refresh token */
export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH_TOKEN_KEY);
}

/** 清除 refresh token */
export function clearRefreshToken(): void {
  localStorage.removeItem(REFRESH_TOKEN_KEY);
}

let isRefreshing = false;
let refreshPromise: Promise<boolean> | null = null;

/**
 * 尝试用 refresh token 刷新 access token
 *
 * 返回 true 表示刷新成功，false 表示失败（需要重新登录）
 * 使用单例模式防止并发刷新
 */
export async function refreshAccessToken(): Promise<boolean> {
  // 防止并发刷新
  if (isRefreshing && refreshPromise) {
    return refreshPromise;
  }

  const refreshToken = getRefreshToken();
  if (!refreshToken) {
    return false;
  }

  isRefreshing = true;
  refreshPromise = (async () => {
    try {
      const res = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ refresh_token: refreshToken }),
      });

      if (!res.ok) {
        clearRefreshToken();
        return false;
      }

      const data = await res.json() as { refresh_token?: string };
      if (data.refresh_token) {
        storeRefreshToken(data.refresh_token);
      }
      return true;
    } catch {
      clearRefreshToken();
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
  const res = await fetch(url, options);

  if (res.status === 401) {
    const refreshed = await refreshAccessToken();
    if (refreshed) {
      // 重试原请求
      return fetch(url, options);
    }
    // 刷新失败，跳转首页
    redirectToLogin();
  }

  return res;
}

/** 刷新失败时跳转到首页 */
function redirectToLogin(): void {
  clearRefreshToken();
  window.location.href = '/';
}

/**
 * 登出：调用后端 logout 端点 + 清除本地存储
 */
export async function logout(): Promise<void> {
  const refreshToken = getRefreshToken();
  try {
    await fetch('/api/v1/auth/logout', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken || '' }),
    });
  } catch {
    // 忽略网络错误
  }
  clearRefreshToken();
  localStorage.removeItem('uppy-player-id');
  window.location.href = '/';
}
