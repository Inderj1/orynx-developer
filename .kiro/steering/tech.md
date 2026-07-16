---
inclusion: always
---

# Tech stack & commands

## Stack

- `server/` — Go backend (Chi router, sqlc, gorilla/websocket). Go 1.26.1.
- `apps/web/` — Next.js App Router.
- `apps/desktop/` — Electron.
- `apps/mobile/` — Expo / React Native (independent; read `apps/mobile/CLAUDE.md`).
- `packages/core/` — headless business logic, API client, React Query hooks, Zustand stores.
- `packages/ui/` — atomic UI components only.
- `packages/views/` — shared business pages/components for web + desktop.
- Node 22, pnpm workspaces + Turborepo, `catalog:` for shared deps.

## Commands

```bash
make dev              # auto-setup and start the app (Postgres + backend + frontend)
make start            # start backend + frontend
make stop             # stop app processes for this checkout
make server           # run Go server only
make daemon           # run local daemon (detects agent CLIs on PATH)
make test             # Go tests
make sqlc             # regenerate sqlc after SQL changes
pnpm install
pnpm dev:web
pnpm dev:desktop
pnpm typecheck
pnpm lint
pnpm test             # TS/Vitest via Turborepo
pnpm exec playwright test
```

## Verification

Run the narrowest useful check while iterating, then broaden:
`pnpm typecheck`, `pnpm test`, `make test`, `pnpm exec playwright test`.
Never claim a check passed without running it.
