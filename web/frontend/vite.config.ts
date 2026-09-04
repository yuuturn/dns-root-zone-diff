import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// ビルド成果物は Go バイナリに go:embed で埋め込むため internal/web/static に出力する。
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../../internal/web/static",
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // 単一チャンク (551KB) を分割し、フレームワークと UI ライブラリが
        // アプリコード変更の影響で再ダウンロードされないようにする。
        // Vite 8 (rolldown) はオブジェクト形式ではなく関数形式のみ対応。
        manualChunks(id: string) {
          if (!id.includes("node_modules")) return undefined;
          if (id.includes("@cloudflare/kumo")) return "kumo";
          return "vendor";
        },
      },
    },
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8080",
    },
  },
});
