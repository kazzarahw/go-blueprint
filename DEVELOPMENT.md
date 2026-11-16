# DEVELOPMENT.md — Working in this repository

For humans and agents. Read [AGENTS.md](AGENTS.md) first for the binding
rules; this document covers setup, tooling mechanics, and conventions in
depth.

## Prerequisites

- **Go 1.27+** (`go version`). The `go` directive in `go.mod` is the
  minimum and what CI installs. Bump it deliberately, as its own commit.
- **make** — the Makefile is the interface to everything here.
- **git** — hooks are not enforced; CI is. Do not rely on local hooks.
- No other tooling needs manual installation. Every analysis tool is pinned
  in `go.mod` as a `tool` directive and runs via `go tool <name>`.

## First-time setup

```sh
git clone https://github.com/kazzarahw/go-blueprint.git
cd go-blueprint
make verify   # expect: all steps pass, ending with "ok  github.com/.../taskstate"
```

If `make verify` is red on a fresh clone of this template, something is
wrong with the environment (e.g. Go older than 1.27) — not with you.

## How tool pinning works (and why)

All developer tools are declared in `go.mod`:

```
tool (
    github.com/golangci/golangci-lint/v2/cmd/golangci-lint
    github.com/nishanths/exhaustive/cmd/exhaustive
    go.uber.org/nilaway/cmd/nilaway
    golang.org/x/tools/cmd/goimports
    golang.org/x/vuln/cmd/govulncheck
    honnef.co/go/tools/cmd/staticcheck
)
```

Consequences:

- `go tool golangci-lint run ./...` uses the exact version in `go.sum`.
  Local runs, CI, and agent runs are the same binary. No `go install ...@latest`.
- Upgrading a tool is a normal, reviewable dependency change:
  `go get -tool <module-path>@<version> && go mod tidy`, then run
  `make verify` and commit `go.mod`/`go.sum` together. Bump tools in
  dedicated commits, never mixed into feature changes.
- staticcheck 2026.2.2 (`honnef.co/go/tools` v0.8.2), golangci-lint v2.15.0,
  exhaustive v0.17.1, govulncheck v1.9.0, goimports (x/tools v0.52.0),
  NilAway `v0.0.0-20261112030514-ccd005690d1e` (NilAway publishes no tagged
  releases; the pseudo-version pins the commit).
- Trade-off: the tools pull a large module graph into `go.sum`. That is the
  price of reproducibility without a separate tools module; it is
  acceptable for a template because it keeps exactly one place to look.

## The verification pipeline

`make verify` runs, in order. CI runs exactly the same command.

| Stage | Command | Catches |
|---|---|---|
| `tidy-check` | `go mod tidy -diff` | undeclared/unused/mis-specified deps |
| `fmt-check` | `gofmt -l .` + `goimports -local <prefix> -l .` | formatting and import grouping drift |
| `vet` | `go vet ./...` | suspicious constructs (printf, range, atomic misuse, ...) |
| `staticcheck` | `go tool staticcheck ./...` | SA/S/ST/QF/U bug classes |
| `lint` | `go tool golangci-lint run ./...` | the rule set in `.golangci.yml` |
| `exhaustive` | `go tool exhaustive ./...` | non-exhaustive switches over enums |
| `nilaway` | `go tool nilaway ./...` | likely nil dereferences (flow analysis) |
| `test` | `go test -race -count=1 ./...` | logic failures + data races |

CI additionally runs `make fuzz FUZZTIME=30s` and `make vuln`.

Also available: `make cover` (per-function coverage), `make build`,
`make fmt` (applies gofmt+goimports via golangci-lint), `make tidy`,
`make clean`, `make help`.

### What the lint config enforces (and what it deliberately does not)

Enabled: `errcheck` (with `check-blank: true`, so `v, _ := f()` fails the build), `errorlint`, `errname`, `govet` (with `shadow`),
`staticcheck`, `unused`, `ineffassign`, `wastedassign`, `unconvert`,
`unparam`, `exhaustive`, `gosec`, `gocritic`, `gocognit` (max 15),
`revive` (exported docs, error flow, naming, defer), `misspell`,
`perfsprint`, `usestdlibvars`, `forcetypeassert`, `fatcontext`,
`containedctx`, `contextcheck`, `copyloopvar`, `gochecknoinits`,
`nolintlint` (no unexplained suppressions).

Deliberate decisions, documented so they are not re-litigated casually:

- **No preset exclusion lists** in `.golangci.yml`. Every enabled linter
  must pass on the whole tree. If you need an exclusion, that is a design
  smell: fix the code, or justify a config change in a dedicated commit.
- **`gochecknoglobals` is not enabled.** Sentinel errors (`var ErrX = ...`)
  are the idiomatic Go exception it cannot distinguish; we accept immutable
  globals and police *mutable* global state by convention and review
  (AGENTS.md §1). If your project wants it, enable it and budget for the
  sentinel-error friction.
