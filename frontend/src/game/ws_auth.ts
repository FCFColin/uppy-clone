import { establishGameSession } from '../shared/session.js';

export async function ensureAuth(): Promise<boolean> {
  const result = await establishGameSession();
  if (!result.ok) {
    console.error('Auth failed:', result.reason, result.status ?? '');
  }
  return result.ok;
}
