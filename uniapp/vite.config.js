import { defineConfig, loadEnv } from 'vite'
import uni from '@dcloudio/vite-plugin-uni'
// https://vitejs.dev/config/
export default defineConfig(({ mode }) => ({
  plugins: [
    uni(),
  ],
  server: {
    proxy: {
      '/api': { target: loadEnv(mode, process.cwd(), '').API_PROXY_TARGET || 'http://127.0.0.1:8080', changeOrigin: false },
    },
  },
}))
