import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    emptyOutDir: true,
    outDir: "dist",
  },
  test: {
    environment: "jsdom",
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8790",
      "/health": "http://127.0.0.1:8790",
    },
  },
});
