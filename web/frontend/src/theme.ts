// dark / light テーマの状態管理。
// 初期表示のちらつきを防ぐため、index.html のインラインスクリプトが React の
// マウント前に同じロジックで data-mode を適用する (キーとロジックを揃えること)。

import { useCallback, useEffect, useState } from "react";

export type ThemeMode = "light" | "dark";

/** localStorage のキー。index.html のインラインスクリプトと共通。 */
export const THEME_STORAGE_KEY = "theme";

function prefersDark(): boolean {
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

/** ユーザーが明示的に選んだテーマ。未選択なら null (システム設定に追従)。 */
export function storedMode(): ThemeMode | null {
  try {
    const value = localStorage.getItem(THEME_STORAGE_KEY);
    return value === "light" || value === "dark" ? value : null;
  } catch {
    // プライベートモードなどで localStorage が使えない場合はシステム設定に追従する
    return null;
  }
}

export function resolveMode(): ThemeMode {
  return storedMode() ?? (prefersDark() ? "dark" : "light");
}

/** kumo は data-mode="dark" でダークテーマの CSS 変数に切り替わる。 */
export function applyMode(mode: ThemeMode): void {
  document.documentElement.dataset.mode = mode;
}

export function useThemeMode() {
  const [mode, setMode] = useState<ThemeMode>(resolveMode);

  useEffect(() => {
    applyMode(mode);
  }, [mode]);

  // 明示的な選択が無い間だけ、システム設定の変更に追従する
  useEffect(() => {
    if (storedMode() !== null) return;
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => setMode(media.matches ? "dark" : "light");
    media.addEventListener("change", onChange);
    return () => media.removeEventListener("change", onChange);
  }, [mode]);

  // 別タブでの切り替えに追従する
  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
      if (event.key === THEME_STORAGE_KEY) setMode(resolveMode());
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, []);

  const toggle = useCallback(() => {
    const next: ThemeMode = mode === "dark" ? "light" : "dark";
    try {
      localStorage.setItem(THEME_STORAGE_KEY, next);
    } catch {
      // 保存できなくても表示は切り替える
    }
    setMode(next);
  }, [mode]);

  return { mode, toggle };
}
