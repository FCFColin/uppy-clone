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
  const actual = await importActual() as any;
  return {
    ...actual,
    state: inputMockState,
    getState: () => inputMockState,
    dispatch: (action: any) => {
      const next = actual.gameReducer(inputMockState, action);
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
vi.mock('../shared/ui/audio.js', () => ({
  playTapSound: vi.fn(),
  vibrate: vi.fn(),
}));
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

  it('requestRestart shows message when game has not ended', () => {
    document.body.innerHTML = '<div id="restart-progress"></div>';
    state.phase = 'playing';
    requestRestart();
    expect(sendOrQueue).not.toHaveBeenCalled();
    expect(document.getElementById('restart-progress')?.textContent).toContain('尚未结束');
  });

  it('requestRestart submits vote when socket is open', () => {
    document.body.innerHTML = '<div id="restart-progress"></div>';
    state.phase = 'ended';
    vi.mocked(getWs).mockReturnValue({ readyState: 1 } as WebSocket);
    requestRestart();
    expect(sendOrQueue).toHaveBeenCalledOnce();
    expect(document.getElementById('restart-progress')?.textContent).toContain('提交');
  });

  it('tapAtBalloonCenter sends tap at balloon position', () => {
    document.body.innerHTML = '<canvas id="game-canvas" width="100" height="100"></canvas>';
    const canvas = document.getElementById('game-canvas')!;
    canvas.getBoundingClientRect = () => ({
      left: 0, top: 0, width: 100, height: 100,
      right: 100, bottom: 100, x: 0, y: 0, toJSON: () => ({}),
    });
    tapAtBalloonCenter();
    expect(sendOrQueue).toHaveBeenCalledOnce();
  });

  it('tapAtBalloonCenter no-ops outside playing phase', () => {
    document.body.innerHTML = '<canvas id="game-canvas" width="100" height="100"></canvas>';
    state.phase = 'waiting';
    tapAtBalloonCenter();
    expect(sendOrQueue).not.toHaveBeenCalled();
  });
});
