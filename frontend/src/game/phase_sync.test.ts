import { describe, it, expect, vi, beforeEach } from 'vitest';

const mocks = vi.hoisted(() => ({
  updateUI: vi.fn(),
  startCountdownTimer: vi.fn(),
  hideCountdownOverlay: vi.fn(),
  showCountdownOverlay: vi.fn(),
  resetInterpolation: vi.fn(),
  freezeInterpolation: vi.fn(),
  clearSeenSeqs: vi.fn(),
  state: {
    phase: 'waiting' as 'waiting' | 'countdown' | 'playing' | 'ended',
    ripples: [] as Array<{ playerIndex: number; x: number; y: number; time: number }>,
    explosionEffect: null as null,
    myCooldownEnd: 0,
    lastTapX: null as number | null,
    lastTapY: null as number | null,
    restartClicked: false,
    restartVotes: { yes: 0, total: 0, countdownMs: 0 },
    countdownTimerInterval: null as ReturnType<typeof setInterval> | null,
    nicknameSubmitted: false,
    players: [{ playerIndex: 1, cooldownEndTime: 0, palette: 0, scoreContribution: 0, nickname: 'A' }],
    score: 0,
    balloon: { x: 0.5, y: 0.5, vx: 0, vy: 0 },
    bird: { x: 0, y: 0, active: false },
    ghost: { x: 0, y: 0, active: false, repelTimer: 0 },
    wind: 0,
  },
}));

vi.mock('./state_types.js', () => ({
  state: mocks.state,
}));
vi.mock('./state_interp.js', () => ({
  resetInterpolation: mocks.resetInterpolation,
  freezeInterpolation: mocks.freezeInterpolation,
  clearSeenSeqs: mocks.clearSeenSeqs,
}));

vi.mock('./state_reset.js', () => ({
  resetRoundClientState: () => {
    mocks.state.ripples = [];
    mocks.state.explosionEffect = null;
    mocks.state.myCooldownEnd = 0;
    mocks.state.lastTapX = null;
    mocks.state.lastTapY = null;
    mocks.state.restartClicked = false;
    mocks.state.restartVotes = { yes: 0, total: 0, countdownMs: 0 };
    mocks.state.score = 0;
    mocks.state.balloon = { x: 0.5, y: 0.95, vx: 0, vy: 0 };
    mocks.state.bird = { x: 0, y: 0, active: false };
    mocks.state.ghost = { x: 0, y: 0, active: false, repelTimer: 0 };
    mocks.state.wind = 0;
  },
}));

vi.mock('./ui.js', () => ({
  updateUI: mocks.updateUI,
  startCountdownTimer: mocks.startCountdownTimer,
  hideCountdownOverlay: mocks.hideCountdownOverlay,
  showCountdownOverlay: mocks.showCountdownOverlay,
  startCooldownUpdater: vi.fn(),
  stopCooldownUpdater: vi.fn(),
}));

vi.mock('./entry_flow.js', () => ({
  tryEntryHandoff: vi.fn(),
}));

import { applyPhaseChange, shouldApplySnapshotPhase } from './phase_sync.js';

