/** Storage may be unavailable (private browsing, quota). Wrap access in try/catch. */

export function safeGetItem(key: string, storage: Storage = localStorage): string | null {
  try {
    return storage.getItem(key);
  } catch {
    return null;
  }
}

export function safeSetItem(key: string, value: string, storage: Storage = localStorage): void {
  try {
    storage.setItem(key, value);
  } catch {
    // storage may be unavailable (private browsing, quota)
  }
}

let toastTimer: ReturnType<typeof setTimeout> | null = null;

export function showToast(message: string, durationMs = 2000): void {
  let el = document.getElementById('app-toast');
  if (!el) {
    el = document.createElement('div');
    el.id = 'app-toast';
    el.className = 'app-toast';
    document.body.appendChild(el);
  }
  el.textContent = message;
  el.classList.add('visible');
  if (toastTimer !== null) clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    el?.classList.remove('visible');
    toastTimer = null;
  }, durationMs);
}