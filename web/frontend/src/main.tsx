import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import "@cloudflare/kumo/styles/standalone";
import "./app.css";
import { App } from "./App.tsx";
import { DiffDetailPage } from "./pages/DiffDetailPage.tsx";
import { DiffListPage } from "./pages/DiffListPage.tsx";
import { TLDHistoryDetailPage } from "./pages/TLDHistoryDetailPage.tsx";
import { TLDHistoryListPage } from "./pages/TLDHistoryListPage.tsx";

// React Router はモジュール読み込み時に window.__reactRouterVersion へ
// 自身のバージョン (例: "7.18.6") を書き出す。Wappalyzer などの技術検出ツールは
// このグローバル変数を読んでバージョンを特定するため、起動直後に削除する。
// アプリの動作には使われない (React Router DevTools 向けの情報)。
// react-router を更新してこの挙動が変わったら、ビルド後の vendor チャンクに
// window.__reactRouterVersion の代入が残っていないか確認すること。
delete (window as unknown as { __reactRouterVersion?: string }).__reactRouterVersion;

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <Routes>
        <Route element={<App />}>
          <Route index element={<DiffListPage />} />
          <Route path="/diffs/:id" element={<DiffDetailPage />} />
          <Route path="/anchors" element={<DiffListPage variant="anchors" />} />
          <Route path="/anchors/diffs/:id" element={<DiffDetailPage variant="anchors" />} />
          <Route path="/tlds" element={<DiffListPage variant="tlds" />} />
          <Route path="/tlds/diffs/:id" element={<DiffDetailPage variant="tlds" />} />
          <Route path="/tld-history" element={<TLDHistoryListPage />} />
          <Route path="/tld-history/:tld" element={<TLDHistoryDetailPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  </StrictMode>,
);
