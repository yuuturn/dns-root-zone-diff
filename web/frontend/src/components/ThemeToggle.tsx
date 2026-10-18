import { MoonIcon, SunIcon } from "@phosphor-icons/react";
import { useThemeMode } from "../theme.ts";

/**
 * sun / moon ピクトグラム付きの dark / light 切り替えスイッチ。
 * つまみ (knob) が現在のテーマ側に寄り、その下のアイコンが強調される。
 * kumo に相当するコンポーネントが無いため、色は kumo のテーマ変数で組む。
 */
export function ThemeToggle() {
  const { mode, toggle } = useThemeMode();
  const isDark = mode === "dark";

  return (
    <button
      type="button"
      role="switch"
      aria-checked={isDark}
      aria-label="Dark mode"
      title={isDark ? "Switch to light mode" : "Switch to dark mode"}
      className="theme-switch"
      onClick={toggle}
    >
      <SunIcon
        className="theme-switch-icon theme-switch-icon-sun"
        size={16}
        weight="bold"
        aria-hidden="true"
      />
      <MoonIcon
        className="theme-switch-icon theme-switch-icon-moon"
        size={16}
        weight="fill"
        aria-hidden="true"
      />
      <span className="theme-switch-knob" aria-hidden="true" />
    </button>
  );
}
