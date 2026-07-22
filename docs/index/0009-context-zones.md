---
id: okf-0009
feature: context-zones
branch: feature/context-zones
status: done
epic: EPIC-01 (kern-orch) — clôture
files:
  - internal/graph/state.go
  - internal/graph/zones.go
  - internal/graph/engine.go
  - internal/cmd/runtime.go
  - examples/freeze.yaml
tests:
  - internal/graph/zones_test.go
decisions:
  - "2026-07-22 : zones de contexte sur le State — clés taguées (défaut persistant, éphémère = scratch) ; Set = persistant, SetZoned = zoné"
  - "2026-07-22 : gel = respawn contexte frais — State.Freeze(carry) garde le carry-over (défaut : persistant), jette le reste, incrémente Frozen, préserve Step"
  - "2026-07-22 : le gel s'expose comme tool builtin `freeze` (modèle hybride) — pas de nouveau type de nœud"
  - "2026-07-22 : moteur — frontière à 1 nœud = REMPLACE le state par la branche (honore suppressions/Frozen) ; fan-out (>1) = Merge additif"
---

**Quoi** : Clôt EPIC-01. Le `State` a des **zones de contexte** (persistant vs éphémère) ; une
opération **gel** (`Freeze`) respawn un **contexte frais** : elle ne garde que le carry-over
(défaut : la zone persistante), jette l'éphémère, incrémente `Frozen`. Exposé comme tool
`freeze` (utilisable en YAML, cf. `examples/freeze.yaml`). Permet à un agent long de garder un
contexte petit (tâches courtes + gel).

**Pièges** : le `Merge` du moteur est **additif** (ne supprime jamais) — un gel sur un clone
puis merge ne propageait ni les suppressions ni `Frozen`. Corrigé : frontière mono-nœud =
**remplacement** par la branche (le clone contient déjà tout le state), fan-out reste additif.
Testé sous `-race`.
