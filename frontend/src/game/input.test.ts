import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('./state.js', () => ({
  state: {
    phase: 'playing',
    myCooldownEnd: 0,
    ripples: [] as Array<Record<string, unknown>>,
    lastTapX: 0,
    lastTapY: 0,
    players: [{ nickname: 'p1' }],
    restartVotes: { yes: 0, total: 1, countdownMs: 0 },
    restartClicked: false,
  },
}));
vi.mock('./websocket.js', () => ({
  sendOrQueue: vi.fn(),
  getWs: vi.fn(() => ({ readyState: 1 })),
}));
vi.mock('./renderer.js', () => ({
  $canvas: {
    getBoundingClientRect: () => ({ left: 0, top: 0, width: 100, height: 100 }),
  },
}));
vi.mock('./ui.js', () => ({ updateUI: vi.fn() }));

import { handleTap, requestRestart } from './input.js';
import { state } from './state.js';
import { sendOrQueue, getWs } from './websocket.js';

describe('input', () => {
  beforeEach(() => {
    state.phase = 'playing';
    state.myCooldownEnd = 0;
    state.ripples = [];
    vi.clearAllMocks();
  });

  it('handleTap sends binary tap message when playing', () => {
    handleTap(50, 50);
    expect(sendOrQueue).toHaveBeenCalledOnce();
    const call = vi.mocked(sendOrQueue).mock.calls[0];
    expect(call).toBeDefined();
    const buf = call![0] as ArrayBuffer;
    expect(new DataView(buf).getUint8(0)).toBe(0x10);
  });

  it('handleTap rejects during cooldown', () => {
    state.myCooldownEnd = Date.now() + 5000;
    handleTap(10, 10);
    expect(sendOrQueue).not.toHaveBeenCalled();
    expect(state.ripples.some((r) => r.rejected)).toBe(true);
  });

  it('handleTap ignores non-playing phase', () => {
    state.phase = 'waiting';
    handleTap(10, 10);
    expect(sendOrQueue).not.toHaveBeenCalled();
  });

  it('requestRestart sends vote when game ended', () => {
    state.phase = 'ended';
    requestRestart();
    expect(sendOrQueue).toHaveBeenCalledOnce();
  });

  it('requestRestart shows message when websocket closed', () => {
    vi.mocked(getWs).mockReturnValue(null);
    document.body.innerHTML = '<div id="restart-progress"></div>';
    state.phase = 'ended';
    requestRestart();
    expect(sendOrQueue).not.toHaveBeenCalled();
    expect(document.getElementById('restart-progress')?.textContent).toContain('断开');
  });
});
