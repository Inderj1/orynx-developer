# Agent execution sandbox (blast-radius reduction)

Status: proposed · Owner: platform/security · Tracking: punch-list #1/#2/#10

## Problem

Agents run Claude Code with `--permission-mode bypassPermissions` (every tool
auto-approved) as the host `ubuntu` user, with **no per-run isolation**. A live
audit of the self-host box found that a single confused or prompt-injected run
can reach host root and the operator's real credentials:

- `ubuntu` has **passwordless `sudo (ALL) NOPASSWD:ALL`** *and* is in the
  **`docker`** group → two independent, unprompted paths to host root
  (`sudo …`, `docker run -v /:/host …`).
- Readable on disk by the agent process: the GitHub token (`repo` +
  `admin:public_key`) + SSH private key, Claude OAuth creds
  (`~/.claude/.credentials.json`), the daemon PAT (`~/.orynx-daemon-pat`), the
  product Postgres DB, and world-readable Terraform state.
- All agents share one uid → cross-run/cross-workspace read and tamper of every
  workdir under `~/multica_workspaces/`.
- Unrestricted egress → any of the above can be exfiltrated.

The only containment today is env-var scrubbing in `server/pkg/agent/claude.go`
(`filterInheritedEnv`), which is cosmetic: it removes `DATABASE_URL` from the
child's *environment* while the agent reads the same secrets straight off disk.

The single spawn point is `server/pkg/agent/claude.go:60`:
```go
cmd := exec.CommandContext(runCtx, execPath, args...)
```
`SysProcAttr` sets only `Setpgid` (kill semantics, not isolation). There is no
Linux sandbox — `execenv/codex_sandbox.go` is a macOS-Seatbelt policy for the
Codex backend and is inert on the Linux host.

## Hard constraint

Agents **must retain the ability to build and test the apps they ship**, which
means running `docker compose up` (the verification loop depends on it — see the
scaffold proof, where the Developer/Reviewer built a live stack). So we cannot
simply drop the `docker` group or block containers. Exposing the host Docker
socket to a sandbox re-grants root, so the sandbox must provide containers
*without host-root-equivalent access*.

## Recommended approach — dedicated low-priv user + rootless containers

Two layers, landed incrementally behind a config flag (default off) so each PR
is safe to merge and the cutover is a deliberate, tested switch.

### Layer 1 — identity separation (box, NOT per-agent code)

Correctness note: the daemon runs **non-root** on purpose (`claudeRootSudoPreflight`
refuses to start as root), and a non-root parent **cannot `setuid`** its children
to a different user. So the per-agent `SysProcAttr.Credential` drop is a dead end
here — it would require a root daemon. The right, simpler approach is to run the
**whole daemon as a dedicated low-priv `orynx-agent` user**; every agent child
then inherits that unprivileged uid with no drop needed. This is an ops migration
(systemd `User=orynx-agent`), not an agent-core code change.

- Create **`orynx-agent`**: **not** in `sudoers`, **not** in the host `docker`
  group, its own home. Flip the systemd unit `User=ubuntu` → `User=orynx-agent`.
- `orynx-agent` gets its own auth (it can't read `ubuntu`'s): a **Claude login**
  (interactive, one-time) and a GitHub credential that is a **per-run, repo-scoped
  GitHub App installation token** (#2) — never the long-lived PAT + SSH key. The
  daemon PAT is copied into `orynx-agent`'s home (0600). The operator's
  `~ubuntu/.claude`, `~ubuntu/.ssh`, `~ubuntu/.config/gh`, and the Terraform state
  stay owned by `ubuntu` and become **unreadable** to the agent uid.
- Result: even with `bypassPermissions`, the agent has no `sudo`, no host Docker
  socket, and cannot read the operator's GitHub/Claude/PAT/DB material.

Interactive steps the operator must run (cannot be automated): `claude login` as
`orynx-agent`, and creating the GitHub App (#2). Everything else (user creation,
home, rootless runtime, unit edit, perms) is scripted and reversible (flip
`User=` back to `ubuntu` to roll back instantly).

### Layer 2 — containers without host root (rootless runtime)
- Give `orynx-agent` a **rootless container runtime** (rootless Podman, or
  rootless dockerd) so `docker compose`/`podman compose` still works for the
  build/test loop, but containers map to the unprivileged uid — no path to host
  root via the socket.
- Per-run workdir stays private to the uid (`0700`), so runs can't read each
  other.

### Layer 3 (optional, later) — wrap the child
- At `claude.go:60`, when `MULTICA_AGENT_SANDBOX` is set, prefix the exec with a
  `bwrap`/`nsjail` profile: read-only host mounts, a private `/tmp`, dropped
  capabilities, and an **egress allowlist** (Anthropic API, the app's own
  services, package registries) via a per-run netns + proxy. This hardens the FS
  and network view even further. Kept as a separate, optional layer because the
  identity+rootless layers already remove the root blast radius.

### Companion — resource caps (#10)
Run the daemon under **systemd** with `MemoryMax`/`CPUQuota` and
`MULTICA_DAEMON_MAX_CONCURRENT_TASKS=3` (the box is 2 vCPU / 8 GB; the current
default of 20 concurrent Claude processes will thrash swap / OOM). This also
fixes the "daemon is a detached, unmanaged process" fragility.

## Config surface (new)
- `MULTICA_AGENT_RUN_AS_USER` — uid/username to drop to for the agent child
  (empty = current behavior; set = `SysProcAttr.Credential`).
- `MULTICA_AGENT_SANDBOX` — `off` (default) | `bwrap` — enable the Layer-3 wrapper.
- `MULTICA_AGENT_CONTAINER_RUNTIME` — `docker` (default) | `podman-rootless`.

Defaults preserve today's behavior; hardening is opt-in per environment and
flipped on only after the box-side setup (dedicated user, rootless runtime,
GitHub App) is in place and a test run passes.

## Rollout order
1. #10 — daemon → systemd with caps (done separately; unblocks safe restarts).
2. #2 — GitHub App + per-run token minting (removes the highest-value secret).
3. Layer 1 — `orynx-agent` user + `SysProcAttr.Credential` drop, secrets moved
   out of reach; verify an agent run still builds/tests via rootless runtime.
4. Layer 2 — rootless container runtime for the build/test loop.
5. Layer 3 — optional `bwrap` FS/egress hardening.

Each step is independently mergeable and independently reversible.

## Verification
- A canary run that attempts `sudo -n true`, reads `~/.claude/.credentials.json`,
  and `docker run -v /:/host …` must **fail** post-cutover.
- The scaffold proof (build + `docker compose up` + live flow) must still **pass**
  under the rootless runtime.
