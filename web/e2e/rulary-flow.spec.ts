import { expect, test } from "@playwright/test";

test("Rulary governed action flow", async ({ page }, testInfo) => {
  await page.goto("/");
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("e2e-password");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: "RuleSets", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "New RuleSet" }).click();
  await page.getByRole("textbox", { name: "Name", exact: true }).fill(`E2E address labels ${Date.now()}`);
  await page.getByRole("button", { name: "Create", exact: true }).click();
  await expect(page.getByText("RuleSet", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Validate" }).click();
  await expect(page.getByText("Draft validated")).toBeVisible();
  await page.getByRole("button", { name: "Preview", exact: true }).click();
  await expect(page.getByText("平顶山市卫东区建设路东段南4号院", { exact: true }).first()).toBeVisible();

  await page.getByRole("button", { name: "Publish", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "Publish version" })).toBeVisible();
  await page.getByRole("dialog", { name: "Publish version" }).getByRole("button", { name: "Publish" }).click();
  await expect(page.getByText("Version published")).toBeVisible();

  await page.getByLabel("Row limit").fill("20");
  await page.getByRole("button", { name: "Preview run" }).click();
  const executeDialog = page.getByRole("dialog", { name: "Execute run" });
  await expect(executeDialog).toBeVisible();
  await executeDialog.getByRole("button", { name: "Execute" }).click();
  await expect(page.getByText("Run completed")).toBeVisible();
  await page.getByRole("button", { name: "Open run" }).click();
  await expect(page).toHaveURL(/\/rulary\/runs\//);
  await expect(page.getByText("succeeded", { exact: true })).toBeVisible();
  await expect(page.getByText("平顶山示例企业")).toBeVisible();
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeLessThanOrEqual(100);

  const viewport = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
    scrollY: window.scrollY,
  }));
  expect(viewport.scrollWidth).toBeLessThanOrEqual(viewport.clientWidth + 1);

  await page.screenshot({
    path: testInfo.outputPath("rulary-run.png"),
    fullPage: testInfo.project.name === "chromium",
    animations: "disabled",
  });
});

test("audit surface records governed execution", async ({ page }, testInfo) => {
  await page.goto("/");
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("e2e-password");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: "RuleSets", exact: true })).toBeVisible();
  const menuButton = page.getByRole("button", { name: "Open navigation" });
  if (await menuButton.isVisible()) {
    await menuButton.click();
  }
  await page.getByRole("link", { name: "Audit" }).click();
  await expect(page.getByRole("heading", { name: "Audit", exact: true })).toBeVisible();
  await expect(page.getByText("rulary.ruleset.list").first()).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("audit.png"),
    fullPage: testInfo.project.name === "chromium",
    animations: "disabled",
  });
});
