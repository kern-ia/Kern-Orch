---
id: okf-0000
feature: bootstrap
branch: feature/bootstrap
status: done
files:
  - go.mod
  - main.go
  - internal/cmd/root.go
  - internal/cmd/commands.go
  - .gitignore
  - .env.example
tests:
  - internal/cmd/root_test.go
decisions:
  - "2026-07-20 : module github.com/yoann/kern-orch, Go 1.26 (Cobra + modernc.org/sqlite, no cgo)"
  - "2026-07-20 : git-flow main/dev/feature, merge --no-ff to dev only when green + E2E"
  - "2026-07-20 : agent CLI absent → AgentRunner interface + stub, wired via KERN_AGENT_CLI (spec §6.4)"
  - "2026-07-20 : hybrid graph (YAML topology + Go funcs by name), Go-pure routing, frontmatter type (spec §6)"
---

**Quoi** : Squelette du harnais — CLI Cobra (`run`/`resume`/`status`/`list-skills` en stubs
`not implemented yet`), module Go + deps, arborescence `internal/`, docs OKF et rétro.
Base verte compilable, aucune logique métier encore.

**Pièges** : brique CLI LLM externe non accessible → isolée derrière une interface (voir retro.md).
