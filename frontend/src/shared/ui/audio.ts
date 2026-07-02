let audioCtx: AudioContext | null = null;

type SoundName = 'tap' | 'ready' | 'gameover' | 'countdown';

const buffers = {} as Record<SoundName, AudioBuffer | null>;

function ctx(): AudioContext | null {
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

async function loadSound(name: SoundName, url: string): Promise<void> {
  const ac = ctx();
  if (!ac) return;
  try {
    const res = await fetch(url);
    const data = await res.arrayBuffer();
    buffers[name] = await ac.decodeAudioData(data);
  } catch {
    buffers[name] = null;
  }
}

function playSound(name: SoundName, volume = 0.5): void {
  const ac = ctx();
  if (!ac) return;
  const buf = buffers[name];
  if (!buf) return;
  const src = ac.createBufferSource();
  const g = ac.createGain();
  src.buffer = buf;
  g.gain.value = volume;
  src.connect(g);
  g.connect(ac.destination);
  src.start();
}

const loaded = false;
export function ensureSoundsLoaded(): void {
  if (loaded) return;
  loadSound('tap', '/assets/sounds/tap.ogg');
  loadSound('ready', '/assets/sounds/ready.ogg');
  loadSound('gameover', '/assets/sounds/gameover.ogg');
  loadSound('countdown', '/assets/sounds/countdown.ogg');
}

export function playTapSound(): void { ensureSoundsLoaded(); playSound('tap', 0.3); }
export function playReadySound(): void { ensureSoundsLoaded(); playSound('ready', 0.35); }
export function playGameOverSound(): void { ensureSoundsLoaded(); playSound('gameover', 0.4); }
export function playCountdownTick(): void { ensureSoundsLoaded(); playSound('countdown', 0.25); }

export function vibrate(pattern: number | number[]): void {
  if (typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
  try {
    navigator.vibrate?.(pattern);
  } catch {
    // Vibrate not supported
  }
}

export function resumeAudioContext(): void {
  ensureSoundsLoaded();
  void ctx()?.resume();
}
