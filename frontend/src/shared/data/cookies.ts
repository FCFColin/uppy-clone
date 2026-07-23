import { apiFetch } from '../network/network.js';

const MAX_AGE_SEC = 31536000;
const BEST_SCORE_COOKIE = 'uppy-best-score';
const TUTORIAL_COOKIE = 'uppy-tutorial';

function setCookie(name: string, value: string): void {
  const secure = window.location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${name}=${value}; path=/; max-age=${MAX_AGE_SEC}; samesite=lax${secure}`;
}

export function getCookieBestScore(): number {
  const match = document.cookie.match(new RegExp(`(?:^|; )${BEST_SCORE_COOKIE}=([^;]*)`));
  if (!match?.[1]) return 0;
  const n = parseInt(match[1], 10);
  return Number.isFinite(n) ? n : 0;
}

export function setCookieBestScore(score: number): void {
  setCookie(BEST_SCORE_COOKIE, String(score));
}

export async function fetchUserBestScore(): Promise<number> {
  try {
    const res = await apiFetch('/api/v1/user/stats');
    if (!res.ok) return getCookieBestScore();
    const data: { bestScore?: number } = await res.json();
    const apiScore = data.bestScore ?? 0;
    // shared-004: Write back to cookie so subsequent loads skip the network round-trip.
    if (apiScore > getCookieBestScore()) setCookieBestScore(apiScore);
    return apiScore;
  } catch {
    return getCookieBestScore();
  }
}

export function updateBestScore(currentScore: number): { best: number; isNewRecord: boolean } {
  const prev = getCookieBestScore();
  if (currentScore > prev) {
    setCookieBestScore(currentScore);
    return { best: currentScore, isNewRecord: true };
  }
  return { best: prev, isNewRecord: false };
}

export function isTutorialDone(): boolean {
  return document.cookie.split(';').some((c) => c.trim().startsWith(`${TUTORIAL_COOKIE}=done`));
}

export function markTutorialDone(): void {
  setCookie(TUTORIAL_COOKIE, 'done');
}

export async function shouldShowTutorial(): Promise<boolean> {
  if (isTutorialDone()) return false;
  try {
    const res = await apiFetch('/api/v1/user/stats');
    if (res.ok) {
      const data: { hasHistory?: boolean } = await res.json();
      if (data.hasHistory) return false;
    }
  } catch {
    // ignore — non-critical operation
  }
  return true;
}
