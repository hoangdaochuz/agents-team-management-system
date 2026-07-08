import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Dev server proxies /api and /api/.../stream to the Go backend so the SPA and
// API share an origin in development (avoids CORS; same-origin in prod when the
// Go binary serves the built bundle).
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: "dist",
    sourcemap: true,
  },
});
