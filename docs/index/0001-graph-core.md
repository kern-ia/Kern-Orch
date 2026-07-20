---
id: okf-0001
feature: graph-core
branch: feature/graph-core
status: done
files:
  - internal/graph/state.go
  - internal/graph/node.go
tests:
  - internal/graph/state_test.go
  - internal/graph/node_test.go
decisions:
  - "2026-07-20 : State = map[string]any + Step, JSON-serializable pour les checkpoints"
  - "2026-07-20 : Clone (copie map top-level) pour isoler une branche fan-out ; Merge remonte le résultat"
  - "2026-07-20 : Node.Execute mute le state et ne choisit JAMAIS le nœud suivant (routing = moteur)"
  - "2026-07-20 : AgentRunner port défini dans graph (dependency inversion) ; impl stub/subprocess en feature #4"
---

**Quoi** : Cœur du moteur. `State` partagé mutable et sérialisable (Get/Set/Clone/Merge,
JSON). `Node` interface (ID/Kind/Execute) avec `ToolNode` (func Go directe) et `AgentNode`
(délègue à un `AgentRunner`, merge la sortie dans le state). Exécution isolée du routage.

**Pièges** : Clone est peu profond (valeurs par référence) — un node ne doit pas muter en
place une valeur référence partagée entre branches fan-out. Documenté dans state.go.
