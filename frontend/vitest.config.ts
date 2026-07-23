import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.test.ts'],
    setupFiles: ['./src/test-setup.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      include: ['src/**/*.ts'],
      exclude: [
        'src/**/*.test.ts',
        'src/**/*_types.ts',
        'src/**/*.d.ts',
        'src/main.ts',
        'src/**/constants.ts',
        'src/game/renderer*.ts',
        'src/game/ui*.ts',
        'src/game/main.ts',
        // Security-sensitive files are intentionally NOT excluded from coverage
        // to ensure they are measured. See audit project-10-002.
        'src/game/tutorial.ts',
        'src/game/visual_helpers.ts',
        'src/game/restart_vote_ui.ts',
        '**/game/window_events.ts',
        '**/game/lifecycle.ts',
      ],
      thresholds: {
        lines: 85,
        functions: 85,
        branches: 80,
        statements: 85,
      },
    },
  },
});
