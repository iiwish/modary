# T036 Admin Experience And Visual Acceptance

- Status: Completed
- Date: 2026-08-02
- Packet: `.ai-platform/specs/008-react-admin-starter/packets/T036.yaml`
- Execution: direct Codex execution; subagent delegation was not authorized

The React Admin work surface completes authenticated routing, login, automatic
return to sign-in after API session expiry, dedicated forbidden presentation,
records loading/error/empty/filter states, create, edit, delete confirmation,
toasts, responsive navigation, and desktop/mobile operational layouts.

Native dialogs focus the primary safe control, dismiss with Escape, and restore
focus. Closed mobile navigation is hidden and inert; opening moves focus to its
close command and Escape returns focus to the menu command. React Strict Mode is
covered and modal initialization is idempotent.

Browser review found that fixed asset names were incorrectly combined with a
one-year immutable cache policy, allowing an old Vue bundle to be reused with a
new React HTML root and produce a blank page. A failing generated-consumer test
captured the defect. Fixed-name assets use `Cache-Control: no-cache` with an ETag,
so normal reloads revalidate the current deterministic bundle.

Current browser evidence is recorded at 1440 x 900 and 390 x 844 in:
`login-desktop.jpg`, `records-desktop.jpg`, `records-mobile.jpg`,
`navigation-mobile.jpg`, `delete-mobile.jpg`, and
`records-mobile-empty.jpg`.
