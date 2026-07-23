import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MSG_TYPE, PHASE_CODE, NICKNAME_REJECT_REASON } from '../shared/game/constants.js';
import { buildMinimalSnapshot } from './test_fixtures/snapshot.js';
import { getMocks, resetWsHandlersMocks, createStateJsMockModule } from './ws_handlers_test_setup.js';

const mocks = getMocks();

vi.mock('./state.js', async (importActual) => createStateJsMockModule(importActual as never));

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
vi.mock('../shared/ui/ui.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../shared/ui/ui.js')>();
  return {
    ...actual,
    playGameOverSound: vi.fn(),
    vibrate: vi.fn(),
  };
});
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
vi.mock('./entry_flow.js', async () => {
  const m = getMocks();
  return {
    applyEntryStep: m.applyEntryStep,
    revertEntryStepToNickname: m.revertEntryStepToNickname,
    clearStartCountdown: m.clearStartCountdown,
    setNicknameStatus: m.setNicknameStatus,
  };
});

import {
  handleBinaryMessage,
  handleTapAccepted,
  handleTapRejected,
  handleGameStateChange,
  handleRestartStatus,
  handleSnapshot,
  handleNicknameRejected,
} from './ws_handlers.js';
import { decodeSnapshot } from './message_codec.js';
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

  it('warns on unknown message types', () => {
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

  it('handleTapRejected clears optimistic cooldown, adds rejected ripple and floating text', () => {
    mocks.state.myCooldownEnd = Date.now() + 5000;
    mocks.state.ripples = [{ isOptimistic: true }];
    handleTapRejected();
    expect(mocks.state.myCooldownEnd).toBe(0);
    expect(mocks.state.ripples.some((r) => r.isOptimistic)).toBe(false);
    expect(mocks.state.ripples.some((r) => r.rejected)).toBe(true);
    expect(pushFloatingText).toHaveBeenCalledWith(0.4, 0.6, '太远了');
  });
});

describe('handleGameStateChange', () => {
  beforeEach(() => {
    resetWsHandlersMocks();
  });

  it('applies playing phase from binary message, skips blocked phase regression', () => {
    const buf = new ArrayBuffer(2);
    const dv = new DataView(buf);
    dv.setUint8(0, 0);
    dv.setUint8(1, 1);
    handleGameStateChange(dv);
    expect(mocks.applyPhaseChange).toHaveBeenCalledWith('playing', 3);

    mocks.applyPhaseChange.mockClear();
    mocks.shouldApplySnapshotPhase.mockReturnValue(false);
    const buf2 = new ArrayBuffer(2);
    new DataView(buf2).setUint8(1, 1);
    handleGameStateChange(new DataView(buf2));
    expect(mocks.applyPhaseChange).not.toHaveBeenCalled();
  });

  it('derives countdown seconds from remaining ms and applies phase synchronously', () => {
    const buf = new ArrayBuffer(6);
    const dv = new DataView(buf);
    dv.setUint8(1, 3);
    dv.setUint32(2, 5500, true);
    handleGameStateChange(dv);
    expect(mocks.applyPhaseChange).toHaveBeenCalledWith('countdown', 6);
  });

  it('handles ended phase with end reason and score banner, skips update when personal-best element is missing', async () => {
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
    document.body.innerHTML = '';
    const dv2 = new DataView(new ArrayBuffer(3));
    dv2.setUint8(1, 2);
    dv2.setUint8(2, 1);
    handleGameStateChange(dv2);
    await Promise.resolve();
    expect(document.getElementById('personal-best')).toBeNull();
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

  it('ignores snapshots that are too short or undecodable', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    handleSnapshot(new DataView(new ArrayBuffer(10)));
    expect(warn).toHaveBeenCalled();
    expect(mocks.state.hasReceivedFirstSnapshot).toBe(false);
    warn.mockRestore();

    vi.mocked(decodeSnapshot).mockReturnValueOnce(null);
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 400)));
    expect(mocks.state.hasReceivedFirstSnapshot).toBe(false);
    expect(mocks.applyPhaseChange).not.toHaveBeenCalled();
  });

  it('parses score, balloon, and phase from a minimal snapshot', () => {
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 200)));
    expect(mocks.state.score).toBe(42);
    expect(mocks.state.balloon.x).toBeCloseTo(0.5);
    expect(mocks.state.balloon.y).toBeCloseTo(0.6);
    expect(mocks.applyPhaseChange).toHaveBeenCalledWith('playing');
    expect(mocks.state.hasReceivedFirstSnapshot).toBe(true);
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
    dv.setUint32(o, 500, true);
    o += 4;
    dv.setUint32(o, 99, true);
    o += 4;
    dv.setUint8(o, 2);
    o += 1;
    dv.setFloat32(o, 0.5, true);
    o += 4;
    dv.setFloat32(o, 0.6, true);
    o += 4;
    dv.setFloat32(o, 0, true);
    o += 4;
    dv.setFloat32(o, 0, true);
    o += 4;
    dv.setUint8(o, 0);
    o += 1;
    dv.setUint8(o, 0);
    o += 1;
    dv.setUint8(o, 1);
    o += 1;
    dv.setUint16(o, 4, true);
    o += 2;
    dv.setUint32(o, 100, true);
    o += 4;
    dv.setUint32(o, 1, true);
    o += 4;
    dv.setUint32(o, 20, true);
    o += 4;
    dv.setUint8(o, nickBytes.length);
    o += 1;
    new Uint8Array(buf, o).set(nickBytes);
    o += nickBytes.length;
    dv.setUint8(o, 1);
    o += 1;
    dv.setUint16(o, 4, true);
    o += 2;
    dv.setFloat32(o, 0.2, true);
    o += 4;
    dv.setFloat32(o, 0.3, true);
    o += 4;
    dv.setFloat32(o, -0.4, true);

    handleSnapshot(new DataView(buf, 0, o + 4));
    expect(mocks.state.wind).toBeCloseTo(-0.4);
    expect(mocks.state.ripples.some((r) => r.playerIndex === 4)).toBe(true);
    expect(mocks.state.pendingNickname).toBeNull();
    expect(mocks.freezeInterpolation).toHaveBeenCalled();
  });
});

