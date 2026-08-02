# React Admin Starter Plan

- Version: 1.0
- Status: Confirmed
- Approval source: owner approval on 2026-08-02
- Last updated: 2026-08-02

## Technical Decisions

1. Replace the Vue application in place. Keep the Admin backend endpoints,
   generated directory layout, asset embedding, CSS design language, and explicit
   module-registry concept so the change is a frontend implementation replacement,
   not a backend architecture rewrite.
2. Use React 19, React DOM, React Router, TypeScript, Vite, pnpm, and Lucide React.
   Use React context and focused hooks for app metadata, authentication, toasts,
   and module state; avoid Redux/Zustand/TanStack Query until product complexity
   demonstrates a real need.
3. Represent each Admin module as typed metadata plus a React component. Compose
   routes and navigation by mapping the explicit immutable registry. Consumer
   source owns the registry and may replace the example records module.
4. Preserve the small framework-neutral API client. Authentication state owns
   initialization, CSRF token acceptance, login, and logout. Protected routing
   waits for initialization rather than flashing protected UI.
5. Use semantic HTML and native dialog behavior with explicit focus management.
   Retain the quiet operational visual system and responsive table-to-list
   transformation; refine only where React acceptance exposes usability gaps.
6. Use Vitest, Testing Library, user-event, happy-dom, and axe-core. Behavioral
   requirements are asserted through user-visible roles and labels instead of
   component internals.
7. Produce deterministic content-hashed JavaScript and CSS filenames. Serve
   bootstrap HTML with `no-cache`, hashed assets with immutable caching, and
   compare a fresh build with the Go-embedded checked-in bundle.
8. Treat `starter/templates/admin/web` as ordinary generated consumer source.
   No reusable runtime package or Vue compatibility layer is introduced.

## Delivery Sequence

1. Add a failing generator contract that expects React and rejects Vue.
2. Replace toolchain, entry point, routing, context/state, module types, and tests;
   remove every `.vue` file and Vue dependency; restore green frontend and starter
   tests.
3. Exercise the complete UI behavior, accessibility, responsive layout, and
   browser workflows; refine implementation and visual details from findings.
4. Rebuild checked-in assets, generate an Admin consumer outside the repository,
   update canonical English and Chinese documentation, add active-source residue
   checks, and run the full release suite.
5. Perform separate spec, engineering, UX/accessibility, and release reviews;
   record evidence and mark the goal complete only when all blocking findings are
   resolved.

## Constitution Check

- Small Core and explicit composition: satisfied; React remains confined to the
  optional generated Admin source.
- Consumer ownership: satisfied; the registry and application source are copied
  once and never rewritten.
- TDD and current evidence: satisfied by generator RED, behavior tests, clean
  frontend installation, copied-out checks, visual QA, and fresh final gates.
- Accessibility and operational UX: explicitly covered by T036 and browser QA.
- No compatibility baggage: satisfied by deleting Vue rather than wrapping it.
- Immutable Alpha 3: satisfied; no tag or historical release artifact is changed.

## Risks And Controls

- Auth initialization race: protected-route tests cover pending, authenticated,
  unauthenticated, and expired-session paths.
- React effect duplication in development: providers use idempotent initialization
  and tests run under Strict Mode.
- Native dialog differences: component tests and real-browser keyboard/focus QA
  cover open, cancel, submit, close, and focus restoration.
- Stale embedded assets: deterministic asset parity is a blocking release gate.
- Documentation drift: current-doc residue check excludes immutable historical
  evidence and validates canonical English/Chinese pages.
