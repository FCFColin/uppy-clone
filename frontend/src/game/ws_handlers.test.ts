import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MSG_TYPE, PHASE_CODE } from '../shared/game/constants.js';
import { buildMinimalSnapshot } from './test_fixtures/snapshot.js';
import { getMocks, resetWsHandlersMocks, createStateJsMockModule } from './ws_handlers_test_setup.js';

const mocks = getMocks();

vi.mock('./state.js', async (importActual) => createStateJsMockModule(importActual as any));

vi.mock('./state_interp.js', async () => {
  const m = getMocks();
  return {
    updateInterpolation: m.updateInterpolation,
    freezeInterpolation: m.freezeInterpolation,
    isDuplicateSeq: m.isDuplicateSeq,
  };
});

vi.mock('./phase_sync.js', async () => {
  const m = getMocks();
  return {
    applyPhaseChange: m.applyPhaseChange,
    shouldApplySnapshotPhase: m.shouldApplySnapshotPhase,
  };
});

vi.mock('./ui_update.js', async () => {
  const m = getMocks();
  return { updateUI: m.updateUI, updateScoresOnly: m.updateScoresOnly };
});

vi.mock('./ui_common.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./ui_common.js')>();
  const m = getMocks();
  return { ...actual, updateWindIndicator: m.updateWindIndicator };
});

vi.mock('./tutorial.js', () => ({ runTutorialIfNeeded: vi.fn(() => Promise.resolve()) }));
vi.mock('../shared/ui/audio.js', () => ({ playGameOverSound: vi.fn(), vibrate: vi.fn() }));
vi.mock('../shared/data/cookies.js', () => ({
  updateBestScore: vi.fn((score: number) => ({ best: score - 10, isNewRecord: false })),
  fetchUserBestScore: vi.fn(() => Promise.resolve(999)),
}));
vi.mock('./visual_helpers.js', () => ({ pushFloatingText: vi.fn() }));
vi.mock('./restart_vote_ui.js', async () => {
  const m = getMocks();
  return { syncRestartVoteUI: m.syncRestartVoteUI };
});
vi.mock('./seen_seqs.js', async () => {
  const m = getMocks();
  return { isDuplicateSeq: m.isDuplicateSeq };
});
vi.mock('./message_codec.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./message_codec.js')>();
  return { ...actual, decodeSnapshot: vi.fn(actual.decodeSnapshot) };
});

import {
  handleBinaryMessage,
  handleTapAccepted,
  handleTapRejected,
  handleGameStateChange,
  handleRestartStatus,
  handleSnapshot,
} from './ws_handlers.js';
import { decodeSnapshot } from './message_codec.js';
import { playGameOverSound } from '../shared/ui/audio.js';
import { pushFloatingText } from './visual_helpers.js';

