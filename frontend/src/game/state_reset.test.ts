import { describe, it, expect, beforeEach } from 'vitest';
import { state } from './state.js';
import { outboundMessageQueue, seenSeqs } from './state.js';
import { resetClientState, resetRoundClientState } from './state_reset.js';

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
    seenSeqs.add(99);
    outboundMessageQueue.push(new ArrayBuffer(1));
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

  it('resetClientState clears snapshot readiness and outbound queue', () => {
    resetClientState();
    expect(state.hasReceivedFirstSnapshot).toBe(false);
    expect(seenSeqs.size).toBe(0);
    expect(outboundMessageQueue.length).toBe(0);
    expect(state.ripples).toEqual([]);
  });
});
