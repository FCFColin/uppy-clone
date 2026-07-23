import { t } from '../i18n/t.js';
import { apiFetch } from '../shared/network/network.js';

export type RoomValidateResult =
  { ok: true } | { ok: false; reason: 'not_found' } | { ok: true; degraded: true };

interface RoomCheckResponse {
  phase?: string;
  degraded?: boolean;
}

export const ROOM_CODE_RE = /^[A-Z2-9]{5}$/;

export function getLobbyCodeFromUrl(): string | null {
  const code = new URLSearchParams(window.location.search).get('code');
  return code && ROOM_CODE_RE.test(code) ? code : null;
}

// RO-042: apiFetch wraps fetchWithRefresh + createFetchTimeout.
export async function matchNewRoomCode(): Promise<string | null> {
  try {
    const res = await apiFetch('/api/v1/registry/match', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      timeoutMs: 8000,
      retries: 0,
    });
    if (!res.ok) return null;
    const data = (await res.clone().json()) as { lobbyCode: string };
    return data.lobbyCode ?? null;
  } catch (e: unknown) {
    console.error('Failed to match room:', e);
    return null;
  }
}

export async function resolveLobbyCode(): Promise<string | null> {
  const fromUrl = new URLSearchParams(window.location.search).get('code');
  if (fromUrl) return fromUrl;
  return matchNewRoomCode();
}

export async function validateRoomCode(code: string): Promise<RoomValidateResult> {
  if (!ROOM_CODE_RE.test(code)) {
    return { ok: false, reason: 'not_found' };
  }
  try {
    const encoded = encodeURIComponent(code);
    const res = await apiFetch(`/api/v1/registry/check/${encoded}`, {
      timeoutMs: 8000,
      retries: 0,
      autoRefresh: false,
    });
    if (res.status === 404) {
      return { ok: false, reason: 'not_found' };
    }
    if (!res.ok) {
      return { ok: true, degraded: true };
    }
    const data = (await res.clone().json()) as RoomCheckResponse;
    if (data.degraded) {
      return { ok: true, degraded: true };
    }
    return { ok: true };
  } catch {
    return { ok: true, degraded: true };
  }
}

export function roomErrorMessage(_reason: 'not_found'): string {
  return t('error.room_not_found');
}
