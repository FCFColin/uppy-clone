import { vi } from 'vitest';

// Shared mocks for ws_handlers test files. Each test file imports this helper
// and calls getMocks() to access the mocks. The mocks are scoped per test file
// because Vitest isolates module state between test files.

export interface WsHandlersMocks {
  state: {
    phase: 'waiting' | 'countdown' | 'playing' | 'ended';
    score: number;
    endReason: number;
    balloon: { x: number; y: number; vx: number; vy: number };
    bird: { active: boolean; x: number; y: number };
    ghost: { active: boolean; x: number; y: number; repelTimer: number };
    players: Array<Record<string, unknown>>;
    ripples: Array<Record<string, unknown>>;
    wind: number;
    hasReceivedFirstSnapshot: boolean;
    nicknameSubmitted: boolean;
    pendingNickname: string | null;
    restartVotes: { yes: number; total: number; countdownMs: number; receivedAt: number };
    lastTapX: number | null;
    lastTapY: number | null;
    myCooldownEnd: number;
    explosionEffect: unknown;
    restartClicked: boolean;
    countdownTimerInterval: ReturnType<typeof setInterval> | null;
  };
  applyPhaseChange: ReturnType<typeof vi.fn>;
  shouldApplySnapshotPhase: ReturnType<typeof vi.fn>;
  updateUI: ReturnType<typeof vi.fn>;
  updateScoresOnly: ReturnType<typeof vi.fn>;
  updateWindIndicator: ReturnType<typeof vi.fn>;
  syncRestartVoteUI: ReturnType<typeof vi.fn>;
  updateInterpolation: ReturnType<typeof vi.fn>;
  freezeInterpolation: ReturnType<typeof vi.fn>;
  isDuplicateSeq: ReturnType<typeof vi.fn>;
}

function createMocks(): WsHandlersMocks {
  return {
    state: {
      phase: 'waiting',
      score: 0,
      endReason: 0,
      balloon: { x: 0.5, y: 0.5, vx: 0, vy: 0 },
      bird: { active: false, x: 0, y: 0 },
      ghost: { active: false, x: 0.5, y: 0.5, repelTimer: 0 },
      players: [],
      ripples: [],
      wind: 0,
      hasReceivedFirstSnapshot: false,
      nicknameSubmitted: true,
      pendingNickname: null,
      restartVotes: { yes: 0, total: 0, countdownMs: 0, receivedAt: 0 },
      lastTapX: 0.5,
      lastTapY: 0.5,
      myCooldownEnd: 0,
      explosionEffect: null,
      restartClicked: false,
      countdownTimerInterval: null,
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
  };
}

let mocks: WsHandlersMocks | null = null;

export function getMocks(): WsHandlersMocks {
  if (!mocks) {
    mocks = createMocks();
  }
  return mocks;
}

export function resetWsHandlersMocks() {
  const m = getMocks();
  vi.clearAllMocks();
  m.state.phase = 'waiting';
  m.state.score = 0;
  m.state.endReason = 0;
  m.state.ripples = [];
  m.state.players = [];
  m.state.wind = 0;
  m.state.hasReceivedFirstSnapshot = false;
  m.state.pendingNickname = null;
  m.state.lastTapX = 0.5;
  m.state.lastTapY = 0.5;
  m.state.myCooldownEnd = 0;
  m.state.explosionEffect = null;
  m.state.restartClicked = false;
  m.state.countdownTimerInterval = null;
  m.state.restartVotes = { yes: 0, total: 0, countdownMs: 0, receivedAt: 0 };
  m.applyPhaseChange.mockReturnValue(true);
  m.shouldApplySnapshotPhase.mockReturnValue(true);
  m.isDuplicateSeq.mockReturnValue(false);
}

// Factory for the state.js mock module. Used as:
//   vi.mock('./state.js', (importActual) => createStateJsMockModule(importActual as any));
export async function createStateJsMockModule(importActual: () => Promise<any>) {
  const actual = await importActual();
  const m = getMocks();
  return {
    ...actual,
    state: m.state,
    getState: () => m.state,
    dispatch: (action: any) => {
      const next = actual.gameReducer(m.state, action);
      if (next !== m.state) Object.assign(m.state, next);
    },
  };
}