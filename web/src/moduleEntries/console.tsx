import { Navigate } from "react-router-dom";
import type { ModuleRoute } from "../moduleTypes";

export const routes: Record<string, ModuleRoute> = {
  "console.home": {
    id: "console.home",
    path: "/",
    Component: () => <Navigate to="/rulary/rules" replace />,
  },
};
