---
id: okf-0006
feature: cmd-run
branch: feature/cmd-run
status: done
files:
  - internal/config/config.go
  - internal/topology/loader.go
  - internal/cmd/runtime.go
  - internal/cmd/commands.go
  - examples/hello.yaml
tests:
  - internal/topology/loader_test.go
  - internal/cmd/run_test.go
decisions:
  - "2026-07-20 : topology YAML → graph.Graph via Registry (hybrid §6.1) : funcs tool/router Go par nom, agent nodes via AgentRunner"
  - "2026-07-20 : run choisit subprocess si KERN_AGENT_CLI sinon Stub ; checkpoint auto par niveau ; runID hex aléatoire"
  - "2026-07-20 : resume = `resume <run-id> <graph.yaml>` (le YAML n'est pas persisté dans le checkpoint — fourni explicitement)"
  - "2026-07-20 : config via env (KERN_SKILLS_DIR/CHECKPOINT_DB/AGENT_CLI) avec défauts"
---

**Quoi** : Câblage CLI complet. `internal/topology` charge un graphe YAML (nodes tool/agent,
edges static/router) en résolvant les noms contre un `Registry`. `internal/config` lit l'env.
`run` charge+exécute+checkpointe, `resume` reprend depuis le dernier checkpoint, `status`
liste les runs. E2E prouvé au binaire (stub + vraie CLI subprocess).

**Pièges** : le chemin du YAML n'est pas stocké dans le checkpoint → resume le redemande.
Les tools sont des funcs Go enregistrées (builtinRegistry) — les projets étendent ce set.
