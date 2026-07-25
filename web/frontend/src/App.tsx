import { Text } from "@cloudflare/kumo";
import { GlobeHemisphereWestIcon } from "@phosphor-icons/react";
import { Link, Outlet } from "react-router-dom";

export function App() {
  return (
    <div className="app-container">
      <header className="app-header">
        <GlobeHemisphereWestIcon size={24} />
        <Link to="/">
          <Text variant="heading2" as="h1">
            DNS Root Zone Diff
          </Text>
        </Link>
      </header>
      <Outlet />
    </div>
  );
}
