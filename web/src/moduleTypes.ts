import type { ComponentType } from "react";
import type { LucideIcon } from "lucide-react";
import type { Actor } from "./api";

export type ModuleRouteProps = { actor: Actor };

export type ModuleRoute = {
  id: string;
  path: string;
  Component: ComponentType<ModuleRouteProps>;
  navigation?: {
    label: string;
    icon: LucideIcon;
    permission?: string;
  };
};
