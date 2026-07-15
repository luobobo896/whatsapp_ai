import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  base: process.env.VITE_APP_BASE || "/",
  plugins: [vue()],
  build: {
    emptyOutDir: true,
    outDir: "../web/dist",
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
