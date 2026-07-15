/**
 * Unified fetch wrapper with timeout, retry, and session-expiration handling.
 *
 * RO-042: Consolidates three prior fetch mechanisms:
 *   - fetch_timeout.ts (timeout-only wrapper, deleted)
 *   - shared/network/fetch.ts fetchWithRetry (retry+timeout, deleted)
 *   - shared/network/auth.ts fetchWithRefresh (401 refresh+retry, removed)
 *
 * Features:
 *   - Configurable timeout (default 10s, aborts via AbortController)
 *   - Retry on transient network failure (default 1, configurable)
 *   - Auto-refresh on 401: attempts refreshAccessToken() once, then retries
 *   - Defaults to credentials: 'include' for cookie-based auth
 *
 * Note: refresh/logout endpoints in auth.ts use raw `fetch` to avoid
 * infinite recursion through apiFetch → refreshAccessToken → apiFetch.
 */

import { refreshAccessToken } from './auth.js';

const DEFAULT_TIMEOUT_MS = 10_000;
const DEFAULT_RETRIES = 1;
const RETRY_DELAY_MS = 500;

export interface ApiFetchOptions extends RequestInit {
  /** Number of retries for transient network failures (default: 1). 0 = no retry. */
  retries?: number;
  /** Timeout in milliseconds (default: 10000). */
  timeoutMs?: number;
  /** If true (default), refresh access token on 401 and retry once. */
  autoRefresh?: boolean;
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
          // Retry without consuming a retry slot.
          attempt--;
          continue;
        }
        // Refresh failed — redirect to login.
        window.location.href = '/';
        return new Response(null, { status: 401, statusText: 'Unauthorized' });
      }

      return res;
    } catch (e) {
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