- **`err113`, `funlen`, `lll`, `nlreturn`, `wsl`, `exhaustruct`, `mnd`**
  are not enabled: they enforce stylistic preferences or produce high-noise
  findings with little defect yield. `gocognit` covers complexity.
- **NilAway is not a proof.** It is a best-effort flow analysis; it finds
  real bugs and can miss others. Treat a NilAway report as a bug report:
  fix the nil flow. Do not suppress without a written explanation.
- **`goimports` local prefix is `github.com/kazzarahw/go-blueprint`** —
  rewrite it to your module path when initializing a new project (the
  rewrite steps are below).

See [docs/analysis.md](docs/analysis.md) for the broader survey of Go
analysis tooling and when to add custom `go/analysis` rules.

## Architecture conventions

### Functional core, imperative shell

Keep decisions in pure functions over plain data; keep I/O at the edges.

```go
// Core: pure, no I/O, no clocks. Trivially testable.
func plan(state State, cmd Command) (State, []Effect, error) {
    if state.Terminal() {
        return State{}, nil, fmt.Errorf("%w: %q", ErrTerminal, cmd)
    }
    // ... decision ...
}

// Shell: owns I/O, wiring, goroutines, cancellation.
func run(ctx context.Context, deps Deps) error {
    // read input → call core → execute effects → loop
}
```

Rules of thumb:

- Core never imports `os`, `net`, `time` (except types), `math/rand`
  without injection, `log`, or any I/O package. (Enforcing this mechanically
  via a custom rule is discussed in docs/analysis.md — the template does
  not ship one.)
- The shell contains no business logic: no branching on domain state except
  to map errors.
- Inject time, randomness, and UUIDs. `time.Now` in core code is a bug.
- Explicit state transitions: model lifecycles as value + pure transition
  functions returning `(newValue, error)`. Reference: `internal/taskstate`.

### Data

- Values over pointers unless a shared, owning reference is required.
- Structs are copied on output; do not retain input references in output.
- Zero values need not be valid, but must be documented; if they are valid,
  make that deliberate.
- No package-level mutable state. No `init()`. Constructor injection.

### Errors

- Sentinels (`var ErrX = errors.New(...)`) for expected, matchable failures.
- Typed errors (`TransitionError`) carrying structured detail; `Unwrap()` to
  a sentinel so `errors.Is` works.
- Wrap at boundaries with context: `fmt.Errorf("open config %q: %w", path, err)`.
- Never match on error strings. Never swallow errors silently.

### Context and goroutines

- `ctx context.Context` first parameter, propagated everywhere.
- Every goroutine: one owner, a stop signal (context cancel or done channel),
  and a join on shutdown (`sync.WaitGroup` or `errgroup`). Leak a goroutine
  and the race detector will not notice — your shutdown path must.
- Select on `ctx.Done()` in loops; check `ctx.Err()` before blocking work.

### Interfaces

- Define interfaces where they are consumed, not where they are
  implemented. Keep them small. Introduce them at the second consumer or
  the first test fake — not preemptively.

## Package layout

```
cmd/<name>/main.go   # one package main per binary; thin shells only
internal/<pkg>/      # everything else; not importable by other modules
internal/treewalk/   # shared directory walker for maintenance tooling (scripts/smoke)
scripts/init         # module-path rewriter: go run ./scripts/init <new-path>
scripts/smoke        # end-to-end check: copy → rename → make verify
testdata/            # golden files and fuzz corpus seeds (per package)
docs/decisions/      # ADRs
```

No `pkg/` by default. Creating one is an explicit decision (public API you
commit to) — record it as an ADR. Do not create `util`, `common`, `helpers`
packages; find the domain name or the package is misdesigned.

## Initializing a new project from this template

Checklist for humans and agents. "Module path" assumes a GitHub-hosted
module; adapt for other forges.

1. **Create the repository** from this template
   (GitHub → "Use this template", or `git clone` + remove `.git` + `git init`).
2. **Rename the module** with the included initializer. It is a small Go
   program (Go is guaranteed to be present), works identically on Linux,
   macOS and Windows, and is self-contained — it can rewrite the module
   even when it does not yet build:
   ```sh
   go run ./scripts/init github.com/you/yourproject
   ```
   What it does:
   - refuses to run unless `go.mod` declares the template module path,
     so it cannot half-run on the wrong directory;
   - validates the new module path (host-like first element, valid
     elements) before touching anything;
   - replaces every occurrence of the template path — and the
     regex-escaped form — in every text file (binary files and `.git` are
     skipped; `scripts/init` itself is the single exception, by design);
   - prints every file it changed and any remaining references, so you can
     eyeball the result.

   Manual alternatives (if you cannot run the script):
   - Linux (GNU sed):
     ```sh
     grep -rl --exclude-dir=.git 'github\.com/kazzarahw/go-blueprint' . \
       | xargs sed -i 's|github.com/kazzarahw/go-blueprint|github.com/you/yourproject|g'
     ```
   - macOS (BSD sed): same, with `sed -i ''`.
   - Windows PowerShell:
     ```powershell
     Get-ChildItem -Recurse -File -Force |
       Where-Object { $_.FullName -notmatch '\\\.git\\' } |
       ForEach-Object {
         $c = [System.IO.File]::ReadAllText($_.FullName)
         [System.IO.File]::WriteAllText($_.FullName,
           $c.Replace('github.com/kazzarahw/go-blueprint', 'github.com/you/yourproject'))
       }
     ```
   - Plus `go mod edit -module github.com/you/yourproject` either way.
