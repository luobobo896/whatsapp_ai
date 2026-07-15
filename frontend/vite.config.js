import { defineConfig, loadEnv } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "VITE_");
  return {
    base: env.VITE_APP_BASE || "/",
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
  };
});
