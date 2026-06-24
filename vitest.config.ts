import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    coverage: {
      provider: 'v8',
      enabled: true,
      reporter: ['text', 'html', 'lcov', 'json-summary'],
      include: [
        // 工具模块
        'src/name-generator.ts',
        'src/enemy-spawner.ts',
        'src/asset-loader.ts',
        'src/ws-manager.ts',
        'src/validators.ts',
        // 核心游戏逻辑（新增 — 之前覆盖率为 0%）
        'src/game-state.ts',
        'src/game-physics.ts',
      ],
      exclude: [
        'src/**/*.d.ts',
        'src/**/*.test.{ts,tsx}',
        'src/**/*.spec.{ts,tsx}',
        'src/test/**',
        'src/types/**'
      ],
      thresholds: {
        statements: 80,
        branches: 80,
        functions: 80,
        lines: 80
      }
    },
    include: ['tests/unit/**/*.test.ts', 'tests/integration/**/*.test.ts'],
    environment: 'node'
  }
});
