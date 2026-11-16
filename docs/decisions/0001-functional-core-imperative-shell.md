# 0001 — Functional core / imperative shell as the default architecture

Status: Accepted (2026-11-15)

## Context

This repository is a template for Go projects developed primarily by AI
coding agents. Agent-generated code fails most often in two places:

1. I/O and business logic fused together, producing functions that cannot be
   evaluated without standing up the world (network, clock, filesystem), so
   verification degrades into "it compiles and one happy-path test passes".
2. Implicit state (globals, struct mutation, hidden dependencies), which
   defeats local reasoning, deterministic tests, and mechanical analysis.

We need an architecture that is easy for an agent to follow mechanically
and cheap for a reviewer to verify, without imposing a domain structure.

## Decision

Default to **functional core / imperative shell**:

- Core: pure functions over plain data; no I/O, no clocks, no randomness, no
  package-level mutable state, no goroutines. Decisions and state machines
  are explicit transitions returning new values and errors.
- Shell: thin adapters that own I/O, wiring, goroutines, and cancellation.
  No domain decisions outside the core.
- Dependency injection via constructors; interfaces defined at the consumer,
  small, introduced only when a second implementation or a test fake exists.
- Immutable-by-convention data; zero values documented; explicit state
  machines for lifecycles (see `internal/taskstate` for the reference).

## Consequences

- Tests become trivial and deterministic: pure functions in, values out, no
  fakes for 90% of logic. Fuzzing the core needs no mocks at all.
- The shell is where races, leaks, and timeouts live; it stays small and is
  reviewed with corresponding care (context, goroutine lifecycle rules in
  AGENTS.md §1).
- Some duplication (passing context and dependencies explicitly instead of
  reaching for globals) is accepted as the price of local reasoning.
- Framework-shaped libraries that encourage lifecycle magic (global
  registries, `init()` wiring, reflection-based DI) are rejected by default,
  regardless of popularity.
- This does not forbid other architectures where they fit; deviations should
  be recorded as their own ADR with the forcing reason.
