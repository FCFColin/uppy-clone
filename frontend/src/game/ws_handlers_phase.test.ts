import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const mocks = vi.hoisted(() => ({
  state: {
    phase: 'waiting' as 'waiting' | 'countdown' | 'playing' | 'ended',
    restartVotes: { yes: 0, total: 0, countdownMs: 0, receivedAt: 0 },
    nicknameSubmitted: true,
    players: [],
    ripples: [],
    explosionEffect: null,
    myCooldownEnd: 0,
    lastTapX: null,
    lastTapY: null,
    restartClicked: false,
    countdownTimerInterval: null,
    endReason: 0,
    score: 0,
  },
  applyPhaseChange: vi.fn(() => true),
  shouldApplySnapshotPhase: vi.fn(() => true),
  updateUI: vi.fn(),
}));

vi.mock('./state.js', () => ({ state: mocks.state }));
vi.mock('./phase_sync.js', () => ({
  applyPhaseChange: mocks.applyPhaseChange,
  shouldApplySnapshotPhase: mocks.shouldApplySnapshotPhase,
}));
vi.mock('./ui.js', () => ({ updateUI: mocks.updateUI }));
vi.mock('./tutorial.js', () => ({
  runTutorialIfNeeded: vi.fn(() => Promise.resolve()),
}));
vi.mock('../shared/audio.js', () => ({
  playGameOverSound: vi.fn(),
  vibrate: vi.fn(),
}));
vi.mock('../shared/best_score_cookie.js', () => ({
  updateBestScore: vi.fn((score: number) => ({ best: score - 10, isNewRecord: false })),
  fetchUserBestScore: vi.fn(() => Promise.resolve(999)),
}));

import { handleGameStateChange, handleRestartStatus } from './ws_handlers_phase.js';

describe('handleGameStateChange', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.shouldApplySnapshotPhase.mockReturnValue(true);
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
    const bestScoreModule = await import('../shared/best_score_cookie.js');
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
    const bestScoreModule = await import('../shared/best_score_cookie.js');
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
    vi.clearAllMocks();
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
    expect(mocks.updateUI).toHaveBeenCalledWith(true);
  });
});
