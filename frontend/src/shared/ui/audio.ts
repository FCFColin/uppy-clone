let audioCtx: AudioContext | null = null;

function ctx(): AudioContext | null {
  if (typeof window === 'undefined') return null;
  if (!audioCtx) {
    try {
      audioCtx = new AudioContext();
    } catch {
      return null;
    }
  }
  return audioCtx;
}

const MUTE_STORAGE_KEY = 'uppy-audio-muted';

export function isMuted(): boolean {
  try {
    return localStorage.getItem(MUTE_STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}

export function setMuted(muted: boolean): void {
  try {
    if (muted) {
      localStorage.setItem(MUTE_STORAGE_KEY, '1');
    } else {
      localStorage.removeItem(MUTE_STORAGE_KEY);
    }
  } catch {
    // localStorage 不可用时静默失败
  }
}

export function toggleMute(): boolean {
  const newMuted = !isMuted();
  setMuted(newMuted);
  return newMuted;
}

function playSoundFile(path: string): void {
  try {
    const audio = new Audio(path);
    audio.play().catch(() => {
      // 自动播放被阻止或文件加载失败，静默处理
    });
  } catch {
    // Audio 构造失败，静默处理
  }
}

export function playTapSound(): void {
  if (isMuted()) return;
  playSoundFile('/assets/sounds/tap.ogg');
}
export function playReadySound(): void {
  if (isMuted()) return;
  playSoundFile('/assets/sounds/ready.ogg');
}
export function playGameOverSound(): void {
  if (isMuted()) return;
  playSoundFile('/assets/sounds/gameover.ogg');
}
export function playCountdownTick(): void {
  if (isMuted()) return;
  playSoundFile('/assets/sounds/countdown.ogg');
}

export function vibrate(pattern: number | number[]): void {
  if (typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
  try {
    navigator.vibrate?.(pattern);
  } catch {
    // unsupported
  }
}

export function resumeAudioContext(): void {
  void ctx()?.resume();
}
