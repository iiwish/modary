# React Admin Starter

- Version: 1.0
- Status: Confirmed
- Approval source: owner approval of the React-only migration on 2026-08-02
- Last updated: 2026-08-02
- Governing constitution: `../../memory/constitution.md`
- Supersedes: feature 007 requirements FR-013 and frontend-specific wording in FR-019

## Objective

Make React the sole first-party frontend implementation for the optional Modary
Admin Profile. The migration removes Vue source, packages, tooling, and
compatibility surfaces rather than maintaining two frontend stacks. Modary Core,
backend components, HTTP contracts, and Profile composition remain frontend
neutral.

## User Stories

- US-001: a Go developer creates an Admin Profile and receives a conventional,
  consumer-owned React application that can be understood, changed, tested, and
  built without learning a Modary-specific frontend runtime.
- US-002: an administrator can sign in, inspect and mutate the generated example
  records, recover from errors, and sign out on desktop or mobile with keyboard
  and assistive-technology support.
- US-003: a framework maintainer can prove that generated source and production
  assets contain React and contain no Vue compatibility residue.

## Functional Requirements

- FR-001: the Admin frontend uses React, TypeScript, Vite, React Router, pnpm,
  Lucide React, and a deliberately small state architecture based on React
  context and hooks; it does not introduce a general state library without a
  demonstrated need.
- FR-002: Vue, Pinia, Vue Router, Vue compiler/test packages, `.vue` files, and
  Vue-specific configuration are absent from the current Admin template,
  generated consumers, lockfile, and production bundle.
- FR-003: the backend Admin API and security contract remain unchanged: same-site
  cookie sessions, current-session bootstrap, CSRF on state-changing requests,
  authenticated routes, authoritative backend permissions, and logout.
- FR-004: the explicit frontend module registry remains the composition boundary
  for routes and navigation. It is ordinary consumer-owned source and is never
  patched by a regeneration command.
- FR-005: the UI provides sign-in, sign-out, authenticated routing, current actor,
  responsive navigation, record list/search/status filtering, create, edit,
  delete confirmation, loading, empty, error, forbidden, and session-expiry
  behavior.
- FR-006: dialogs manage initial focus and return focus, mobile navigation is
  focusable only while open, controls have accessible names, error feedback is
  announced, and core flows pass automated accessibility checks.
- FR-007: existing backend/profile packages do not import or expose React types;
  frontend selection is an Admin Starter implementation decision, not a Core
  runtime dependency.
- FR-008: generated source remains buildable and testable outside the Modary
  checkout with `GOWORK=off`; the checked-in production bundle is reproducible
  from the checked-in lockfile and matches the generated project.
- FR-009: English and Chinese onboarding documentation identify React as the
  official Admin frontend and explain the module registry, development loop,
  production asset build, backend boundary, and customization ownership.
- FR-010: release checks reject Vue residue in active source and generated
  output while allowing immutable historical release evidence to retain its
  original terminology.

## Non-Functional Requirements

- NFR-001: TypeScript strict mode, ESLint, unit/component tests, production build,
  deterministic asset checks, Go tests, vet, race tests, repeat tests, and copied-
  out Profile checks pass.
- NFR-002: the initial production JS and CSS bundle remains appropriate for a
  compact Admin shell; unnecessary UI frameworks and duplicate state libraries
  are not added.
- NFR-003: the desktop and mobile UI has no clipped controls, incoherent overlap,
  horizontal document overflow, blank routes, or inaccessible modal/navigation
  state in the accepted viewports.
- NFR-004: reduced-motion preferences are respected; animations are restrained,
  functional, and do not shift fixed controls or table geometry.
- NFR-005: no new compatibility adapter, parallel Vue package, migration command,
  or generated-source rewriter is added.

## Acceptance Criteria

- AC-001: `rg -i "vue|pinia"` finds no active Vue implementation or dependency
  under `starter/templates/admin`, generated Admin source, current product docs,
  or current release checks; documented historical evidence is explicitly
  excluded.
- AC-002: all frontend lint, strict typecheck, Vitest, build, and asset parity
  commands pass from a clean frozen-lockfile installation.
- AC-003: focused tests prove authenticated redirects, login/logout, CSRF,
  module registry composition, record CRUD/error states, dialog focus behavior,
  and accessible names.
- AC-004: desktop and mobile browser acceptance proves login and record workflows,
  responsive navigation, keyboard focus, nonblank rendering, zero document
  overflow, and no console or network errors in the primary path.
- AC-005: copied-out Admin generation builds and tests Go and React source,
  serves the embedded production bundle, and contains no Vue artifacts.
- AC-006: current English and Chinese docs, support matrix, release guidance,
  canonical product design, and TDR agree on React-only Admin ownership.
- AC-007: final spec-compliance, engineering-quality, UX/accessibility, and
  release-readiness reviews have no unresolved P0, P1, or P2 finding.

## Non-Goals

- Supporting Vue and React simultaneously.
- A compatibility package, migration codemod, or automated conversion of
  consumer-written Vue applications.
- Changing Admin backend HTTP routes, PostgreSQL schema, session format, RBAC,
  Governed Profile, or Core composition.
- Adding a component library, visual page builder, low-code editor, server-state
  cache, or global state framework merely to demonstrate ecosystem breadth.
- Building downstream product screens as part of the framework migration.

## Constraints And Stop Conditions

The work proceeds directly because the owner explicitly approved the React-only
strategy and removal of compatibility baggage. Execution pauses only if the
existing Admin HTTP contract cannot support behavior parity, if the migration
requires a backend/security contract break, if current user changes conflict
irreconcilably with the bounded frontend work, or if release validation exposes
an unrelated destructive migration requirement.