describe('handleBinaryMessage routing', () => {
  beforeEach(() => {
    resetWsHandlersMocks();
    mocks.state.lastTapX = 0.5;
    mocks.state.lastTapY = 0.5;
  });

  it('routes snapshot messages to snapshot handler', () => {
    handleBinaryMessage(buildMinimalSnapshot(PHASE_CODE.PLAYING));
    expect(mocks.state.score).toBe(42);
    expect(mocks.state.hasReceivedFirstSnapshot).toBe(true);
  });

  it('routes tap accepted messages', () => {
    const buf = new ArrayBuffer(18);
    const dv = new DataView(buf);
    dv.setUint8(0, MSG_TYPE.TAP_ACCEPTED);
    dv.setUint16(1, 2, true);
    dv.setUint32(3, 800, true);
    dv.setFloat32(7, 0.3, true);
    dv.setFloat32(11, 0.7, true);
    handleBinaryMessage(buf);
    expect(mocks.state.myCooldownEnd).toBeGreaterThan(Date.now());
    expect(mocks.state.ripples.some((r) => r.playerIndex === 2)).toBe(true);
  });

  it('routes tap rejected messages', () => {
    const buf = new ArrayBuffer(1);
    new DataView(buf).setUint8(0, MSG_TYPE.TAP_REJECTED);
    handleBinaryMessage(buf);
    expect(mocks.state.ripples.some((r) => r.rejected)).toBe(true);
    expect(pushFloatingText).toHaveBeenCalled();
  });

  it('routes game state change messages', () => {
    const buf = new ArrayBuffer(2);
    const dv = new DataView(buf);
    dv.setUint8(0, MSG_TYPE.GAME_STATE_CHANGE);
    dv.setUint8(1, PHASE_CODE.PLAYING);
    handleBinaryMessage(buf);
    expect(mocks.applyPhaseChange).toHaveBeenCalledWith('playing', 3);
  });

  it('routes ended game state with end reason', () => {
    const buf = new ArrayBuffer(3);
    const dv = new DataView(buf);
    dv.setUint8(0, MSG_TYPE.GAME_STATE_CHANGE);
    dv.setUint8(1, PHASE_CODE.ENDED);
    dv.setUint8(2, 1);
    handleBinaryMessage(buf);
    expect(playGameOverSound).toHaveBeenCalled();
    expect(mocks.state.endReason).toBe(1);
  });

  it('routes restart status messages', () => {
    const buf = new ArrayBuffer(7);
    const dv = new DataView(buf);
    dv.setUint8(0, MSG_TYPE.RESTART_STATUS);
    dv.setUint8(1, 2);
    dv.setUint8(2, 4);
    dv.setUint32(3, 4000, true);
    handleBinaryMessage(buf);
    expect(mocks.state.restartVotes.yes).toBe(2);
    expect(mocks.syncRestartVoteUI).toHaveBeenCalled();
  });

  it('routes pong messages without throwing', () => {
    const buf = new ArrayBuffer(1);
    new DataView(buf).setUint8(0, MSG_TYPE.PONG);
    expect(() => handleBinaryMessage(buf)).not.toThrow();
  });

  it('ignores player join and leave opcodes', () => {
    for (const msgType of [MSG_TYPE.PLAYER_JOIN, MSG_TYPE.PLAYER_LEAVE]) {
      const buf = new ArrayBuffer(1);
      new DataView(buf).setUint8(0, msgType);
      expect(() => handleBinaryMessage(buf)).not.toThrow();
    }
  });

  it('ignores unknown message types', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const buf = new ArrayBuffer(1);
    new DataView(buf).setUint8(0, 0xff);
    expect(() => handleBinaryMessage(buf)).not.toThrow();
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });
});

describe('ws_handlers_events', () => {
  beforeEach(() => {
    resetWsHandlersMocks();
    mocks.state.lastTapX = 0.4;
    mocks.state.lastTapY = 0.6;
  });

  it('handleTapAccepted updates cooldown and ripples', () => {
    mocks.state.ripples = [{ isOptimistic: true }];
    const buf = new ArrayBuffer(18);
    const view = new DataView(buf);
    view.setUint8(0, MSG_TYPE.TAP_ACCEPTED);
    view.setUint16(1, 1, true);
    view.setUint32(3, 500, true);
    view.setFloat32(7, 0.5, true);
    view.setFloat32(11, 0.5, true);
    handleTapAccepted(view);
    expect(mocks.state.myCooldownEnd).toBeGreaterThan(Date.now());
    expect(mocks.state.ripples.some((r) => r.playerIndex === 1)).toBe(true);
  });

  it('handleTapAccepted falls back to server balloon coordinates', () => {
    mocks.state.lastTapX = null;
    mocks.state.lastTapY = null;
    const buf = new ArrayBuffer(18);
    const view = new DataView(buf);
    view.setUint8(0, MSG_TYPE.TAP_ACCEPTED);
    view.setUint16(1, 2, true);
    view.setUint32(3, 250, true);
    view.setFloat32(7, 0.7, true);
    view.setFloat32(11, 0.8, true);
    handleTapAccepted(view);
    expect(mocks.state.ripples.at(-1)).toMatchObject({ playerIndex: 2 });
    expect(mocks.state.ripples.at(-1)!.x).toBeCloseTo(0.7, 5);
    expect(mocks.state.ripples.at(-1)!.y).toBeCloseTo(0.8, 5);
  });

  it('handleTapRejected clears optimistic cooldown', () => {
    mocks.state.myCooldownEnd = Date.now() + 5000;
    mocks.state.ripples = [{ isOptimistic: true }];
    handleTapRejected();
    expect(mocks.state.myCooldownEnd).toBe(0);
    expect(mocks.state.ripples.some((r) => r.isOptimistic)).toBe(false);
  });

  it('handleTapRejected adds rejected ripple and floating text', () => {
    handleTapRejected();
    expect(mocks.state.ripples.some((r) => r.rejected)).toBe(true);
    expect(pushFloatingText).toHaveBeenCalledWith(0.4, 0.6, '太远了');
  });

  it('handleTapRejected is a no-op without last tap coordinates', () => {
    mocks.state.lastTapX = null;
    handleTapRejected();
    expect(mocks.state.ripples).toHaveLength(0);
    expect(pushFloatingText).not.toHaveBeenCalled();
  });
});

