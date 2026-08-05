import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  build: {
    target: 'es2022',
    chunkSizeWarningLimit: 4500,
  },
  worker: {
    format: 'es',
  },
  // 端口与 wails3 dev 对齐：裸 `wails3 dev` 期待 9245（默认），
  // `wails3 dev -port X`（Taskfile dev 任务）通过 WAILS_VITE_PORT 覆盖。
  // host 必须 127.0.0.1：Vite 默认绑 IPv6（::1），wails 后端用 IPv4 拨号会
  // connection refused（ExternalAssetHandler proxy error）。
  server: {
    host: '127.0.0.1',
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
})
