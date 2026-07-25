import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// ビルド成果物は Go バイナリに go:embed で埋め込むため internal/web/static に出力する。
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../../internal/web/static",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8080",
    },
  },
});
