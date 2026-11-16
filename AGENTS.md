# AGENTS.md — Persistent instructions for AI coding agents

This file binds every agent (autonomous or assisted) working in this
repository. It is part of the contract of the project. Read it before your
first change. If an instruction here conflicts with a task prompt, say so
explicitly instead of silently ignoring it.

The companion documents are equally binding:

- [README.md](README.md) — what this repository is and how to start from it.
- [DEVELOPMENT.md](DEVELOPMENT.md) — setup, tooling, verification pipeline, conventions.
- [TESTING.md](TESTING.md) — testing philosophy and mechanics.
- [docs/decisions/](docs/decisions/) — recorded architectural decisions (ADRs).

## 0. Non-negotiables

1. **Never weaken verification.** Do not disable linters, skip hooks, delete
   tests, lower coverage, add `//nolint` without a specific linter name and a
   written explanation, or edit CI to make a failing change pass. If a check
   is wrong, fix the check in a dedicated commit with written rationale.
2. **`make verify` is the definition of done.** A change is complete only
   when `make verify` exits 0. Do not claim completion before running it.
3. **Never bypass the module system.** Do not hand-edit `go.sum`. Change
   dependencies only with `go get` / `go mod tidy`.
4. **Never fabricate results.** Report exactly what you ran and what it
   printed. If you could not verify something, say so.

## 1. Architecture conventions

### Functional core / imperative shell

- **Functional core**: pure functions and plain values. No I/O, no clocks,
  no randomness, no package-level mutable state, no goroutines. Decisions
  are expressed as data transformations and explicit state transitions.
- **Imperative shell**: thin adapters around the core. It owns I/O — HTTP,
  disk, network, env, logging, timers — and wires dependencies together.
  Business logic does not live in the shell.
- Push decision-making down into the core. Keep the shell boring: parse
  (or fail) at the boundary, call the core, map the result to output.

### Data and state

- Immutable by convention: pass structs by value, return new values instead
  of mutating, share nothing mutable without explicit ownership.
- Explicit state transitions: when a type has a lifecycle, model it as an
  explicit state with pure transition functions that return
  `(newValue, error)`. See `internal/taskstate` for the reference example.
- No global mutable state. Package-level `var` is for sentinel errors and
  other genuinely immutable values only.
- No `init()` functions. Construction happens explicitly, at the entry point.
- No package-level singletons. Dependencies are passed, not reached for.

### Errors

- Handle every error explicitly. `_ = f()` and `v, _ := f()` discard errors
  silently and are forbidden — errcheck with `check-blank: true` enforces
  this and fails the build.
- Wrap with `%w` and match with `errors.Is` / `errors.AsType`. Sentinels are
  `var ErrX = errors.New("...")`; custom error types carry structured detail
  and wrap a sentinel. (`errors.As` with a pointer-to-pointer target is the
  equivalent older idiom.)
- Do not branch on error strings. Do not panic for expected failures; panics
  are for programmer errors and irrecoverable startup failures only.

### Context and concurrency

- `context.Context` is the first parameter, named `ctx`. Propagate it; do
  not store it in structs; do not pass `nil`.
- Every goroutine has a clear owner, a cancellation mechanism (channel or
  context), and a shutdown path (join via `sync.WaitGroup` / `errgroup` or a
  done channel). Fire-and-forget goroutines are forbidden.
- No goroutines in the functional core. Concurrency is a shell concern.
- Prefer channels for ownership transfer; prefer mutexes for protecting
  shared state; document which model you used and why.
- Respect cancellation: check `ctx.Err()` in loops, select on `ctx.Done()`.

### Interfaces and dependencies

- Interfaces are defined at the consumer, are small (1–3 methods), and are
  introduced only when there are at least two implementations or a real need
  to fake in tests.
- Dependency injection via constructors: `func New(dep Dep) *T`. No hidden
  dependencies, no service locators, no `init()` wiring.
- Depend on concretions until the seam is needed; introduce the interface
  at the first actual second implementation or test fake.

### Naming and layout

- Package names: short, lowercase, singular, no underscores or stuttering
  (`taskstate`, not `task_state` or `util`). No `utils`, `helpers`,
  `common`, `misc` dumping grounds.
- `cmd/<binary>/` for entry points; `internal/` for all non-public code;
  no `pkg/` unless a directory is deliberately a public API you want
  imported (rare; argue it in an ADR first).
