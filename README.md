# go-blueprint

A reusable, agent-first blueprint for strict, predictable Go projects.

[![ci](https://github.com/kazzarahw/go-blueprint/actions/workflows/ci.yml/badge.svg)](https://github.com/kazzarahw/go-blueprint/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

This repository is an application-agnostic starting point for Go projects
developed primarily by AI coding agents (with humans in the loop). It pins a
strict toolchain, encodes engineering conventions as mechanically enforced
checks, and documents the working agreement for agents in [AGENTS.md](AGENTS.md).

It is **not** a framework and contains no application code. It ships one
small reference example (`internal/taskstate`) that demonstrates the
preferred conventions: pure functions, explicit state transitions, immutable
values, and deterministic tests including a fuzz target.

## What you get

- **Pinned toolchain** — Go 1.27; gofmt/goimports, go vet, staticcheck,
  golangci-lint, exhaustive, NilAway, govulncheck, race detector and native
  Go fuzzing. Tool versions live in `go.mod` (`tool` directives) and run via
  `go tool`; no global installs, no version drift between local and CI.
- **One verification gate** — `make verify` runs formatting checks, module
  tidiness, vet, static analysis, linting and race-detector tests. CI runs
  the same gate, plus a fuzz smoke test and govulncheck.
- **Agent contract** — [AGENTS.md](AGENTS.md): architecture rules
  (functional core / imperative shell), error handling, context and goroutine
  lifecycle, testing discipline, and a definition of done.
- **Docs** — [DEVELOPMENT.md](DEVELOPMENT.md) (setup, tooling, conventions),
  [TESTING.md](TESTING.md) (testing philosophy), [docs/analysis.md](docs/analysis.md)
  (static-analysis landscape and custom rule guidance), [docs/decisions/](docs/decisions/) (ADRs).

## Quick start (use this template for a new project)

```sh
# 1. Create your repo from this template (GitHub: "Use this template"),
#    or clone it directly.
git clone https://github.com/kazzarahw/go-blueprint.git myproject
cd myproject

# 2. Point the module at your project.
go mod edit -module github.com/you/myproject

# 3. Rewrite the module path references in configs and docs.
grep -rl --exclude-dir=.git 'github.com/kazzarahw/go-blueprint' . \
  | xargs sed -i 's|github.com/kazzarahw/go-blueprint|github.com/you/myproject|g'

# 4. Make it yours: keep or delete internal/taskstate, update LICENSE.
#    See DEVELOPMENT.md#initializing-a-new-project-from-this-template for the
#    full, checklist-style procedure.

# 5. Verify.
make verify
```

## Everyday commands

| Command | What it does |
|---|---|
| `make verify` | The gate: tidy, fmt, vet, staticcheck, lint, exhaustive, NilAway, race tests |
| `make test` | Tests with `-race -count=1` |
| `make fuzz FUZZTIME=1m` | Fuzz all targets (default 20s) |
| `make cover` | Coverage report (`coverage.out` + per-function summary) |
| `make fmt` / `make fmt-check` | Apply / check gofmt + goimports |
| `make lint` | golangci-lint only |
| `make vuln` | govulncheck against the latest vulnerability DB |
| `make help` | List all targets |

## Layout

```
.
├── cmd/                    # one directory per binary; empty by design (see cmd/README.md)
├── internal/               # all non-public code; never importable by other modules
│   └── taskstate/          # reference example of the preferred conventions
├── docs/
│   ├── analysis.md         # static-analysis landscape; custom go/analysis rule guidance
│   └── decisions/          # architecture decision records (ADRs)
├── .github/workflows/ci.yml
├── .golangci.yml           # golangci-lint v2 config (strict, no preset exclusions)
├── AGENTS.md               # binding instructions for coding agents
├── DEVELOPMENT.md          # setup, tooling, conventions, template initialization
├── TESTING.md              # testing philosophy and mechanics
└── Makefile                # make verify is the source of truth
```

There is deliberately **no** `pkg/`, no `api/`, no `web/`, no database
layer, no HTTP framework. Add only what your project needs, when it needs
it, following the conventions in [DEVELOPMENT.md](DEVELOPMENT.md).

## Design philosophy

- Functional core, imperative shell. Pure, explicit, testable decisions;
  I/O pushed to the edges.
- Boring over clever: stdlib first, small interfaces, explicit state
  machines, no globals, no `init()`.
- Strictness is mechanical, not aspirational: if a rule matters, it is a
  check in `make verify`, not a paragraph in a wiki.
- Small, verifiable changes. `make verify` stays green on `main` always.

## License

MIT — see [LICENSE](LICENSE). Copyright 2026 Kazzarah. New projects derived
from this template should update the copyright holder.