describe('handleNicknameRejected', () => {
  beforeEach(() => {
    resetWsHandlersMocks();
    mocks.state.nicknameSubmitted = true;
    mocks.state.pendingNickname = 'Alice';
  });

  function buildNicknameRejectedBuf(reason: number): DataView {
    const buf = new ArrayBuffer(2);
    const dv = new DataView(buf);
    dv.setUint8(0, MSG_TYPE.NICKNAME_REJECTED);
    dv.setUint8(1, reason);
    return dv;
  }

  it('resets nickname state, regresses to nickname step, clears countdown, and shows empty-nickname message on EMPTY', () => {
    handleNicknameRejected(buildNicknameRejectedBuf(NICKNAME_REJECT_REASON.EMPTY));
    expect(mocks.state.nicknameSubmitted).toBe(false);
    expect(mocks.state.pendingNickname).toBeNull();
    expect(mocks.revertEntryStepToNickname).toHaveBeenCalled();
    expect(mocks.clearStartCountdown).toHaveBeenCalled();
    expect(mocks.setNicknameStatus).toHaveBeenCalledWith('昵称不能为空');
    expect(mocks.updateUI).toHaveBeenCalledWith({ force: true });
  });

  it.each([
    [NICKNAME_REJECT_REASON.DUPLICATE, '昵称已被占用'],
    [NICKNAME_REJECT_REASON.COOLDOWN, '昵称冷却中，请稍后'],
    [NICKNAME_REJECT_REASON.DECODE_ERROR, '昵称格式无效'],
    [0xff, '昵称被拒绝，请重试'],
  ])('shows reason-specific message for reason %i', (reason, expectedMessage) => {
    handleNicknameRejected(buildNicknameRejectedBuf(reason));
    expect(mocks.setNicknameStatus).toHaveBeenCalledWith(expectedMessage);
    expect(mocks.revertEntryStepToNickname).toHaveBeenCalled();
    expect(mocks.clearStartCountdown).toHaveBeenCalled();
    expect(mocks.updateUI).toHaveBeenCalledWith({ force: true });
  });

  it('ignores messages shorter than 2 bytes and warns', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const buf = new ArrayBuffer(1);
    new DataView(buf).setUint8(0, MSG_TYPE.NICKNAME_REJECTED);
    handleNicknameRejected(new DataView(buf));
    expect(warn).toHaveBeenCalledWith('[ws] NICKNAME_REJECTED too short, ignoring');
    expect(mocks.revertEntryStepToNickname).not.toHaveBeenCalled();
    expect(mocks.clearStartCountdown).not.toHaveBeenCalled();
    expect(mocks.setNicknameStatus).not.toHaveBeenCalled();
    expect(mocks.updateUI).not.toHaveBeenCalled();
    expect(mocks.state.nicknameSubmitted).toBe(true);
    expect(mocks.state.pendingNickname).toBe('Alice');
    warn.mockRestore();
  });
});