describe('handleGameStateChange', () => {
  beforeEach(() => {
    resetWsHandlersMocks();
  });

  it('applies playing phase from binary message', () => {
    const buf = new ArrayBuffer(2);
    const dv = new DataView(buf);
    dv.setUint8(0, 0);
    dv.setUint8(1, 1);
    handleGameStateChange(dv);
    expect(mocks.applyPhaseChange).toHaveBeenCalledWith('playing', 3);
  });

  it('skips blocked phase regression', () => {
    mocks.shouldApplySnapshotPhase.mockReturnValue(false);
    const buf = new ArrayBuffer(2);
    new DataView(buf).setUint8(1, 1);
    handleGameStateChange(new DataView(buf));
    expect(mocks.applyPhaseChange).not.toHaveBeenCalled();
  });

  it('derives countdown seconds from remaining ms', async () => {
    const buf = new ArrayBuffer(6);
    const dv = new DataView(buf);
    dv.setUint8(1, 3);
    dv.setUint32(2, 5500, true);
    handleGameStateChange(dv);
    await vi.waitFor(() => {
      expect(mocks.applyPhaseChange).toHaveBeenCalledWith('countdown', 6);
    });
  });

  it('handles ended phase with end reason and score banner', async () => {
    mocks.state.score = 88;
    document.body.innerHTML = '<div id="personal-best"></div>';
    const buf = new ArrayBuffer(3);
    const dv = new DataView(buf);
    dv.setUint8(1, 2);
    dv.setUint8(2, 1);
    handleGameStateChange(dv);
    expect(mocks.state.endReason).toBe(1);
    await vi.waitFor(() => {
      expect(document.getElementById('personal-best')?.textContent).toContain('本局 88');
      expect(document.getElementById('personal-best')?.textContent).toContain('个人最佳 999');
    });
  });

  it('marks new record when cookie best is fresh', async () => {
    const bestScoreModule = await import('../shared/data/cookies.js');
    vi.mocked(bestScoreModule.updateBestScore).mockReturnValueOnce({ best: 50, isNewRecord: true });
    mocks.state.score = 88;
    document.body.innerHTML = '<div id="personal-best"></div>';
    const dv = new DataView(new ArrayBuffer(3));
    dv.setUint8(1, 2);
    dv.setUint8(2, 1);
    handleGameStateChange(dv);
    await vi.waitFor(() => {
      expect(document.getElementById('personal-best')?.textContent).toContain('新纪录');
    });
  });

  it('shows new record when score exceeds server best', async () => {
    const bestScoreModule = await import('../shared/data/cookies.js');
    vi.mocked(bestScoreModule.updateBestScore).mockReturnValueOnce({ best: 10, isNewRecord: false });
    vi.mocked(bestScoreModule.fetchUserBestScore).mockResolvedValueOnce(5);
    mocks.state.score = 88;
    document.body.innerHTML = '<div id="personal-best"></div>';
    const dv = new DataView(new ArrayBuffer(3));
    dv.setUint8(1, 2);
    dv.setUint8(2, 1);
    handleGameStateChange(dv);
    await vi.waitFor(() => {
      expect(document.getElementById('personal-best')?.textContent).toContain('新纪录');
    });
  });

  it('skips end-screen update when personal-best element is missing', async () => {
    document.body.innerHTML = '';
    const buf = new ArrayBuffer(3);
    const dv = new DataView(buf);
    dv.setUint8(1, 2);
    dv.setUint8(2, 1);
    handleGameStateChange(dv);
    await Promise.resolve();
    expect(document.getElementById('personal-best')).toBeNull();
  });
});

describe('handleRestartStatus', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    resetWsHandlersMocks();
    mocks.state.phase = 'ended';
    window._restartCountdownTimer = null;
    document.body.innerHTML = '<div id="restart-progress"></div><div id="restart-countdown"></div>';
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('updates restart vote state and UI', () => {
    const buf = new ArrayBuffer(7);
    const dv = new DataView(buf);
    dv.setUint8(1, 2);
    dv.setUint8(2, 3);
    dv.setUint32(3, 5000, true);
    handleRestartStatus(dv);
    expect(mocks.state.restartVotes.yes).toBe(2);
    expect(mocks.state.restartVotes.total).toBe(3);
    expect(mocks.updateUI).toHaveBeenCalledWith({ force: true });
  });
});

