# Static analysis landscape and custom rules

This document records the investigation behind the tools pinned in this
template, the tools considered and rejected for the default path, and how to
add custom `go/analysis` rules when conventions outgrow stock linters.
Version numbers were verified against the Go module proxy on 2026-11-15;
see `go.mod` for the pins actually in force.

## What this template enforces

| Tool | Version | Role | Where |
|---|---|---|---|
| gofmt (+ goimports) | stdlib / x/tools v0.52.0 | canonical formatting, import grouping | `make fmt(-check)`; golangci-lint `formatters` |
| go vet | Go 1.27 | syscall-level sanity (printf, range, copylocks, ...) | `make vet` |
| staticcheck | 2026.2.2 (`honnef.co/go/tools` v0.8.2) | SA bugs, S simplifications, ST style, QF quick-fix, U unused | `make staticcheck`, inside golangci-lint |
| golangci-lint | v2.15.0 | aggregator: errcheck, errorlint, errname, govet+shadow, gosec, gocritic, gocognit(≤15), revive, usestdlibvars, perfsprint, fatcontext, containedctx, contextcheck, forcetypeassert, wastedassign, copyloopvar, misspell, unconvert, unparam, ineffassign, unused, gochecknoinits, nolintlint | `make lint` |
| exhaustive | v0.17.1 | switches over enum types must be exhaustive | `make exhaustive`, and via golangci-lint `exhaustive` |
| NilAway | `v0.0.0-20261112030514-ccd005690d1e` | static nil-flow analysis (best-effort, soundly biased) | `make nilaway` |
| govulncheck | v1.9.0 | known-vulnerability scan of *reachable* code | `make vuln`, CI |
| race detector | stdlib | data races at test time | `make test` (`-race`) |
| native fuzzing | stdlib | adversarial/invariant testing | `make fuzz`, CI smoke |

NilAway publishes **no tagged releases**; pinning a pseudo-version of a
reviewed commit is the only available pinning. It is Apache-2.0 and actively
maintained by Uber (≈3.9k stars). It is a best-effort analysis, not a proof:
it has false positives around generics, reflection, and complex interface
dispatch. Treat reports as bugs to fix, not noise to suppress.

## Recommended additions for real projects (not enabled by default)

- **depguard** (`github.com/OpenPeeDeeP/depguard`) — forbid imports. Two
  policies this template's audience usually wants; add to `.golangci.yml`
  under `linters.settings.depguard` in your project:
  ```yaml
  depguard:
    rules:
      main:
        deny:
          - pkg: "log/slog"
            desc: "stdlib log is allowed; choose one logging approach project-wide"
      no-unsafe-time:
        files:
          - "!internal/clock/**"
          - "!internal/**/clock.go"
        deny:
          - pkg: "time"
            desc: "use the injected clock (internal/clock), not wall time"
  ```
  (Adapt paths to your layout. The point: *policy as config*, reviewable in
  diffs, not tribal knowledge.)
- **forbidigo** — regex-level API bans, e.g. forbid `fmt.Println` in
  non-`cmd/` code (stdout prints are a shell concern), forbid `os.Exit`
  outside `cmd/`.
- **gomoddirectives** — forbid `replace` directives in `go.mod` outside
  `tools/` (prevents stealth forks), forbid `exclude` without a comment.
- **gocritic** additional checkers — already enabled with defaults; add
  `performance` and `style` tags selectively as you learn their noise ratio
  in your codebase.
- **gosec** is enabled with its default ruleset. Projects handling secrets
  should also enable `gosec.conf`-level rules G101/G304 review or plug a
  dedicated scanner; template stays minimal.
- **go test coverage enforcement** — deliberately not enabled; see
  TESTING.md §Coverage.

## Considered and rejected (for the default template)