- One responsibility per package; a package is a good unit of review.

## 2. Testing

- Write tests alongside implementation in the same change. A feature without
  tests is not done.
- Table-driven tests are the default. Make the table *be* the spec:
  enumerate cases as data, iterate, subtest with `t.Run`.
- No package-level mutable state in tests either: build fixtures inside the
  test (see the constructor functions in `internal/taskstate/taskstate_test.go`).
- Deterministic always: no wall clock, no randomness without a seed, no
  sleeps, no network, no real filesystem unless `t.TempDir()`. Inject clocks
  and randomness via small interfaces or function fields.
- Fuzz targets for anything that parses untrusted input or implements state
  machines / invariants. Keep corpus seeds in `testdata/` (committed).
- Run the race detector (`make test` already does).
- Tests must fail when the code is wrong. Assert behavior and invariants,
  not error strings or implementation details.
- See [TESTING.md](TESTING.md) for mechanics and examples.

## 3. Verification

`make verify` runs, in order:

1. `tidy-check` — go.mod/go.sum must be tidy (`go mod tidy -diff`).
2. `fmt-check` — gofmt and goimports (local prefix) clean.
3. `vet` — `go vet ./...`.
4. `staticcheck` — honnef.co/go/tools staticcheck.
5. `lint` — golangci-lint: govet with shadow, staticcheck, errcheck,
   errorlint, errname, exhaustive, gosec, gocritic, gocognit, revive, and more.
6. `exhaustive` — standalone exhaustive switch analysis.
7. `nilaway` — NilAway static nil-flow analysis.
8. `test` — `go test -race -count=1 ./...`.

CI additionally runs a fuzz smoke test and `govulncheck`.

Rules:

- Run `make verify` before declaring any change complete. Report failures
  honestly; a red pipeline is not done.
- Fix code, not checks. If a linter or the config is genuinely wrong,
  change it in a dedicated commit with written rationale, and never as part
  of a feature change.
- `//nolint:<linter> // <reason>` is the only acceptable suppression, always
  with a specific linter and explanation. nolintlint enforces this.
- Warnings are errors in spirit: do not leave "pre-existing" lint noise in
  files you touch; fix it in the same change if small, otherwise in a
  separate cleanup commit.

## 4. Change discipline

- Small, single-purpose changes. One logical change per commit. If you
  cannot describe the change in one sentence, split it.
- Keep the build green at every commit: format → static checks → tests.
- Do not refactor adjacent code "while you're in there". Note it; fix it in
  its own change if it matters.
- Do not introduce abstractions before the second caller exists.
- New dependency? Justify it in the commit message or an ADR: why stdlib is
  insufficient, maintenance status, license, size, security posture. Prefer
  stdlib. Reject dependencies that "may be useful later".
- Avoid generated code unless a generator is checked in and reproducible.

## 5. Documentation

- Exported symbols have doc comments that say what they do (revive enforces
  presence). Comments explain *why*, not *what* — the code says what.
- Meaningful architectural decisions are recorded as ADRs in
  `docs/decisions/NNNN-short-title.md`. See
  [docs/decisions/README.md](docs/decisions/README.md) for the format and
  index. If you chose between two reasonable approaches, write it down.
- Keep README.md, DEVELOPMENT.md and TESTING.md accurate. Stale docs are
  bugs; update them in the change that invalidates them.
- Public behavior changes require matching doc updates in the same commit.

## 6. Definition of done

A change is complete when **all** hold:

- [ ] `make verify` exits 0 (output shown, not asserted from memory).
- [ ] New behavior has deterministic tests; new state machines/parsers have fuzz targets.
- [ ] No errors are silently discarded; no new globals, `init()`, or untracked goroutines.
- [ ] Doc comments on new exported symbols; ADRs for decisions that outlive the change.
- [ ] README/DEVELOPMENT/TESTING updated if behavior, commands, or conventions changed.
- [ ] No debug scaffolding, commented-out code, or TODOs without an owner and a linked issue.

## 7. Working effectively in this repo

- First commands: `git status`, read this file, read `DEVELOPMENT.md`,
  then `make verify` to see the baseline (it must be green before you touch
  anything).
- The example in `internal/taskstate` is the template for style: copy it.
- When in doubt, choose the boring option: stdlib over dependency, value
  over pointer, explicit over clever, small over general.
- If the task is ambiguous or this file seems wrong, surface it — do not
  silently improvise around the contract.