describe('applyPhaseChange', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.state.phase = 'waiting';
    mocks.state.nicknameSubmitted = false;
    mocks.state.ripples = [{ playerIndex: 1, x: 0.5, y: 0.5, time: 1 }];
    mocks.state.myCooldownEnd = 100;
    mocks.state.restartVotes = { yes: 1, total: 2, countdownMs: 500 };
    mocks.state.score = 42;
    mocks.state.balloon = { x: 0.3, y: 0.7, vx: 0.1, vy: -0.1 };
    mocks.state.ghost = { x: 0.4, y: 0.6, active: true, repelTimer: 5 };
    mocks.state.wind = 0.5;
    window.__gamePhase = 'waiting';
  });

  it('returns false when phase unchanged', () => {
    expect(applyPhaseChange('waiting')).toBe(false);
    expect(mocks.updateUI).not.toHaveBeenCalled();
  });

  it('clears round FX and resets interpolation when entering playing', () => {
    mocks.state.phase = 'countdown';
    expect(applyPhaseChange('playing')).toBe(true);
    expect(mocks.state.phase).toBe('playing');
    expect(mocks.state.ripples).toEqual([]);
    expect(mocks.state.myCooldownEnd).toBe(0);
    expect(mocks.resetInterpolation).toHaveBeenCalled();
    expect(mocks.hideCountdownOverlay).toHaveBeenCalled();
    expect(mocks.updateUI).toHaveBeenCalledWith(true);
    expect(window.__gamePhase).toBe('playing');
  });

  it('clears seenSeqs when entering playing', () => {
    mocks.state.phase = 'countdown';
    applyPhaseChange('playing');
    expect(mocks.clearSeenSeqs).toHaveBeenCalled();
  });

  it('resets score, balloon, ghost, and wind when entering playing', () => {
    mocks.state.phase = 'countdown';
    applyPhaseChange('playing');
    expect(mocks.state.score).toBe(0);
    expect(mocks.state.balloon).toEqual({ x: 0.5, y: 0.95, vx: 0, vy: 0 });
    expect(mocks.state.bird).toEqual({ x: 0, y: 0, active: false });
    expect(mocks.state.ghost).toEqual({ x: 0, y: 0, active: false, repelTimer: 0 });
    expect(mocks.state.wind).toBe(0);
  });

  it('shows countdown overlay when entering countdown', () => {
    mocks.state.nicknameSubmitted = true;
    expect(applyPhaseChange('countdown', 5)).toBe(true);
    expect(mocks.showCountdownOverlay).toHaveBeenCalled();
    expect(mocks.startCountdownTimer).toHaveBeenCalledWith(5);
    expect(mocks.resetInterpolation).toHaveBeenCalled();
  });

  it('blocks countdown before nickname submitted', () => {
    mocks.state.nicknameSubmitted = false;
    expect(applyPhaseChange('countdown')).toBe(false);
    expect(mocks.state.phase).toBe('waiting');
    expect(mocks.showCountdownOverlay).not.toHaveBeenCalled();
  });

  it('blocks ended before nickname submitted', () => {
    mocks.state.nicknameSubmitted = false;
    expect(applyPhaseChange('ended')).toBe(false);
    expect(mocks.state.phase).toBe('waiting');
    expect(mocks.freezeInterpolation).not.toHaveBeenCalled();
  });

  it('allows countdown after restart from ended', () => {
    mocks.state.phase = 'ended';
    mocks.state.nicknameSubmitted = true;
    expect(applyPhaseChange('countdown')).toBe(true);
    expect(mocks.showCountdownOverlay).toHaveBeenCalled();
  });

  it('freezes interpolation and sets restart votes on ended', () => {
    mocks.state.phase = 'playing';
    expect(applyPhaseChange('ended')).toBe(true);
    expect(mocks.freezeInterpolation).toHaveBeenCalled();
    expect(mocks.state.restartVotes.total).toBe(mocks.state.players.length);
  });

  it('allows playing -> countdown transition (server restart scenario)', () => {
    mocks.state.phase = 'playing';
    mocks.state.nicknameSubmitted = true;
    expect(applyPhaseChange('countdown')).toBe(true);
    expect(mocks.state.phase).toBe('countdown');
    expect(mocks.showCountdownOverlay).toHaveBeenCalled();
  });

  it('allows playing -> waiting transition (server reset scenario)', () => {
    mocks.state.phase = 'playing';
    expect(applyPhaseChange('waiting')).toBe(true);
    expect(mocks.state.phase).toBe('waiting');
  });
});

describe('shouldApplySnapshotPhase', () => {
  beforeEach(() => {
    mocks.state.phase = 'waiting';
  });

  it('allows same phase', () => {
    expect(shouldApplySnapshotPhase('waiting')).toBe(true);
  });

  it('allows playing -> countdown (new round after restart)', () => {
    mocks.state.phase = 'playing';
    expect(shouldApplySnapshotPhase('countdown')).toBe(true);
  });

  it('allows playing -> waiting (server reset)', () => {
    mocks.state.phase = 'playing';
    expect(shouldApplySnapshotPhase('waiting')).toBe(true);
  });

  it('allows playing -> ended (normal game over)', () => {
    mocks.state.phase = 'playing';
    expect(shouldApplySnapshotPhase('ended')).toBe(true);
  });

  it('allows ended -> countdown (restart)', () => {
    mocks.state.phase = 'ended';
    expect(shouldApplySnapshotPhase('countdown')).toBe(true);
  });

  it('allows ended -> waiting (server reset)', () => {
    mocks.state.phase = 'ended';
    expect(shouldApplySnapshotPhase('waiting')).toBe(true);
  });

  it('blocks countdown -> ended regression', () => {
    mocks.state.phase = 'countdown';
    expect(shouldApplySnapshotPhase('ended')).toBe(false);
  });

  it('blocks countdown -> waiting regression', () => {
    mocks.state.phase = 'countdown';
    expect(shouldApplySnapshotPhase('waiting')).toBe(false);
  });

  it('allows unknown client phase via default branch', () => {
    mocks.state.phase = 'unknown' as typeof mocks.state.phase;
    expect(shouldApplySnapshotPhase('playing')).toBe(true);
  });

  it('clears countdown and restart timers when entering playing', () => {
    mocks.state.phase = 'countdown';
    mocks.state.countdownTimerInterval = setInterval(() => {}, 1000);
    window._restartCountdownTimer = setInterval(() => {}, 1000);
    applyPhaseChange('playing');
    expect(mocks.state.countdownTimerInterval).toBeNull();
    expect(window._restartCountdownTimer).toBeNull();
  });

  it('handles missing nickname inline element when entering playing', () => {
    document.body.innerHTML = '<div id="nickname-setup-screen"></div><div id="nickname-inline"></div>';
    mocks.state.phase = 'countdown';
    mocks.state.nicknameSubmitted = true;
    applyPhaseChange('playing');
    expect(document.getElementById('nickname-inline')!.classList.contains('hidden')).toBe(true);
  });

  it('allows waiting snapshot transitions from waiting phase', () => {
    mocks.state.phase = 'waiting';
    expect(shouldApplySnapshotPhase('countdown')).toBe(true);
    expect(shouldApplySnapshotPhase('playing')).toBe(true);
    expect(shouldApplySnapshotPhase('ended')).toBe(true);
  });
});
