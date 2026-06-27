import { matchNewRoomCode } from './room_validate.js';

let errorActionsBound = false;

export function hideReconnectBanner(): void {
  const $banner: HTMLElement | null = document.getElementById('reconnect-banner');
  if ($banner) $banner.classList.add('hidden');
}

export function showReconnectBanner(attempt: number): void {
  const $banner: HTMLElement | null = document.getElementById('reconnect-banner');
  const $text: HTMLElement | null = document.getElementById('reconnect-text');
  if ($text) $text.textContent = `网络断开，正在重连…（第${attempt}次尝试）`;
  if ($banner) $banner.classList.remove('hidden');
}

export function updatePingDisplay(rttMs: number): void {
  const $ping: HTMLElement | null = document.getElementById('ping-display');
  if (!$ping) return;
  if (rttMs <= 150) {
    $ping.classList.add('hidden');
    return;
  }
  $ping.classList.remove('hidden');
  $ping.textContent = `${rttMs}ms`;
  if (rttMs > 200) {
    $ping.classList.add('ping-unstable');
  } else {
    $ping.classList.remove('ping-unstable');
  }
}

let retryBound = false;

export function bindReconnectRetry(retryFn: () => void): void {
  if (retryBound) return;
  retryBound = true;
  const btn = document.getElementById('reconnect-retry-btn');
  btn?.addEventListener('click', () => retryFn());
}

function bindErrorPanelActions(): void {
  if (errorActionsBound) return;
  errorActionsBound = true;

  const backBtn = document.getElementById('loading-back-btn');
  if (backBtn) {
    backBtn.addEventListener('click', () => {
      window.location.href = '/';
    });
  }

  const matchBtn = document.getElementById('loading-match-btn');
  if (matchBtn) {
    matchBtn.addEventListener('click', () => {
      void (async () => {
        matchBtn.setAttribute('disabled', 'true');
        const code = await matchNewRoomCode();
        if (code) {
          window.location.href = `/play.html?code=${code}`;
          return;
        }
        matchBtn.removeAttribute('disabled');
        const errorText = document.getElementById('loading-error-text');
        if (errorText) {
          errorText.textContent = '匹配失败，请稍后重试或返回大厅';
        }
      })();
    });
  }
}

export interface ConnectionErrorOptions {
  showActions?: boolean;
  title?: string;
  midGameDisconnect?: boolean;
}

function errorTitleForMessage(message: string): string {
  if (message.includes('已结束')) return '房间已结束';
  if (message.includes('不存在')) return '无法进入房间';
  if (message.includes('超时') || message.includes('网络') || message.includes('连接')) return '连接失败';
  return '无法进入房间';
}

export function showConnectionError(message: string, options?: ConnectionErrorOptions): void {
  const overlay = document.getElementById('loading-overlay');
  if (!overlay) return;
  overlay.dataset.error = 'true';
  overlay.style.display = 'flex';

  const spinner = overlay.querySelector('.loading-spinner') as HTMLElement | null;
  const loadingText = overlay.querySelector('.loading-text') as HTMLElement | null;
  const errorPanel = document.getElementById('loading-error-panel');
  const errorTitle = document.getElementById('loading-error-title');
  const errorText = document.getElementById('loading-error-text');
  const actions = document.getElementById('loading-error-actions');

  if (spinner) spinner.classList.add('hidden');
  if (loadingText) loadingText.classList.add('hidden');
  if (errorTitle) {
    errorTitle.textContent = options?.title
      ?? (options?.midGameDisconnect ? '对局连接中断' : errorTitleForMessage(message));
  }
  if (errorText) errorText.textContent = message;
  if (errorPanel) errorPanel.classList.remove('hidden');
  if (actions) {
    if (options?.showActions) {
      actions.classList.remove('hidden');
    } else {
      actions.classList.add('hidden');
    }
  }

  bindErrorPanelActions();
  hideReconnectBanner();
}
