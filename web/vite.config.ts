import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import basicSsl from '@vitejs/plugin-basic-ssl';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const target = env.API_TARGET || 'http://127.0.0.1:8080';
  return {
    plugins: [react(), basicSsl()],
    server: {
      host: 'localhost',
      port: 5173,
      strictPort: true,
      https: {},
      proxy: Object.fromEntries(
        ['/auth', '/rooms', '/files', '/presence', '/ws'].map((path) => [
          path, { target, changeOrigin: true, ws: path === '/ws' },
        ]),
      ),
    },
  };
});
