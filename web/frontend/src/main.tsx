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
