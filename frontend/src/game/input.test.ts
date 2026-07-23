import { describe, it, expect, vi, beforeEach } from 'vitest';

const inputMockState = vi.hoisted(() => ({
  phase: 'playing',
  myCooldownEnd: 0,
  ripples: [] as Array<Record<string, unknown>>,
  lastTapX: 0,
  lastTapY: 0,
  players: [{ nickname: 'p1' }],
  restartVotes: { yes: 0, total: 1, countdownMs: 0 },
  restartClicked: false,
  balloon: { x: 0.5, y: 0.5 },
}));
vi.mock('./state.js', async (importActual) => {
  const actual = (await importActual()) as Record<string, unknown>;
  const gameReducer = actual.gameReducer as (state: unknown, action: unknown) => unknown;
  return {
    ...actual,
    state: inputMockState,
    getState: () => inputMockState,
    dispatch: (action: Record<string, unknown>) => {
      const next = gameReducer(inputMockState, action);
      if (next !== inputMockState) Object.assign(inputMockState, next);
    },
  };
});
vi.mock('./ws_connection.js', () => ({
  sendOrQueue: vi.fn(),
  getWs: vi.fn(() => ({ readyState: 1 })),
}));
vi.mock('./renderer.js', () => ({
  $canvas: { getBoundingClientRect: () => ({ left: 0, top: 0, width: 400, height: 300 }) },
  clientToNormalized: (x: number, y: number) => ({ x: x / 100, y: y / 100 }),
}));
vi.mock('./visual_helpers.js', () => ({
  pushFloatingText: vi.fn(),
}));
vi.mock('../shared/ui/ui.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../shared/ui/ui.js')>();
  return {
    ...actual,
    playTapSound: vi.fn(),
    vibrate: vi.fn(),
  };
});
vi.mock('./ui_update.js', () => ({ updateUI: vi.fn() }));

import { handleTap, requestRestart, tapAtBalloonCenter } from './input.js';
import { state } from './state.js';
import { sendOrQueue, getWs } from './ws_connection.js';

describe('input', () => {
  beforeEach(() => {
    state.phase = 'playing';
    state.myCooldownEnd = 0;
    state.ripples = [];
    vi.clearAllMocks();
  });

  it('handleTap uses player count for optimistic cooldown', () => {
    state.players = [
      { playerIndex: 0, nickname: 'a', palette: 0, cooldownEndTime: 0, scoreContribution: 0 },
      { playerIndex: 1, nickname: 'b', palette: 0, cooldownEndTime: 0, scoreContribution: 0 },
    ];
    handleTap(50, 50);
    expect(state.myCooldownEnd).toBeGreaterThan(Date.now());
    expect(sendOrQueue).toHaveBeenCalledOnce();
  });

  it('handleTap sends binary tap message when playing', () => {
    handleTap(50, 50);
    expect(sendOrQueue).toHaveBeenCalledOnce();
    const call = vi.mocked(sendOrQueue).mock.calls[0];
    expect(call).toBeDefined();
    const buf = call![0] as ArrayBuffer;
    expect(new DataView(buf).getUint8(0)).toBe(0x10);
  });

  it('handleTap rejects during cooldown and ignores non-playing phase', () => {
    // Rejects during cooldown
    state.myCooldownEnd = Date.now() + 5000;
    handleTap(10, 10);
    expect(sendOrQueue).not.toHaveBeenCalled();
    expect(state.ripples.some((r) => r.rejected)).toBe(true);
    // Ignores non-playing phase
    state.phase = 'waiting';
    state.myCooldownEnd = 0;
    state.ripples = [];
    handleTap(10, 10);
    expect(sendOrQueue).not.toHaveBeenCalled();
  });

  it('requestRestart sends vote when game ended', () => {
    state.phase = 'ended';
    requestRestart();
    expect(sendOrQueue).toHaveBeenCalledOnce();
  });

  it('requestRestart shows message when websocket closed or game has not ended', () => {
    // WebSocket closed
    vi.mocked(getWs).mockReturnValue(null);
    document.body.innerHTML = '<div id="restart-progress"></div>';
    state.phase = 'ended';
    requestRestart();
    expect(sendOrQueue).not.toHaveBeenCalled();
    expect(document.getElementById('restart-progress')?.textContent).toContain('断开');
    // Game not ended
    state.phase = 'playing';
    requestRestart();
    expect(sendOrQueue).not.toHaveBeenCalled();
    expect(document.getElementById('restart-progress')?.textContent).toContain('尚未结束');
  });

  it('tapAtBalloonCenter sends tap at balloon position, no-ops outside playing phase', () => {
    document.body.innerHTML = '<canvas id="game-canvas" width="100" height="100"></canvas>';
    const canvas = document.getElementById('game-canvas')!;
    canvas.getBoundingClientRect = () => ({
      left: 0,
      top: 0,
      width: 100,
      height: 100,
      right: 100,
      bottom: 100,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    });
    tapAtBalloonCenter();
    expect(sendOrQueue).toHaveBeenCalledOnce();
    // No-op outside playing phase
    state.phase = 'waiting';
    tapAtBalloonCenter();
    expect(sendOrQueue).toHaveBeenCalledOnce();
  });
});
