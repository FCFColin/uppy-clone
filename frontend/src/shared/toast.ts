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
