import { defineConfig, loadEnv } from 'vite';
import { resolve } from 'path';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, resolve(__dirname, '..'), '');
  const backendUrl = env.BACKEND_URL || 'http://localhost:8080';

  return {
  root: '.',
  publicDir: 'public',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      input: {
        index: resolve(__dirname, 'index.html'),
        play: resolve(__dirname, 'play.html'),
        leaderboard: resolve(__dirname, 'leaderboard.html'),
      },
    },
  },
  server: {
    host: true,
    port: Number(env.FRONTEND_PORT) || 5173,
    strictPort: !!env.FRONTEND_PORT,
    proxy: {
      '/api': {
        target: backendUrl,
        ws: true,
      },
      '/lobby': {
        target: backendUrl,
        ws: true,
      },
    },
  },
};
});
