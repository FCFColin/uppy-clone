import { state } from './state_types.js';
import { pickRandomNickname } from './ui_elements.js';
import { showToast } from '../shared/ui/toast.js';
import { playCountdownTick } from '../shared/ui/audio.js';
import { resizeCanvas } from './renderer.js';

export {
  $waitingScreen, $endedScreen, $gameHud,
  $lobbyCode, $hudCode, $copyCodeBtn, $hudCopyBtn,
  $nicknameSetupScreen, $setupNicknameInput,
} from './ui_elements.js';

export { updateUI } from './ui_update.js';
export { startCooldownUpdater, stopCooldownUpdater, updateCooldownBar } from './ui_cooldown.js';

export function startCountdownTimer(seconds: number): void {
  if (state.countdownTimerInterval !== null) {
    clearInterval(state.countdownTimerInterval);
    state.countdownTimerInterval = null;
  }
  const countdownEl: HTMLElement | null = document.getElementById('countdown-overlay');
  if (!countdownEl) return;
  const numberEl: Element | null = countdownEl.querySelector('.countdown-number');
  let remaining: number = seconds;
  if (numberEl) {
    numberEl.textContent = String(remaining);
    numberEl.classList.remove('countdown-pop');
    void numberEl.clientWidth;
    numberEl.classList.add('countdown-pop');
  }
  playCountdownTick();
  countdownEl.classList.remove('hidden');

  state.countdownTimerInterval = setInterval(() => {
    remaining--;
    if (remaining <= 0) {
      clearInterval(state.countdownTimerInterval!);
      state.countdownTimerInterval = null;
      countdownEl.classList.add('hidden');
    } else {
      if (numberEl) {
        numberEl.textContent = String(remaining);
        numberEl.classList.remove('countdown-pop');
        void numberEl.clientWidth;
        numberEl.classList.add('countdown-pop');
      }
      playCountdownTick();
    }
  }, 1000);
}

export function hideCountdownOverlay(): void {
  document.getElementById('countdown-overlay')?.classList.add('hidden');
}

export function showCountdownOverlay(): void {
  document.getElementById('countdown-overlay')?.classList.remove('hidden');
}

export function generateRandomNickname(): string {
  return pickRandomNickname();
}

export async function copyCode(): Promise<void> {
  const url: string = `${window.location.origin}/play.html?code=${state.lobbyCode}`;
  try {
    await navigator.clipboard.writeText(url);
    showToast('已复制邀请链接');
  } catch {
    showToast('复制失败，请手动复制房间码');
  }
}

export function refreshLayout(): void {
  resizeCanvas();
}

export function showFallbackErrorScreen(message: string): void {
  if (document.getElementById('game-fallback-error')) return;
  const overlay: HTMLDivElement = document.createElement('div');
  overlay.id = 'game-fallback-error';
  overlay.style.cssText = 'position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.8);z-index:99999;display:flex;align-items:center;justify-content:center;flex-direction:column;color:#fff;font-family:sans-serif;';

  const h2: HTMLHeadingElement = document.createElement('h2');
  h2.style.marginBottom = '1rem';
  h2.textContent = '\u{1F635} 出错了';

  const p: HTMLParagraphElement = document.createElement('p');
  p.style.cssText = 'margin-bottom:1.5rem;color:#ccc;';
  p.textContent = message;

  const btn: HTMLButtonElement = document.createElement('button');
  btn.style.cssText = 'padding:0.8rem 2rem;font-size:1rem;cursor:pointer;border:none;border-radius:8px;background:#0f3460;color:#fff;';
  btn.textContent = '刷新页面';
  btn.onclick = () => location.reload();

  overlay.appendChild(h2);
  overlay.appendChild(p);
  overlay.appendChild(btn);
  document.body.appendChild(overlay);
}
