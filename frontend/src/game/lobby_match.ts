import { apiFetch } from '../shared/network/api_fetch.js';

export async function resolveLobbyCode(): Promise<string | null> {
  const params: URLSearchParams = new URLSearchParams(window.location.search);
  const fromUrl: string | null = params.get('code');
  if (fromUrl) return fromUrl;

  // RO-042: apiFetch replaces fetchWithRefresh + createFetchTimeout.
  // timeoutMs=8000 aligns with lifecycle.ts connection timeout (v2-R-14/46/50).
  // retries=0 preserves fetchWithRefresh behavior (no network-error retry).
  try {
    const res: Response = await apiFetch('/api/v1/registry/match', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      timeoutMs: 8000,
      retries: 0,
    });
    if (res.ok) {
      const data: { lobbyCode: string } = await res.json() as { lobbyCode: string };
      return data.lobbyCode;
    }
  } catch (e: unknown) {
    console.error('Failed to match room:', e);
  }
  return null;
}