| Tool | Why not by default |
|---|---|
| `gochecknoglobals` | Indistinguishable from sentinel errors; noisy without benefit once AGENTS.md §1 is in force. Enable it in your project if you want the friction. |
| `err113` | Static-errors-only rule; fights legitimate dynamic wrapped errors with little defect yield. `errorlint` covers wrapping correctness. |
| `exhaustruct` | Forbids partial struct literals — hostile to immutable-by-convention value construction. |
| `funlen`, `lll`, `nlreturn`, `wsl`, `mnd`, `goconst` | Style/metric dogma; `gocognit` (≤15) already bounds complexity. High noise, low signal. |
| `godox` | Forbids TODOs; this template bans unowned TODOs by convention instead (AGENTS.md §6). |
| `tenv`/`usetesting`/`thelper` | Enable when your tests actually use env/TB helpers in non-trivial ways. |
| `wrapcheck`/`goerr113` clones | Forced wrapping everywhere; fights %w discipline via errorlint instead. |
| Linter exclusion presets (`common-false-positives`, `legacy`, `std-error-handling`, `comments`) | Hiding findings wholesale is the failure mode this repository exists to avoid. Stay preset-free; fix code or change config deliberately. |

## Custom `go/analysis` rules

Stock linters cover 90% of useful mechanical conventions. Custom rules are
worth writing when a convention is (a) important, (b) checkable via AST/type
information, and (c) repeated often enough that review misses it. Good
candidates in this style of project:

1. **No `time.Now` / `time.Since` outside designated packages** — enforces
   clock injection without any runtime cost.
2. **No `fmt.Print*` / `log.Print*` outside `cmd/` or `internal/log`** —
   enforces shell-only I/O.
3. **No `os.Getenv` outside `internal/config`** — enforces a single,
   validated configuration boundary.
4. **No goroutines in `internal/<core packages>`** — mechanically separates
   functional core from imperative shell.
5. **No functions returning `error` that ignore it at call sites** (errcheck
   already does this; write your own only for domain-specific sinks).

### Minimal skeleton

A standalone analyzer is a `singlechecker` main (module `golang.org/x/tools`
is already an indirect dependency; adding it directly is one `go get`):

```go
// tools/noclock/main.go
package main

import (
    "golang.org/x/tools/go/analysis"
    "golang.org/x/tools/go/analysis/singlechecker"
    "golang.org/x/tools/go/ast/inspector"
    "go/ast"
)

var flagPkgs = ... // e.g. -core=internal/taskstate,...

var Analyzer = &analysis.Analyzer{
    Name: "noclock",
    Doc:  "disallow time.Now and time.Since in functional-core packages",
    Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
    insp := inspector.New(pass.Files)
    insp.Preorder([]ast.Node{(*ast.SelectorExpr)(nil)}, func(n ast.Node) {
        sel, ok := n.(*ast.SelectorExpr)
        if !ok { return }
        // ... check sel.Sel.Name ∈ {"Now","Since"} with sel.X ident "time" ...
        pass.Reportf(sel.Pos(), "use an injected clock instead of time.%s", sel.Sel.Name)
    })
    return nil, nil
}

func main() { singlechecker.Main(Analyzer) }
```

Wire it as a `tool` directive in a dedicated `tools.go`-free module or in
the main module, and call `go tool noclock ./...` from `make verify`.

### Or: custom linters inside golangci-lint

golangci-lint v2 supports plugins: compile the analyzer as a module plugin
(`buildmode=plugin` via `go.mod` `replace` + `linters.custom` in
`.golangci.yml`), or as a [module plugin](https://golangci-lint.run/plugins/module-plugins/).
This is heavier than a singlechecker binary; prefer the binary unless you
want all findings in one golangci-lint report.

### Decision recorded in this template: none shipped

Custom analyzers are **not** shipped here because:

- they would add `golang.org/x/tools` as a direct dependency of every
  downstream project (module-graph weight, bump churn),
- every rule this template cares about deeply (format, static analysis,
  lint, nil-flow, races, vulns, exhaustive switches) is already enforced by
  pinned stock tools,
- a custom rule that is wrong is worse than a convention written down:
  false positives teach agents to fight the checker.

If you adopt one, do it as its own module under `tools/<name>/` with its own
`go.mod`, and invoke it from CI explicitly. Record the decision in an ADR.

## Layered defense summary

```
editor (gopls + staticcheck + shadow)     ← immediate, per keystroke
make verify (tidy,fmt,vet,sa,lint,nil)     ← per change, local
CI (verify + fuzz smoke + govulncheck)     ← per push/PR, authoritative
review (agents + humans, AGENTS.md rules)  ← judgement for what analysis can't see
```

No layer is optional; each covers the failure modes of the one above it.
