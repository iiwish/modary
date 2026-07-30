import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EmptyState, StatusBadge } from "./components";

describe("shared operational components", () => {
  it("renders semantic status text", () => {
    render(<StatusBadge status="published" />);
    expect(screen.getByText("published")).toBeVisible();
  });

  it("renders an empty state command", () => {
    render(<EmptyState icon={<span>+</span>} title="No RuleSets" action={<button>Create RuleSet</button>} />);
    expect(screen.getByRole("heading", { name: "No RuleSets" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Create RuleSet" })).toBeEnabled();
  });
});
