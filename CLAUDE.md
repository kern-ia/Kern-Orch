# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status

Bootstrapped (Phase 2 done, `feature/bootstrap`). The harness is a **Go 1.26** module (`github.com/yoann/kern-orch`) using **Cobra** (CLI) and **`modernc.org/sqlite`** (pure-Go, no cgo). The Cobra command tree exists with stubbed subcommands (`run`/`resume`/`status`/`list-skills` return `not implemented yet`); the graph engine, agentrunner, checkpoint store, and skills registry are not built yet. The design spec `harnais-agentique-CDC-v2.md` is in French; keep design discussion consistent with it.

## Commands

- Build: `go build ./...`
- Vet: `go vet ./...`
- Test all: `go test ./...`
- Single package: `go test ./internal/cmd/`
- Single test: `go test ./internal/cmd/ -run TestRootHasExpectedSubcommands -v`
- Run CLI: `go run . <command>` (e.g. `go run . --help`, `go run . list-skills`)

## Workflow (greenfield-tdd-okf skill)

Git-flow: `main` (stable jalons) ← `dev` ← one branch per feature. **Merge `--no-ff` to `dev` only when tests are green + build passes + E2E done. Never commit directly to `main`/`dev`.** Each feature: write tests first on pure logic in the engine/services, then wire thin orchestration; write an OKF fiche in `docs/index/<n>-<feature>.md` (template in `.claude/skills/greenfield-tdd-okf/references/okf-fiche-template.md`, body ≤15 lines, dated decisions); log pitfalls in `retro.md` when they bite. Generic pitfalls also go back into the skill's `references/pieges.md`.

## What is being built

An **agentic harness** in Go: an orchestrator that runs an explicit **graph of agents and sub-agents** (LangGraph-style: shared mutable state, nodes, edges, conditional routing). It is not a sequential script runner — it owns agent/sub-agent lifecycle, routing, and state.

Critical architectural boundary: **the harness never calls LLMs directly.** LLM execution lives in a *separate, existing* multi-provider streaming CLI (Pi-inspired). The harness invokes that CLI **as a subprocess** for every "agent" node, streaming its stdout/stderr. This subprocess contract is the load-bearing seam of the whole system.

### Core model (LangGraph ported to Go)

- **State** — a shared Go struct/map mutated at each node and threaded along the graph.
- **Node** — either a **skill-tool** (direct execution, no LLM) or a **skill-agent** (spawns the external CLI subprocess with state as context).
- **Edge** — static or **conditional** (a routing function evaluated on prior node state/output).
- **Sub-graph / sub-agent** — a node may trigger a nested graph (child run with its own state, result bubbles back to the parent state).
- **Checkpoint** — state persisted to SQLite at each step for resume-after-failure, debug, and audit.

The same skills library is used two ways: as **tools** (functions invoked by an agent) and as **agents** (a skill that is itself a graph node). Tool-vs-agent is intended to be marked in the skill's `SKILL.md` frontmatter (`type: tool | agent`).

### Planned layout (`harness/`)

`internal/graph/` (engine: `state.go`, `node.go`, `edge.go`, `engine.go`) · `internal/skills/` (registry: `manager.go`, `loader.go`) · `internal/agentrunner/` (subprocess wrapper to the external CLI: spawn, stream parsing, timeout, retry) · `internal/checkpoint/` (SQLite) · `internal/cmd/` (Cobra: `run <graph>`, `resume <run-id>`, `status`, `list-skills`) · `internal/config/`.

## Working guidance from the spec

- **Do not start implementation before the subprocess contract (spec §6.4) is decided.** How state is serialized into the external CLI and how its streamed output is parsed back is the blocking dependency for `internal/agentrunner/` — everything else can proceed, this cannot.
- Start from `internal/graph/state.go` and `internal/graph/node.go` (the engine core). **Keep node execution fully isolated from the routing engine.**
- Several design choices are deliberately still open (spec §6) and should not be silently assumed: graph declaration format (static YAML/JSON vs. programmatic Go construction), whether conditional routing is decided in Go or delegated to an LLM/skill, checkpoint granularity (per-node vs. sub-graph boundaries), and how tool-vs-agent is determined. Surface these rather than picking one unprompted.

## Engineering constraints

- **SOLID.** Respect the five principles throughout — especially single responsibility (keep node execution, routing, skill registry, subprocess handling, and persistence in separate units) and dependency inversion (depend on interfaces, e.g. the `agentrunner` subprocess boundary and the `checkpoint` store, not concrete types). This mirrors the spec's requirement to isolate node execution from the routing engine.
- **DRY.** Do not duplicate logic. Factor shared behaviour (state serialization, stream parsing, error/retry handling) into reusable functions rather than copy-pasting across nodes, skills, or commands.
- **Never delete without consent.** Do not remove files, functions, code, or data without explicit user approval. If something looks obsolete, propose the deletion and wait for confirmation before acting.

## Reference points (for design, not to fork)

- **LangGraph (Python)** — conceptual reference for the state/node/edge model.
- **charmbracelet/crush (Go)** — inspiration for `internal/skills/manager.go` (registry pattern) and its event bus (run observability); *not* its agent model (Crush has a simple coordinator/session, no graph).
- **Protocol-Lattice/go-agent** — Go multi-agent framework with graph-aware memory; closer to the graph/multi-agent need than Crush.
