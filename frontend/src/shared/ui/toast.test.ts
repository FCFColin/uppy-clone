import { describe, it, expect, vi, beforeEach } from 'vitest';
import { showToast } from './toast.js';

describe('toast', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  it('shows toast with message', () => {
    showToast('hello');
    const el = document.getElementById('app-toast');
    expect(el).not.toBeNull();
    expect(el?.textContent).toBe('hello');
    expect(el?.classList.contains('visible')).toBe(true);
  });

  it('reuses existing toast element on multiple calls', () => {
    showToast('first');
    showToast('second');
    const el = document.getElementById('app-toast');
    expect(el).not.toBeNull();
    expect(el?.textContent).toBe('second');
  });

  it('auto-closes after duration', () => {
    vi.useFakeTimers();
    showToast('auto-close', 1000);
    const el = document.getElementById('app-toast')!;
    expect(el.classList.contains('visible')).toBe(true);
    vi.advanceTimersByTime(1000);
    expect(el.classList.contains('visible')).toBe(false);
    vi.useRealTimers();
  });

  it('handles very long message', () => {
    const long = 'x'.repeat(1000);
    showToast(long);
    const el = document.getElementById('app-toast');
    expect(el?.textContent).toBe(long);
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
