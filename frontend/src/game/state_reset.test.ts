import { describe, it, expect, beforeEach, vi } from 'vitest';
import { state } from './state.js';
import { isDuplicateSeq, getSeenSeqsSize } from './seen_seqs.js';

vi.mock('./ws_connection.js', () => ({
  clearOutboundQueue: vi.fn(),
  getOutboundQueueLength: vi.fn(() => 0),
  sendOrQueue: vi.fn(),
  getWs: vi.fn(() => null),
  setWs: vi.fn(),
  getWsEverOpened: vi.fn(() => false),
  setWsEverOpened: vi.fn(),
  resetReconnectAttempts: vi.fn(),
  clearReconnectTimer: vi.fn(),
  setRoomPreChecked: vi.fn(),
  wasRoomPreChecked: vi.fn(() => false),
  setReconnectTimer: vi.fn(),
  scheduleReconnect: vi.fn(),
  waitForWebSocket: vi.fn(() => Promise.resolve()),
  showConnectionError: vi.fn(),
  flushPendingQueue: vi.fn(),
  hideReconnectBanner: vi.fn(),
  startHeartbeat: vi.fn(),
  stopHeartbeat: vi.fn(),
  handlePong: vi.fn(),
}));

import { resetClientState, resetRoundClientState } from './state_interp.js';

describe('client_state_reset', () => {
  beforeEach(() => {
    state.ripples = [{ playerIndex: 1, x: 0, y: 0, time: 1 }];
    state.explosionEffect = { x: 0, y: 0, startTime: 1 };
    state.myCooldownEnd = Date.now() + 1000;
    state.lastTapX = 0.1;
    state.lastTapY = 0.2;
    state.restartClicked = true;
    state.restartVotes = { yes: 1, total: 2, countdownMs: 100, receivedAt: 1 };
    state.score = 50;
    state.hasReceivedFirstSnapshot = true;
    isDuplicateSeq(99);
  });

  it('resetRoundClientState clears round gameplay fields', () => {
    resetRoundClientState();
    expect(state.ripples).toEqual([]);
    expect(state.explosionEffect).toBeNull();
    expect(state.myCooldownEnd).toBe(0);
    expect(state.lastTapX).toBeNull();
    expect(state.score).toBe(0);
    expect(state.restartClicked).toBe(false);
  });

  it('resetClientState clears snapshot readiness and seenSeqs', () => {
    resetClientState();
    expect(state.hasReceivedFirstSnapshot).toBe(false);
    expect(getSeenSeqsSize()).toBe(0);
    expect(state.ripples).toEqual([]);
  });
});
