import { describe, it, expect, vi, beforeEach } from 'vitest';

const { mockState } = vi.hoisted(() => ({
  mockState: {
    players: [] as Array<{ playerIndex: number; nickname: string; palette: number }>,
    ripples: [] as Array<Record<string, unknown>>,
    myCooldownEnd: 0,
    explosionEffect: null as unknown,
    phase: 'waiting' as string,
    lastTapX: null as number | null,
    lastTapY: null as number | null,
  },
}));

vi.mock('./state.js', () => ({ state: mockState }));
vi.mock('./ui.js', () => ({ updateUI: vi.fn() }));
vi.mock('./visual_helpers.js', () => ({
  pushFloatingText: vi.fn(),
}));

import { handleTapAccepted, handleTapRejected } from './ws_handlers_events.js';
import { pushFloatingText } from './visual_helpers.js';
import { MSG_TYPE } from './constants.js';

describe('ws_handlers_events', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockState.ripples = [];
    mockState.myCooldownEnd = 0;
    mockState.lastTapX = 0.4;
    mockState.lastTapY = 0.6;
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

  it('handleTapAccepted falls back to server balloon coordinates', () => {
    mockState.lastTapX = null;
    mockState.lastTapY = null;
    const buf = new ArrayBuffer(18);
    const view = new DataView(buf);
    view.setUint8(0, MSG_TYPE.TAP_ACCEPTED);
    view.setUint16(1, 2, true);
    view.setUint32(3, 250, true);
    view.setFloat32(7, 0.7, true);
    view.setFloat32(11, 0.8, true);
    handleTapAccepted(view);
    expect(mockState.ripples.at(-1)).toMatchObject({ playerIndex: 2 });
    expect(mockState.ripples.at(-1)!.x).toBeCloseTo(0.7, 5);
    expect(mockState.ripples.at(-1)!.y).toBeCloseTo(0.8, 5);
  });

  it('handleTapRejected clears optimistic cooldown', () => {
    mockState.myCooldownEnd = Date.now() + 5000;
    mockState.ripples = [{ isOptimistic: true }];
    handleTapRejected();
    expect(mockState.myCooldownEnd).toBe(0);
    expect(mockState.ripples.some((r) => r.isOptimistic)).toBe(false);
  });

  it('handleTapRejected adds rejected ripple and floating text', () => {
    handleTapRejected();
    expect(mockState.ripples.some((r) => r.rejected)).toBe(true);
    expect(pushFloatingText).toHaveBeenCalledWith(0.4, 0.6, '太远了');
  });

  it('handleTapRejected is a no-op without last tap coordinates', () => {
    mockState.lastTapX = null;
    handleTapRejected();
    expect(mockState.ripples).toHaveLength(0);
    expect(pushFloatingText).not.toHaveBeenCalled();
  });
});
