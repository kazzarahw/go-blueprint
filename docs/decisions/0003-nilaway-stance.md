# 0003 — NilAway as advisory flow analysis, not a hard proof

Status: Accepted (2026-11-15)

## Context

Nil pointer dereferences are a large share of production Go panics, and
`nilaway` is the only maintained static nil-flow analysis for Go (Uber,
Apache-2.0). It publishes no tagged releases — only pseudo-versions — and
its analysis is deliberately conservative ("soundly biased"): it reports
potential nil dereferences without whole-program proof, so false positives
are a known and documented behavior, particularly around generics,
reflection, and complex interface dispatch.

Options considered:

1. Do not adopt it (status quo: race + staticcheck only).
2. Adopt it in `make verify`, treat every report as an error, and expect the
   codebase to be written so reports never appear.
3. Adopt it with an explicit policy: reports are bugs until triaged;
   suppression requires a written explanation; config stays minimal.

## Decision

Option 3. NilAway runs as a mandatory step in `make verify` and CI
(`go tool nilaway ./...`). Reports must be resolved by fixing the nil flow —
narrower types, earlier validation, explicit guards — not by suppression.
If a report is a proven false positive, the suppression must carry a
mechanism-level explanation (why the flow is safe), ideally as a tracking
issue link. If a project finds the false-positive rate disruptive, the
escape hatch is to remove NilAway from *its* `make verify` in a dedicated,
ADR-documented commit — never to weaken surrounding checks.

## Consequences

- The functional-core discipline (explicit transitions, validated inputs at
  boundaries) keeps NilAway's false positives rare; the example package
  passes clean with zero suppressions.
- A misfiring NilAway can block a change. That is accepted: the answer is
  to restructure the nil flow (usually improving the code), not to lower
  the bar.
- We accept the supply-chain posture of a pseudo-version pin (0002) — the
  alternative (tagged-only tools) would exclude the one useful nil analysis
  in the ecosystem.
