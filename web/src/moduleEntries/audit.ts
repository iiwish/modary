import { FileClock } from "lucide-react";
import { AuditPage } from "../pages/AuditPage";
import type { ModuleRoute } from "../moduleTypes";

export const routes: Record<string, ModuleRoute> = {
  "audit.logs": {
    id: "audit.logs",
    path: "/audit",
    Component: AuditPage,
    navigation: { label: "Audit", icon: FileClock, permission: "audit.read" },
  },
};