3. **Verify nothing was missed**:
   ```sh
   grep -rn 'kazzarahw' --exclude-dir=.git .
   # Expected: only scripts/init/main.go (the template path is data there,
   # by design). Bare "go-blueprint" hits (template name in README,
   # Makefile header, .golangci.yml) are intentional.
   make verify
   ```
4. **(Recommended) prove the initializer worked for your fork**:
   ```sh
   make smoke   # copies the tree, renames it, and runs make verify there
   ```
   CI runs this on every push, so your fork inherits it automatically.
5. **Make it yours**:
   - Update the copyright holder in `LICENSE`.
   - Replace or delete `internal/taskstate`. If you delete it, do not
     re-add an example later — you have your own code now. If you keep it,
     it is a working example of the conventions; it is safe to leave in place.
   - Trim or edit the seed ADRs (`docs/decisions/0001-*.md` … `0004-*.md`)
     if they describe decisions your project does not share; keep the ADR format.
   - Update the README badge URL to your repository.
   - Replace this checklist section in your local copy with your own
     project notes (or leave it — it is accurate for downstream forks).
6. **First commit**: run `make verify` and commit the rename as one change.
   Do not mix the rename with feature work.
7. **CI**: the workflow in `.github/workflows/ci.yml` needs no changes if
   you use GitHub Actions and Go from `go.mod`. It pins actions by commit
   SHA — keep it that way, and bump SHAs deliberately.

## Adding dependencies

1. Prefer stdlib. If you reach for a package, write the justification first
   (see AGENTS.md §4): what stdlib lacks, maintenance activity, license,
   transitive weight, security record.
2. `go get module@version` — always an explicit version; never `@latest`
   in a committed change.
3. `go mod tidy`, then `make verify`. Commit `go.mod` and `go.sum` together.
4. A dependency with broad needs (e.g. a web framework) warrants an ADR.

## Architecture decision records

Decisions that outlive a diff are written down in `docs/decisions/`.
See [docs/decisions/README.md](docs/decisions/README.md) for the process and
the existing records (functional core / imperative shell; go.mod tool
pinning; NilAway stance). Write a new ADR when you choose between two
reasonable alternatives, or reverse/supersede an existing ADR (mark the old
one `Superseded by NNNN`, never delete it silently).

## Continuous integration

`.github/workflows/ci.yml` runs on every push and PR:

1. `make verify` — the full gate (same command as local).
2. `make fuzz FUZZTIME=30s` — fuzz smoke test on every fuzz target.
3. `make vuln` — govulncheck against the live vulnerability database.
4. `make smoke` — end-to-end: copy the template to a temp dir, rename the
   module path with `scripts/init`, and run `make verify` in the result.
   This is the template's own regression test for the initializer.

Actions are pinned by commit SHA with the tag in a comment. `go` is read
from `go.mod` via `actions/setup-go` with module caching on. The workflow
holds only `contents: read` permission. Branch protection is a repository
setting, not a file — configure it to require this workflow.

## Upgrading Go

1. `go version` in CI comes from `go.mod`. Edit the `go` directive, commit
   as its own change, watch CI.
2. Re-check tool compatibility: `make verify` after the bump. golangci-lint
   in particular tracks Go releases; if a new Go breaks an old golangci-lint,
   bump it too (see "How tool pinning works").

## Editor setup

- **VS Code**: `.vscode/settings.json` ships gopls with staticcheck, shadow
  analysis, and the correct local import prefix; `.vscode/extensions.json`
  recommends the official Go extension. Format on save + organize imports.
- **Any editor**: `.editorconfig` enforces UTF-8, LF, tabs for Go, trailing
  newline, and no trailing whitespace. gofmt is the arbiter — if your
  editor disagrees with `gofmt -l`, the editor is wrong.

## Troubleshooting

- **`go tool` says "no such tool"** → your working directory is not the
  module root, or `go.mod` lost the `tool` directives (check `git diff`).
- **`make verify` fails in `tidy-check`** → run `make tidy`, review the
  diff, commit the result. Never silence it.
- **NilAway reports a possible nil dereference** → it is usually right.
  Introduce a guard, reduce the nil flow, or return an error earlier. Do
  not alias the offending pointer just to hide the flow.
- **Race detector fires** → you have a real data race. Fix ownership or
  synchronization; never add a lock just to make the test pass without
  understanding who owns the value.
- **golangci-lint version mismatch with latest** → that is intentional.
  Tools bump deliberately (see above). Do not bump to "fix" an unrelated
  failure; investigate instead.
