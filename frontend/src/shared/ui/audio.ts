let audioCtx: AudioContext | null = null;
const muted = false;

function ctx(): AudioContext | null {
  if (muted) return null;
  if (typeof window === 'undefined') return null;
  if (typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches) return null;
  if (!audioCtx) {
    try {
      audioCtx = new AudioContext();
    } catch {
      return null;
    }
  }
  return audioCtx;
}

function beep(freq: number, durationMs: number, gain = 0.08): void {
  const ac = ctx();
  if (!ac) return;
  const osc = ac.createOscillator();
  const g = ac.createGain();
  osc.type = 'sine';
  osc.frequency.value = freq;
  g.gain.value = gain;
  osc.connect(g);
  g.connect(ac.destination);
  osc.start();
  osc.stop(ac.currentTime + durationMs / 1000);
}

export function playTapSound(): void { beep(520, 60, 0.06); }
export function playReadySound(): void { beep(880, 80, 0.07); }
export function playGameOverSound(): void { beep(220, 200, 0.1); }
export function playCountdownTick(): void { beep(440, 50, 0.05); }

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
