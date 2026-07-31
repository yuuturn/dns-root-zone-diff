import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import "@cloudflare/kumo/styles/standalone";
import "./app.css";
import { App } from "./App.tsx";
import { AnchorListPage } from "./pages/AnchorListPage.tsx";
import { DiffDetailPage } from "./pages/DiffDetailPage.tsx";
import { DiffListPage } from "./pages/DiffListPage.tsx";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <Routes>
        <Route element={<App />}>
          <Route index element={<DiffListPage />} />
          <Route path="/diffs/:id" element={<DiffDetailPage />} />
          <Route path="/anchors" element={<AnchorListPage />} />
          <Route path="/anchors/diffs/:id" element={<DiffDetailPage variant="anchors" />} />
        </Route>
      </Routes>
    </BrowserRouter>
  </StrictMode>,
);
