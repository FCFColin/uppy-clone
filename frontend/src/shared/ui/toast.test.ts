import { describe, it, expect, vi, beforeEach } from 'vitest';
import { showToast } from './utils.js';

describe('toast', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  it('shows toast with message, reuses element on multiple calls', () => {
    showToast('hello');
    const el = document.getElementById('app-toast');
    expect(el).not.toBeNull();
    expect(el?.textContent).toBe('hello');
    expect(el?.classList.contains('visible')).toBe(true);
    showToast('second');
    expect(el?.textContent).toBe('second');
  });

  it('auto-closes after duration, handles very long message', () => {
    vi.useFakeTimers();
    // Auto-close
    showToast('auto-close', 1000);
    const el = document.getElementById('app-toast')!;
    expect(el.classList.contains('visible')).toBe(true);
    vi.advanceTimersByTime(1000);
    expect(el.classList.contains('visible')).toBe(false);
    // Very long message
    const long = 'x'.repeat(1000);
    showToast(long);
    expect(document.getElementById('app-toast')?.textContent).toBe(long);
    vi.useRealTimers();
  });

  it('resets timer when called again before previous expires', () => {
    vi.useFakeTimers();
    showToast('first', 2000);
    showToast('second', 100);
    vi.advanceTimersByTime(150);
    const el = document.getElementById('app-toast')!;
    expect(el.classList.contains('visible')).toBe(false);
    vi.useRealTimers();
  });
});