describe('handleSnapshot', () => {
  beforeEach(() => {
    resetWsHandlersMocks();
  });

  it('ignores messages shorter than 44 bytes', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    handleSnapshot(new DataView(new ArrayBuffer(10)));
    expect(warn).toHaveBeenCalled();
    expect(mocks.state.hasReceivedFirstSnapshot).toBe(false);
    warn.mockRestore();
  });

  it('parses score, balloon, and phase from a minimal snapshot', () => {
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 200)));
    expect(mocks.state.score).toBe(42);
    expect(mocks.state.balloon.x).toBeCloseTo(0.5);
    expect(mocks.state.balloon.y).toBeCloseTo(0.6);
    expect(mocks.applyPhaseChange).toHaveBeenCalledWith('playing');
    expect(mocks.state.hasReceivedFirstSnapshot).toBe(true);
  });

  it('skips duplicate sequence timestamps', () => {
    mocks.isDuplicateSeq.mockReturnValue(true);
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 300)));
    expect(mocks.state.score).toBe(0);
    expect(mocks.applyPhaseChange).not.toHaveBeenCalled();
  });

  it('handles Error parse failures', () => {
    vi.mocked(decodeSnapshot).mockImplementationOnce(() => {
      throw new Error('boom');
    });
    const err = vi.spyOn(console, 'error').mockImplementation(() => {});
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 405)));
    expect(err).toHaveBeenCalledWith('[snapshot] parse error:', 'boom');
    err.mockRestore();
  });

  it('handles parse errors without throwing', () => {
    vi.mocked(decodeSnapshot).mockImplementationOnce(() => {
      throw 'string failure';
    });
    const err = vi.spyOn(console, 'error').mockImplementation(() => {});
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 404)));
    expect(err).toHaveBeenCalledWith('[snapshot] parse error:', 'string failure');
    err.mockRestore();
  });

  it('ignores undecodable snapshots', () => {
    vi.mocked(decodeSnapshot).mockReturnValueOnce(null);
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 400)));
    expect(mocks.state.hasReceivedFirstSnapshot).toBe(false);
    expect(mocks.applyPhaseChange).not.toHaveBeenCalled();
  });

  it('skips blocked phase transitions from snapshot', () => {
    mocks.shouldApplySnapshotPhase.mockReturnValueOnce(false);
    mocks.state.phase = 'countdown';
    handleSnapshot(new DataView(buildMinimalSnapshot(2, 401)));
    expect(mocks.applyPhaseChange).not.toHaveBeenCalled();
  });

  it('keeps pending nickname until player appears in roster', () => {
    mocks.state.pendingNickname = 'Ghost';
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 402)));
    expect(mocks.state.pendingNickname).toBe('Ghost');
  });

  it('applies wind, ripples, pending nickname, and freezes on ended phase', () => {
    mocks.state.phase = 'ended';
    mocks.state.pendingNickname = 'Bob';
    mocks.state.ripples = [{ playerIndex: -2, x: 0.1, y: 0.1, time: 1, isOptimistic: true }];
    const nick = 'Bob';
    const nickBytes = new TextEncoder().encode(nick);
    const buf = new ArrayBuffer(80);
    const dv = new DataView(buf);
    let o = 1;
    dv.setUint32(o, 500, true); o += 4;
    dv.setUint32(o, 99, true); o += 4;
    dv.setUint8(o, 2); o += 1;
    dv.setFloat32(o, 0.5, true); o += 4;
    dv.setFloat32(o, 0.6, true); o += 4;
    dv.setFloat32(o, 0, true); o += 4;
    dv.setFloat32(o, 0, true); o += 4;
    dv.setUint8(o, 0); o += 1;
    dv.setUint8(o, 0); o += 1;
    dv.setUint8(o, 1); o += 1;
    dv.setUint16(o, 4, true); o += 2;
    dv.setUint32(o, 100, true); o += 4;
    dv.setUint32(o, 1, true); o += 4;
    dv.setUint32(o, 20, true); o += 4;
    dv.setUint8(o, nickBytes.length); o += 1;
    new Uint8Array(buf, o).set(nickBytes); o += nickBytes.length;
    dv.setUint8(o, 1); o += 1;
    dv.setUint16(o, 4, true); o += 2;
    dv.setFloat32(o, 0.2, true); o += 4;
    dv.setFloat32(o, 0.3, true); o += 4;
    dv.setFloat32(o, -0.4, true);

    handleSnapshot(new DataView(buf, 0, o + 4));
    expect(mocks.state.wind).toBeCloseTo(-0.4);
    expect(mocks.state.ripples.some((r) => r.playerIndex === 4)).toBe(true);
    expect(mocks.state.pendingNickname).toBeNull();
    expect(mocks.freezeInterpolation).toHaveBeenCalled();
  });
});