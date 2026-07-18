import { vi } from 'vitest';

/**
 * Shared ui_common.js mock factory for ws_connection* tests.
 * Each call returns a fresh set of vi.fn() mocks so test files keep
 * independent mock instances (Vitest isolates module state per file).
 */
export function createUiCommonMock() {
  return {
    hideReconnectBanner: vi.fn(),
    showReconnectBanner: vi.fn(),
    updatePingDisplay: vi.fn(),
    showConnectionError: vi.fn(),
  };
}
