import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockState = vi.hoisted(() => ({
  phase: 'ended',
  restartVotes: { yes: 0, total: 2, countdownMs: 0, receivedAt: 0 },
}));
vi.mock('./state.js', () => ({
  getState: () => mockState,
}));

import {
  syncRestartVoteProgress,
  syncRestartVoteUI,
  clearRestartCountdownTimer,
} from './restart_vote_ui.js';

describe('restart_vote_ui', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="restart-progress"></div><div id="restart-countdown"></div>';
    mockState.phase = 'ended';
    mockState.restartVotes = { yes: 0, total: 2, countdownMs: 0, receivedAt: 0 };
    clearRestartCountdownTimer();
  });

  describe('syncRestartVoteProgress', () => {
    it('no-ops when phase is not ended', () => {
      mockState.phase = 'playing';
      syncRestartVoteProgress();
      expect(document.getElementById('restart-progress')!.textContent).toBe('');
    });

    it('no-ops when restartVotes is null', () => {
      mockState.phase = 'ended';
      mockState.restartVotes = null as unknown as typeof mockState.restartVotes;
      syncRestartVoteProgress();
      expect(document.getElementById('restart-progress')!.textContent).toBe('');
    });

    it('clears text when total is 0', () => {
      mockState.restartVotes = { yes: 0, total: 0, countdownMs: 0, receivedAt: 0 };
      syncRestartVoteProgress();
      expect(document.getElementById('restart-progress')!.textContent).toBe('');
    });

    it('shows restarting text when yes >= total', () => {
      mockState.restartVotes = { yes: 2, total: 2, countdownMs: 0, receivedAt: 0 };
      syncRestartVoteProgress();
      expect(document.getElementById('restart-progress')!.textContent).toBe('正在重启游戏...');
    });

    it('shows vote progress text when yes < total', () => {
      mockState.restartVotes = { yes: 1, total: 3, countdownMs: 0, receivedAt: 0 };
      syncRestartVoteProgress();
      expect(document.getElementById('restart-progress')!.textContent).toBe(
        '1/3 人已投票，还差 2 人',
      );
    });
  });

  describe('syncRestartVoteUI', () => {
    it('delegates to progress and countdown sync', () => {
      mockState.restartVotes = { yes: 1, total: 2, countdownMs: 0, receivedAt: 0 };
      syncRestartVoteUI();
      expect(document.getElementById('restart-progress')!.textContent).toContain('已投票');
    });
  });

  describe('syncRestartVoteCountdown', () => {
    it('clears countdown text when no active countdown', async () => {
      const { syncRestartVoteCountdown } = await import('./restart_vote_ui.js');
      mockState.restartVotes = { yes: 0, total: 2, countdownMs: 0, receivedAt: 0 };
      syncRestartVoteCountdown();
      expect(document.getElementById('restart-countdown')!.textContent).toBe('');
    });

    it('starts countdown timer when countdownMs > 0', async () => {
      const { syncRestartVoteCountdown } = await import('./restart_vote_ui.js');
      mockState.restartVotes = {
        yes: 1,
        total: 2,
        countdownMs: 5000,
        receivedAt: Date.now(),
      };
      syncRestartVoteCountdown();
      // Timer started; fast-forward and verify text appears.
      await new Promise((r) => setTimeout(r, 1100));
      expect(document.getElementById('restart-countdown')!.textContent).toMatch(/秒后自动重启/);
      clearRestartCountdownTimer();
    });
  });
});
