import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.test.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      include: ['src/**/*.ts'],
      exclude: [
        'src/**/*.test.ts',
        'src/**/*_types.ts',
        'src/**/*.d.ts',
        'src/main.ts',
        'src/index.ts',
        'src/**/constants.ts',
        'src/game/renderer*.ts',
        'src/game/ui*.ts',
        'src/game/main.ts',
        'src/admin.ts',
        'src/admin_config.ts',
        'src/admin_login.ts',
        'src/leaderboard.ts',
        'src/index_leaderboard.ts',
        'src/game/connection_ui.ts',
        'src/game/tutorial.ts',
        'src/game/waiting_tips.ts',
        'src/game/visual_helpers.ts',

        'src/verify.ts',
        'src/game/restart_vote_ui.ts',
        '**/game/window_events.ts',
        '**/game/lifecycle.ts',
      ],
      // TODO: Gradually improve these thresholds.
      // Current (approximate): lines ~87%, functions ~86%, branches ~82%, statements ~87%
      thresholds: {
        lines: 85,
        functions: 85,
        branches: 80,
        statements: 85,
      },
    },
  },
});
