import { defineConfig } from 'vitest/config';
import { cloudflareTest } from '@cloudflare/vitest-pool-workers';

/**
 * Durable Object 集成测试配置
 *
 * 使用 @cloudflare/vitest-pool-workers 在 Miniflare 环境中运行真实 DO，
 * 捕获 Node.js 同步环境无法发现的异步交错 Bug。
 *
 * 文件扩展名 .mts 强制 ESM 模式，避免 CJS require 加载 ESM-only 包失败。
 *
 * 运行：npm run test:do
 */
export default defineConfig({
  plugins: [
    cloudflareTest({
      // Worker 入口，使 DO 绑定（LOBBY/REGISTRY/ADMIN_CONFIG）在测试中可用
      main: 'src/index.ts',
      wrangler: { configPath: './wrangler.jsonc' },
      miniflare: {
        // 确保 DO alarm 和 nodejs_compat 正常工作
        compatibilityFlags: ['nodejs_compat'],
      },
    }),
  ],
  test: {
    include: ['tests/do/**/*.test.ts'],
  },
});
