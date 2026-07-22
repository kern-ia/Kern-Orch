---
id: okf-0008
feature: resume-graphpath
branch: feature/checkpoint-graphpath
status: done
epic: EPIC-01 (kern-orch)
files:
  - internal/checkpoint/store.go
  - internal/checkpoint/sqlite.go
  - internal/cmd/commands.go
  - internal/cmd/runtime.go
tests:
  - internal/checkpoint/graphpath_test.go
  - internal/cmd/resume_test.go
decisions:
  - "2026-07-22 : le chemin (absolu) du graphe est persisté par checkpoint (colonne graph_path, DEFAULT '')"
  - "2026-07-22 : resume <run-id> [graph.yaml] — l'argument graphe devient optionnel ; défaut = chemin enregistré ; un arg explicite l'emporte"
  - "2026-07-22 : run stocke filepath.Abs(args[0]) pour que resume marche depuis n'importe quel cwd"
---

**Quoi** : Ferme le reste « S » d'EPIC-01. Le `run` enregistre le chemin du graphe dans chaque
checkpoint ; `resume <run-id>` recharge ce chemin tout seul (plus besoin de re-passer le YAML).
Un chemin explicite reste accepté et prioritaire. Message d'aide au crash simplifié en
`resume <run-id>`.

**Pièges** : colonne ajoutée avec `DEFAULT ''` pour que les vieux enregistrements/DB ne cassent
pas le scan. Chemin stocké en absolu (résolu au `run`) sinon `resume` depuis un autre cwd échoue.
