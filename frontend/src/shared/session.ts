import {
  storeRefreshToken, getRefreshToken, refreshAccessToken, clearRefreshToken,
} from './auth.js';
import { fetchWithRetry } from './fetch.js';

export type SessionResult =
  | { ok: true }
  | { ok: false; status?: number; reason: 'rate_limit' | 'network' | 'server' };

/** Normalize host so HttpOnly cookies stay on one origin (localhost vs 127.0.0.1). */
export function normalizeAuthHost(): void {
  if (window.location.hostname === '127.0.0.1') {
    const port = window.location.port ? `:${window.location.port}` : '';
    window.location.replace(
      `http://localhost${port}${window.location.pathname}${window.location.search}${window.location.hash}`,
    );
  }
}

/**
 * Ensure the browser has a valid game session (access cookie + refresh token).
 * Falls back to quickplay when check/refresh fail.
 */
export async function establishGameSession(): Promise<SessionResult> {
  try {
    const checkRes: Response = await fetch('/api/v1/auth/check', { credentials: 'include' });
    if (checkRes.ok) return { ok: true };

    const refreshToken = getRefreshToken();
    if (refreshToken) {
      const refreshed = await refreshAccessToken();
      if (refreshed) {
        const recheck: Response = await fetch('/api/v1/auth/check', { credentials: 'include' });
        if (recheck.ok) return { ok: true };
      }
      clearRefreshToken();
    }

    const savedNick: string = localStorage.getItem('uppy-nickname') || '';
    const res: Response = await fetchWithRetry('/api/v1/auth/quickplay', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(savedNick ? { nickname: savedNick } : {}),
    }, 2);

    if (!res.ok) {
      if (res.status === 429) return { ok: false, status: 429, reason: 'rate_limit' };
      return { ok: false, status: res.status, reason: 'server' };
    }

    const data: { userId?: string; refreshToken?: string } = await res.json() as {
      userId?: string;
      refreshToken?: string;
    };
    if (data.userId) localStorage.setItem('uppy-player-id', data.userId);
    if (data.refreshToken) storeRefreshToken(data.refreshToken);
    return { ok: true };
  } catch {
    return { ok: false, reason: 'network' };
  }
}

export function sessionErrorMessage(result: SessionResult): string {
  if (result.ok) return '';
  if (result.reason === 'rate_limit') {
    return '请求过于频繁，请等待 1 分钟后重试';
  }
  if (result.reason === 'network') {
    return '网络错误，请确认后端已启动并重试';
  }
  return '认证失败，请用 http://localhost:3000 打开并强制刷新（Ctrl+Shift+R）';
}
