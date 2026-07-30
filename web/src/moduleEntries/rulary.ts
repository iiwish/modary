import { ClipboardCheck } from "lucide-react";
import { RuleSetPage } from "../pages/RuleSetPage";
import { RuleSetsPage } from "../pages/RuleSetsPage";
import { RunPage } from "../pages/RunPage";
import type { ModuleRoute } from "../moduleTypes";

export const routes: Record<string, ModuleRoute> = {
  "rulary.rules": {
    id: "rulary.rules",
    path: "/rulary/rules",
    Component: RuleSetsPage,
    navigation: { label: "RuleSets", icon: ClipboardCheck },
  },
  "rulary.rules.detail": {
    id: "rulary.rules.detail",
    path: "/rulary/rules/:rulesetId",
    Component: RuleSetPage,
  },
  "rulary.runs.detail": {
    id: "rulary.runs.detail",
    path: "/rulary/runs/:runId",
    Component: RunPage,
  },
};
