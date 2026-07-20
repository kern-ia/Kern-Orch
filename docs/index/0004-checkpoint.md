---
id: okf-0004
feature: checkpoint
branch: feature/checkpoint
status: done
files:
  - internal/checkpoint/store.go
  - internal/checkpoint/sqlite.go
  - internal/graph/engine.go
tests:
  - internal/checkpoint/store_test.go
  - internal/checkpoint/resume_test.go
decisions:
  - "2026-07-20 : checkpoint par niveau — Record{run_id, step, frontier, state, status}, PK (run_id, step), upsert idempotent"
  - "2026-07-20 : Engine.OnStep(StepFunc) = seam de persistance ; graph n'importe PAS checkpoint (inversion de dépendance)"
  - "2026-07-20 : resume = Engine.RunFrom(state, frontier) depuis le Latest ; RunFrom généralise Run (entry = frontière initiale)"
  - "2026-07-20 : nombres JSON se décodent en float64 — comparer en conséquence côté state restauré"
---

**Quoi** : Persistance SQLite (`modernc.org/sqlite`, no cgo) de l'état par niveau. `Store`
interface (Save/Latest/List/Close) + `SQLiteStore`. L'Engine expose `OnStep` (hook appelé
après chaque niveau avec state+frontière) et `RunFrom` pour reprendre depuis un checkpoint.
Test crash→resume complet (b échoue puis réussi à la reprise).

**Pièges** : le state JSON-restauré a ses entiers en float64 (json.Unmarshal en any).
Dépendance sqlite ré-ajoutée (go mod tidy l'avait retirée tant qu'aucun import).
