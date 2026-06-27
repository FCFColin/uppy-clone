import { describe, it, expect, vi, beforeEach } from 'vitest';

const { mockState, applyPhaseChange } = vi.hoisted(() => ({
  mockState: {
    restartVotes: { yes: 0, total: 1, countdownMs: 0, receivedAt: 0 },
    restartClicked: false,
  },
  applyPhaseChange: vi.fn(),
}));

vi.mock('./state.js', () => ({ state: mockState }));
vi.mock('./phase_sync.js', () => ({ applyPhaseChange }));
vi.mock('./ui.js', () => ({ updateUI: vi.fn() }));

import { handleRestartStatus } from './ws_handlers_phase.js';
import { MSG_TYPE } from './constants.js';

describe('handleRestartStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockState.restartVotes = { yes: 0, total: 1, countdownMs: 0, receivedAt: 0 };
    document.body.innerHTML = '<div id="restart-countdown"></div>';
    delete (window as unknown as { _restartCountdownTimer?: ReturnType<typeof setInterval> })._restartCountdownTimer;
  });

  it('updates restart vote counts from binary payload', () => {
    const buf = new ArrayBuffer(8);
    const view = new DataView(buf);
    view.setUint8(0, MSG_TYPE.RESTART_STATUS);
    view.setUint8(1, 2);
    view.setUint8(2, 4);
    view.setUint32(3, 5000, true);
    handleRestartStatus(view);
    expect(mockState.restartVotes.yes).toBe(2);
    expect(mockState.restartVotes.total).toBe(4);
  });
});
