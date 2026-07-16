"use client";

import Link from "next/link";
import { MulticaIcon } from "@multica/ui/components/common/multica-icon";
import { useAuthStore } from "@multica/core/auth";
import { useDashboardCtaHref } from "../utils/use-dashboard-cta";

// The coding runtimes Orynx auto-detects and drives. Kept as a single quiet
// line rather than a logo wall — the one-pager stays clear, not busy.
const RUNTIMES = ["Claude Code", "Codex", "Cursor", "Copilot", "Kiro", "OpenCode"];

export function MulticaLanding() {
  const user = useAuthStore((s) => s.user);
  const ctaHref = useDashboardCtaHref();
  const ctaLabel = user ? "Open dashboard" : "Get started";

  return (
    <div className="relative flex min-h-full flex-col overflow-hidden bg-[#070b16] text-white">
      {/* Navy gradient wash — no external image, keeps the page fast and clean. */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          background:
            "radial-gradient(125% 85% at 50% -12%, #1b2c63 0%, #0c1533 46%, #070b16 100%)",
        }}
      />

      {/* Minimal top bar: wordmark + one clear action. */}
      <header className="relative z-10 mx-auto flex w-full max-w-[1100px] items-center justify-between px-6 py-6">
        <Link href="/" className="flex items-center gap-2.5">
          <MulticaIcon className="size-5 text-white" noSpin />
          <span className="text-[19px] font-semibold lowercase tracking-[0.04em] text-white/92">
            orynx
          </span>
        </Link>
        <Link
          href={ctaHref}
          className="inline-flex h-10 items-center rounded-[10px] bg-white px-4 text-[14px] font-semibold text-[#0b1330] transition-colors hover:bg-white/90"
        >
          {ctaLabel}
        </Link>
      </header>

      {/* Hero — one screen, centered, one message. */}
      <main className="relative z-10 mx-auto flex w-full max-w-[880px] flex-1 flex-col items-center justify-center px-6 pb-24 pt-8 text-center">
        <span className="mb-6 inline-flex items-center rounded-full border border-white/15 px-3.5 py-1.5 text-[11px] font-medium uppercase tracking-[0.16em] text-white/65">
          Orynx Developer
        </span>

        <h1 className="text-balance font-[family-name:var(--font-serif)] text-[2.9rem] leading-[1.02] tracking-[-0.03em] text-white sm:text-[4.1rem]">
          Human and AI teammates,
          <br />
          on one board.
        </h1>

        <p className="mt-6 max-w-[600px] text-[16px] leading-[1.7] text-white/70 sm:text-[18px]">
          Assign issues to AI agents like teammates. They pick up the work, run
          it on your own machine, comment, and update status in real time — you
          manage the whole team in one place.
        </p>

        <div className="mt-9 flex flex-wrap items-center justify-center gap-3">
          <Link
            href={ctaHref}
            className="inline-flex h-12 items-center rounded-[12px] bg-white px-6 text-[15px] font-semibold text-[#0b1330] transition-transform hover:-translate-y-0.5"
          >
            {ctaLabel}
          </Link>
          <Link
            href="/login"
            className="inline-flex h-12 items-center rounded-[12px] border border-white/18 px-6 text-[15px] font-semibold text-white/90 transition-colors hover:bg-white/[0.08]"
          >
            Sign in
          </Link>
        </div>

        <div className="mt-14 flex flex-col items-center gap-3">
          <span className="text-[11px] uppercase tracking-[0.18em] text-white/40">
            Works with your coding agents
          </span>
          <div className="flex flex-wrap items-center justify-center gap-x-2 gap-y-1.5 text-[14px] text-white/60">
            {RUNTIMES.map((name, i) => (
              <span key={name} className="flex items-center gap-2">
                {i > 0 ? <span className="text-white/20">·</span> : null}
                {name}
              </span>
            ))}
          </div>
        </div>
      </main>

      <footer className="relative z-10 mx-auto w-full max-w-[1100px] px-6 pb-8 text-center text-[13px] text-white/35">
        © 2026 Orynx Developer
      </footer>
    </div>
  );
}
