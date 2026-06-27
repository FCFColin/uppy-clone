import { describe, it, expect, vi, beforeEach } from 'vitest';

const { mockState } = vi.hoisted(() => ({
  mockState: {
    players: [] as Array<{ playerIndex: number; nickname: string; palette: number }>,
    ripples: [] as Array<Record<string, unknown>>,
    myCooldownEnd: 0,
    explosionEffect: null as unknown,
    phase: 'waiting' as string,
  },
}));

vi.mock('./state.js', () => ({ state: mockState }));
vi.mock('./ui.js', () => ({ updateUI: vi.fn() }));

import { handlePlayerJoin, handleTapAccepted } from './ws_handlers_events.js';
import { MSG_TYPE } from './constants.js';

describe('ws_handlers_events', () => {
  beforeEach(() => {
    mockState.ripples = [];
    mockState.myCooldownEnd = 0;
    vi.spyOn(console, 'log').mockImplementation(() => {});
  });

  it('handlePlayerJoin decodes nickname from payload', () => {
    const buf = new ArrayBuffer(12);
    const view = new DataView(buf);
    view.setUint8(0, MSG_TYPE.PLAYER_JOIN);
    view.setUint16(1, 1, true);
    view.setUint8(3, 2);
    new Uint8Array(buf, 4, 2).set([97, 98]);
    view.setUint32(6, 2, true);
    expect(() => handlePlayerJoin(view)).not.toThrow();
  });

  it('handleTapAccepted updates cooldown and ripples', () => {
    mockState.ripples = [{ isOptimistic: true }];
    const buf = new ArrayBuffer(18);
    const view = new DataView(buf);
    view.setUint8(0, MSG_TYPE.TAP_ACCEPTED);
    view.setUint16(1, 1, true);
    view.setUint32(3, 500, true);
    view.setFloat32(7, 0.5, true);
    view.setFloat32(11, 0.5, true);
    handleTapAccepted(view);
    expect(mockState.myCooldownEnd).toBeGreaterThan(Date.now());
    expect(mockState.ripples.some((r) => r.playerIndex === 1)).toBe(true);
  });
});
