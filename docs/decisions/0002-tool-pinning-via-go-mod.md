# 0002 — Pin development tools via `go.mod` tool directives

Status: Accepted (2026-11-15)

## Context

The verification pipeline depends on external tools (golangci-lint,
staticcheck, exhaustive, NilAway, govulncheck, goimports). Without
pinning, local runs, CI, and agent runs execute different tool versions,
findings diverge, and "works on my machine" leaks into review. The classic
`tools.go` blank-import pattern works but obscures intent and forces busy
`require` blocks; `go install module@latest` is unpinned by definition.

Go 1.24 added the `tool` directive in `go.mod`: first-class tool dependency
declarations, resolved like any other module requirement and executed via
`go tool <name>`.

## Decision

All developer tools are pinned as `tool` directives in `go.mod` and run
exclusively via `go tool <name>` from the Makefile. Versions are exact
(module version or pseudo-version for untagged tools like NilAway). Bumps
happen in dedicated commits with a `make verify` run. CI reads `go` from
`go.mod` (`actions/setup-go` with `go-version-file`), so the language
version is pinned the same way. GitHub Actions are pinned by commit SHA
with the tag in a trailing comment.

## Consequences

- `go.sum` covers the tools' module graphs (~215 modules). This is accepted:
  it buys bit-for-bit reproducibility across every environment with zero
  external pinning files (`tools.go`, `.tool-versions`, Docker images).
- Bumping a tool is an ordinary, reviewable diff to `go.mod`/`go.sum`.
- A new tool becomes part of the build graph of every downstream project
  that forks this template; keep the tool set lean and justified (AGENTS.md
  §4 applies to tools as much as to libraries).
- If a tool misbehaves under the `tool` mechanism (build constraints,
  plugins), the fallback decision belongs in a new ADR (e.g. moving that one
  tool to a pinned CI download), not in ad-hoc local installs.
