import { t } from '../../i18n/t.js';

const DEFAULT_TIMEOUT_MS = 10_000;
const DEFAULT_RETRIES = 1;
const RETRY_DELAY_MS = 500;

function isAbortError(e: unknown): boolean {
  return e instanceof DOMException && e.name === 'AbortError';
}

async function fetchWithTimeout(
  url: string,
  init: RequestInit,
  timeoutMs: number,
  externalSignal?: AbortSignal | null,
): Promise<Response> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);

  const onExternalAbort = (): void => controller.abort();
  if (externalSignal) {
    if (externalSignal.aborted) {
      controller.abort();
    } else {
      externalSignal.addEventListener('abort', onExternalAbort, { once: true });
    }
  }

  try {
    return await fetch(url, { ...init, signal: controller.signal });
  } finally {
    clearTimeout(timer);
    if (externalSignal) {
      externalSignal.removeEventListener('abort', onExternalAbort);
    }
  }
}

let isRefreshing = false;
let refreshPromise: Promise<boolean> | null = null;

export async function refreshAccessToken(): Promise<boolean> {
  if (isRefreshing && refreshPromise) {
    return refreshPromise;
  }

  isRefreshing = true;
  refreshPromise = (async () => {
    try {
      const res = await fetchWithTimeout('/api/v1/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
      }, 10_000);

      if (!res.ok) return false;

      const data = (await res.json()) as { refreshed?: boolean };
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

export async function logout(): Promise<void> {
  try {
    await fetchWithTimeout('/api/v1/auth/logout', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
    }, 5_000);
  } catch {
    // ignore — best-effort logout, redirect anyway
  }
  window.location.href = '/';
}

export interface ApiFetchOptions extends RequestInit {
  retries?: number;
  timeoutMs?: number;
  autoRefresh?: boolean;
}

export async function apiFetch(url: string, options: ApiFetchOptions = {}): Promise<Response> {
  const {
    retries = DEFAULT_RETRIES,
    timeoutMs = DEFAULT_TIMEOUT_MS,
    autoRefresh = true,
    signal: externalSignal,
    ...init
  } = options;

  if (!init.credentials) {
    init.credentials = 'include';
  }

  let hasRefreshed = false;

  for (let attempt = 0; attempt <= retries; attempt++) {
    try {
      const res = await fetchWithTimeout(url, init, timeoutMs, externalSignal);

      if (autoRefresh && res.status === 401 && !hasRefreshed) {
        hasRefreshed = true;
        const refreshed = await refreshAccessToken();
        if (refreshed) {
          attempt--;
          continue;
        }
        window.location.href = '/';
        return new Response(null, { status: 401, statusText: 'Unauthorized' });
      }

      return res;
    } catch (e) {
      if (isAbortError(e)) {
        throw e;
      }
      if (attempt < retries) {
        console.warn(`Request to ${url} failed, retrying... (${attempt + 1}/${retries})`);
        await new Promise<void>((r) => setTimeout(r, RETRY_DELAY_MS));
        continue;
      }
      throw e;
    }
  }
  throw new Error('apiFetch: unreachable');
}

export type SessionResult = { ok: true } | { ok: false; status?: number; reason: 'rate_limit' | 'network' | 'server' };

export function normalizeAuthHost(): void {
  if (window.location.hostname === '127.0.0.1' && !window.__CI) {
    const port = window.location.port ? `:${window.location.port}` : '';
    window.location.replace(
      `http://localhost${port}${window.location.pathname}${window.location.search}${window.location.hash}`,
    );
  }
}

declare global {
  interface Window {
    __CI?: boolean;
  }
}

export async function establishGameSession(): Promise<SessionResult> {
  try {
    if (sessionStorage.getItem('uppy-auth-ready') === '1') {
      sessionStorage.removeItem('uppy-auth-ready');
      const quickCheck = await apiFetch('/api/v1/auth/check', { retries: 0, autoRefresh: false });
      if (quickCheck.ok) return { ok: true };
    }

    const checkRes = await apiFetch('/api/v1/auth/check', { retries: 0, autoRefresh: false });
    if (checkRes.ok) return { ok: true };

    const refreshed = await refreshAccessToken();
    if (refreshed) {
      const recheck = await apiFetch('/api/v1/auth/check', { retries: 0, autoRefresh: false });
      if (recheck.ok) return { ok: true };
    }

    const savedNick = localStorage.getItem('uppy-nickname') || '';
    const body = savedNick ? { nickname: savedNick } : {};
    // shared-002: POST is non-idempotent — never retry. A retry could create
    const res = await apiFetch('/api/v1/auth/quickplay', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      retries: 0,
      autoRefresh: false,
    });

    if (!res.ok) {
      if (res.status === 429) return { ok: false, status: 429, reason: 'rate_limit' };
      return { ok: false, status: res.status, reason: 'server' };
    }
    return { ok: true };
  } catch (e) {
    if (isAbortError(e)) {
      return { ok: false, reason: 'network' };
    }
    return { ok: false, reason: 'network' };
  }
}

export function sessionErrorMessage(session: SessionResult & { ok: false }): string {
  const reasons: Record<string, string> = {
    rate_limit: t('error.session_rate_limit'),
    network: t('error.session_network'),
    server: t('error.session_auth'),
  };
  return reasons[session.reason] || t('error.session_generic');
}
