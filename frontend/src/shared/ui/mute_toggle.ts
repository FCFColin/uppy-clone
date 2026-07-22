import { isMuted, toggleMute } from './audio.js';
import { icons } from '../../icons.js';

const MUTE_TOGGLE_ID = 'mute-toggle';
const BOUND_ATTR = 'data-bound';

/** 查找静音按钮节点，未找到时返回 null。 */
function getButton(): HTMLElement | null {
  return document.getElementById(MUTE_TOGGLE_ID);
}

/**
 * 初始化静音按钮：绑定 click 事件并刷新图标。
 * 多次调用幂等（已绑定过的按钮不会重复绑定）。
 * 按钮不存在时静默返回，不抛错。
 */
export function initMuteToggle(): void {
  const btn = getButton();
  if (!btn) return;
  if (btn.hasAttribute(BOUND_ATTR)) return;
  btn.setAttribute(BOUND_ATTR, 'true');
  btn.addEventListener('click', () => {
    toggleMute();
    updateMuteToggleIcon();
  });
  updateMuteToggleIcon();
}

/** 根据 isMuted() 同步按钮图标：静音显示 VolumeX，非静音显示 Volume2。按钮不存在时静默返回。 */
export function updateMuteToggleIcon(): void {
  const btn = getButton();
  if (!btn) return;
  btn.innerHTML = isMuted() ? icons.VolumeX({ size: 18 }) : icons.Volume2({ size: 18 });
  btn.setAttribute('aria-label', isMuted() ? '开启声音' : '静音');
  btn.setAttribute('title', isMuted() ? '开启声音' : '静音');
}
