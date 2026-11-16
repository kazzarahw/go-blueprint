# Architecture decision records

This directory records decisions that outlive a single diff: choices between
reasonable alternatives, deviations from defaults, and anything a future
agent or maintainer would otherwise have to reverse-engineer from git log.

## Process

1. File: `docs/decisions/NNNN-short-kebab-title.md` with the next sequential
   number. Numbers are never reused.
2. Status: `Proposed` → `Accepted` (or `Rejected`). A decision later found
   wrong is marked `Superseded by NNNN` — never silently deleted.
3. Content: **Context** (forces, constraints), **Decision** (what we do),
   **Consequences** (what this makes easy, hard, or mandatory).
4. Keep it short. One page is plenty; a decision you cannot write in a page
   is two decisions.
5. Reference ADRs from code/docs when they explain a non-obvious choice,
   e.g. "// see docs/decisions/0002-tool-pinning.md".

## Index

| # | Title | Status |
|---|---|---|
| [0001](0001-functional-core-imperative-shell.md) | Functional core / imperative shell as the default architecture | Accepted |
| [0002](0002-tool-pinning-via-go-mod.md) | Pin development tools via `go.mod` tool directives | Accepted |
| [0003](0003-nilaway-stance.md) | NilAway as advisory flow analysis, not a hard proof | Accepted |
