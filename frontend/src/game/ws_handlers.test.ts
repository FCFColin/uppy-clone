import { describe, it, expect, vi, beforeEach } from 'vitest';
import { MSG_TYPE } from '../shared/game/protocol.js';
import { PHASE_CODE } from '../shared/game/protocol.js';

const mocks = vi.hoisted(() => ({
  state: {
    phase: 'waiting' as 'waiting' | 'countdown' | 'playing' | 'ended',
    score: 0,
    endReason: 0,
    balloon: { x: 0.5, y: 0.5, vx: 0, vy: 0 },
    bird: { active: false, x: 0, y: 0 },
    ghost: { active: false, x: 0.5, y: 0.5, repelTimer: 0 },
    players: [] as Array<{ playerIndex: number; cooldownEndTime: number; palette: number; scoreContribution: number; nickname: string }>,
    ripples: [] as Array<{ playerIndex: number; x: number; y: number; time: number; isOptimistic?: boolean; rejected?: boolean }>,
    wind: 0,
    hasReceivedFirstSnapshot: false,
    nicknameSubmitted: true,
    pendingNickname: null as string | null,
    restartVotes: { yes: 0, total: 0, countdownMs: 0, receivedAt: 0 },
    lastTapX: 0.5 as number | null,
    lastTapY: 0.5 as number | null,
    myCooldownEnd: 0,
    explosionEffect: null as unknown,
  },
  applyPhaseChange: vi.fn(() => true),
  shouldApplySnapshotPhase: vi.fn(() => true),
  updateUI: vi.fn(),
  updateScoresOnly: vi.fn(),
  updateWindIndicator: vi.fn(),
  syncRestartVoteUI: vi.fn(),
  updateInterpolation: vi.fn(),
  freezeInterpolation: vi.fn(),
  isDuplicateSeq: vi.fn(() => false),
}));

vi.mock('./state.js', () => ({
  state: mocks.state,
  updateInterpolation: mocks.updateInterpolation,
  freezeInterpolation: mocks.freezeInterpolation,
  isDuplicateSeq: mocks.isDuplicateSeq,
}));
vi.mock('./phase_sync.js', () => ({
  applyPhaseChange: mocks.applyPhaseChange,
  shouldApplySnapshotPhase: mocks.shouldApplySnapshotPhase,
}));
vi.mock('./ui.js', () => ({ updateUI: mocks.updateUI }));
vi.mock('./ui_update.js', () => ({ updateScoresOnly: mocks.updateScoresOnly }));
vi.mock('./ui_wind.js', () => ({ updateWindIndicator: mocks.updateWindIndicator }));
vi.mock('./tutorial.js', () => ({ runTutorialIfNeeded: vi.fn(() => Promise.resolve()) }));
vi.mock('../shared/ui/audio.js', () => ({
  playGameOverSound: vi.fn(),
  vibrate: vi.fn(),
}));
vi.mock('../shared/data/best_score_cookie.js', () => ({
  updateBestScore: vi.fn(() => ({ best: 0, isNewRecord: false })),
  fetchUserBestScore: vi.fn(() => Promise.resolve(0)),
}));
vi.mock('./visual_helpers.js', () => ({ pushFloatingText: vi.fn() }));
vi.mock('./restart_vote_ui.js', () => ({ syncRestartVoteUI: mocks.syncRestartVoteUI }));

import { handleBinaryMessage } from './ws_handlers.js';
import { playGameOverSound } from '../shared/ui/audio.js';
import { pushFloatingText } from './visual_helpers.js';
import { buildMinimalSnapshot } from './test_fixtures/snapshot.js';

describe('handleBinaryMessage routing', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.state.phase = 'waiting';
    mocks.state.ripples = [];
    mocks.state.lastTapX = 0.5;
    mocks.state.lastTapY = 0.5;
    mocks.isDuplicateSeq.mockReturnValue(false);
    mocks.shouldApplySnapshotPhase.mockReturnValue(true);
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
