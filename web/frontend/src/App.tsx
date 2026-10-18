import { LinkButton, Text } from "@cloudflare/kumo";
import { GithubLogoIcon, GlobeHemisphereWestIcon } from "@phosphor-icons/react";
import { Link, NavLink, Outlet } from "react-router-dom";
import { ThemeToggle } from "./components/ThemeToggle.tsx";

const GITHUB_REPO_URL = "https://github.com/yuuturn/dns-root-zone-diff";

export function App() {
  return (
    <div className="app-container">
      <header className="app-header">
        <span className="app-brand">
          <GlobeHemisphereWestIcon size={24} />
          <Link to="/">
            <Text variant="heading2" as="h1">
              DNS Root Zone Diff
            </Text>
          </Link>
        </span>
        <div className="app-header-actions">
          {/* GitHub の "View on GitHub" ボタン。kumo の LinkButton を
              secondary + external (target=_blank / rel=noopener noreferrer) で使う */}
          <LinkButton
            className="github-link"
            href={GITHUB_REPO_URL}
            external
            variant="secondary"
            size="base"
            icon={GithubLogoIcon}
            aria-label="View on GitHub"
          >
            View on GitHub
          </LinkButton>
          <ThemeToggle />
        </div>
      </header>
      <nav className="app-nav">
        <NavLink
          to="/"
          end
          className={({ isActive }) => (isActive ? "nav-link active" : "nav-link")}
        >
          Root Zone
        </NavLink>
        <NavLink
          to="/anchors"
          className={({ isActive }) => (isActive ? "nav-link active" : "nav-link")}
        >
          Root Anchors
        </NavLink>
      </nav>
      <Outlet />
    </div>
  );
}
