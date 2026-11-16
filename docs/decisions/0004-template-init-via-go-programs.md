# 0004 — Template initialization via self-contained Go maintenance programs

Status: Accepted (2026-11-16)

## Context

New projects initialize from this template by swapping the module path
(`github.com/kazzarahw/go-blueprint` → theirs). The original instructions
used a GNU `sed -i` one-liner. That is not portable: macOS ships BSD sed
(`-i ''`), Windows has PowerShell semantics, and AI agents driving
initialization hit exactly these edges. Additionally, the manual flow had no
end-to-end check that the renamed template actually builds and passes
verification — the failure mode is silent until the first CI run.

## Decision

Template maintenance is done by small, stdlib-only Go programs, because
anyone who can build the template already has Go:

- `scripts/init` — rewrites the module path across the repository. It is
  **deliberately self-contained**: it imports nothing from the module being
  rewritten and holds the template path as data in one constant. It refuses
  to run unless `go.mod` declares the template path, validates the new
  module path, skips `.git` and its own source, and prints every change it
  makes.
- `scripts/smoke` — copies the template to a temporary directory, renames it
  with `scripts/init`, checks for stale references, and runs `make verify`
  in the result. It runs in CI (`make smoke`) and is the template's own
  end-to-end regression test for initialization.
- A shared walker (`internal/treewalk`) backs the smoke program. `scripts/init`
  intentionally duplicates the small walker instead of importing it: the
  rewriter must remain correct even when the module does not build.
- GNU/BSD sed and PowerShell one-liners remain documented in
  DEVELOPMENT.md as manual fallbacks, not the supported path.

## Consequences

- Initialization is one command on every platform:
  `go run ./scripts/init github.com/you/yourproject`.
- The smoke test already caught a real class of failure it was designed
  against: an initializer importing template code by its (soon to be
  rewritten) module path produced a module that could not resolve its own
  packages. Self-containment eliminated the class, not just the instance.
- `make smoke` is not part of `make verify` (it re-verifies a full copy,
  doubling the run); it is a CI gate and a documented fork-time check.
- Maintenance programs live in the module and are themselves verified
  (format, vet, lint, race-tested `internal/treewalk`). They add no
  dependencies.
