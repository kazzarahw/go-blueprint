# cmd/

One directory per binary, each a thin `package main` shell around code in
`internal/` (see DEVELOPMENT.md and AGENTS.md).

This directory is intentionally empty in the template: the blueprint ships
no application. When you add your first binary, create e.g.
`cmd/worker/main.go` and keep it boring — flags/env in, wire
dependencies, run, exit with explicit status.
