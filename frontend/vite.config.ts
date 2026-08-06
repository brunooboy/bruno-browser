import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    host: '127.0.0.1',
    // Keep the development UI away from the Discord OAuth callback server,
    // which intentionally owns localhost:34115 while login is in progress.
    port: 5173,
    strictPort: true,
  },
})
