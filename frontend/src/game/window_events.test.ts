import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockCanvas = vi.hoisted(() => {
  const el = document.createElement('canvas');
  el.id = 'game-canvas';
  return el;
});
const mockGetState = vi.hoisted(() => vi.fn(() => ({ phase: 'playing' })));
const mockDispatch = vi.hoisted(() => vi.fn());
const mockResizeCanvas = vi.hoisted(() => vi.fn());
const mockGameLoop = vi.hoisted(() => vi.fn());
const mockSetRenderActive = vi.hoisted(() => vi.fn());
const mockRenderOnce = vi.hoisted(() => vi.fn());
const mockResumeAudioContext = vi.hoisted(() => vi.fn());
const mockHandleTap = vi.hoisted(() => vi.fn());
const mockRequestRestart = vi.hoisted(() => vi.fn());
const mockTapAtBalloonCenter = vi.hoisted(() => vi.fn());
const mockConnectWebSocket = vi.hoisted(() => vi.fn());
const mockStopHeartbeat = vi.hoisted(() => vi.fn());
const mockGetWs = vi.hoisted(() => vi.fn());

vi.mock('../shared/ui/ui.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../shared/ui/ui.js')>();
  return {
    ...actual,
    resumeAudioContext: mockResumeAudioContext,
  };
});
vi.mock('./input.js', () => ({
  handleTap: mockHandleTap,
  requestRestart: mockRequestRestart,
  tapAtBalloonCenter: mockTapAtBalloonCenter,
}));
vi.mock('./state.js', () => ({ getState: mockGetState, dispatch: mockDispatch }));
vi.mock('./renderer.js', () => ({
  resizeCanvas: mockResizeCanvas,
  gameLoop: mockGameLoop,
  setRenderActive: mockSetRenderActive,
  renderOnce: mockRenderOnce,
  $canvas: mockCanvas,
}));
vi.mock('./ws_connection.js', () => ({
  stopHeartbeat: mockStopHeartbeat,
  getWs: mockGetWs,
  connectWebSocket: mockConnectWebSocket,
}));

import { bindWindowEvents } from './window_events.js';

describe('windowEvents', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    document.body.innerHTML = '';
  });

  it.each([
    ['click', mockCanvas, mockResumeAudioContext],
    ['touchstart', mockCanvas, mockResumeAudioContext],
  ] as const)('canvas %s listener resumes audio context', (_evt, target, mock) => {
    bindWindowEvents();
    const eventCtor = _evt === 'click' ? MouseEvent : TouchEvent;
    target.dispatchEvent(new eventCtor(_evt));
    expect(mock).toHaveBeenCalled();
  });
});
