---
inclusion: always
---

# Structure & hard boundaries

`CLAUDE.md` at the repo root is the authoritative rulebook — read it. This file
is the Kiro-facing digest of the constraints that are easiest to get wrong.

## Dependency direction

`views -> core + ui`. `core` and `ui` must stay independent.

## Package boundaries (hard)

- `packages/core/`: no `react-dom`, `localStorage` (use `StorageAdapter`),
  `process.env`, or UI libraries.
- `packages/ui/`: no `@multica/core` imports, no business logic.
- `packages/views/`: no `next/*`, no `react-router-dom`, no stores. Use
  `NavigationAdapter`, `useNavigation()`, `<AppLink>`.
- `apps/web/platform/`: only place for Next.js navigation/platform APIs.
- `apps/desktop/src/renderer/src/platform/`: only place for `react-router-dom`.

## State rules

- TanStack Query owns server state; Zustand owns client/view state. Never mirror
  server payloads into Zustand.
- Workspace-scoped query keys must include `wsId`.
- Optimistic updates only when outcome is locally predictable, user stays on the
  same screen, failure is rare, and rollback is trivial. Create/delete/navigate
  flows await the server first.

## Database & migrations (hard)

- No FOREIGN KEY / REFERENCES, no cascading deletes/updates. Resolve
  relationships and cleanup in application code, in a transaction when atomic.
- Every migration index uses `CREATE INDEX CONCURRENTLY`, one statement per file.

## API compatibility

- Parse API JSON with `parseWithFallback` (`packages/core/api/schema.ts`) + a zod
  schema. Never cast network JSON to `T`. Enum switches need a `default` branch.

## Conventions

- TypeScript strict; Go uses gofmt/go vet/checked errors. Comments in English.
- Prefer existing patterns/components over new parallel abstractions.
- i18n glossary + Chinese voice source of truth:
  `apps/docs/content/docs/developers/conventions.mdx` (and `.zh.mdx`).
