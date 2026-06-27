const TUTORIAL_COOKIE = 'uppy-tutorial';
const TUTORIAL_MAX_AGE_SEC = 31536000;

export function isTutorialDone(): boolean {
  return document.cookie.split(';').some((c) => c.trim().startsWith(`${TUTORIAL_COOKIE}=done`));
}

export function markTutorialDone(): void {
  document.cookie = `${TUTORIAL_COOKIE}=done; path=/; max-age=${TUTORIAL_MAX_AGE_SEC}; samesite=lax`;
}

export async function shouldShowTutorial(): Promise<boolean> {
  if (isTutorialDone()) return false;
  try {
    const res = await fetch('/api/v1/user/stats', { credentials: 'include' });
    if (res.ok) {
      const data: { hasHistory?: boolean } = await res.json();
      if (data.hasHistory) return false;
    }
  } catch {
    // Anonymous or offline — show tutorial with skip option.
  }
  return true;
}
