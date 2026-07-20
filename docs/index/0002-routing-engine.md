---
id: okf-0002
feature: routing-engine
branch: feature/routing-engine
status: done
files:
  - internal/graph/edge.go
  - internal/graph/engine.go
tests:
  - internal/graph/engine_test.go
decisions:
  - "2026-07-20 : RouteFunc(state) []string — routing Go-pur, hors des nodes ; []>1 = fan-out, vide = terminal"
  - "2026-07-20 : ordonnancement level-synchronous — frontière exécutée en parallèle sur clones, merge stable (ordre frontière), frontière suivante dédupliquée+triée"
  - "2026-07-20 : cycle guard via maxSteps (défaut 10000 niveaux)"
  - "2026-07-20 : loader YAML de la topologie reporté à la feature cmd/config #6"
---

**Quoi** : Moteur d'exécution. `Graph` (nodes + entry + une RouteFunc par node) avec
`Validate`. `Engine.Run` exécute niveau par niveau : chaque frontière tourne en parallèle
sur des clones du state, résultats mergés de façon déterministe, frontière suivante calculée
depuis les routes. `Static`/`Conditional`/`Terminal` construisent les edges.

**Pièges** : merge fan-out = « dernière branche gagne » sur clé en conflit (ordre frontière) —
déterministe mais les branches ne doivent pas écrire la même clé avec des valeurs divergentes.
Testé sous `-race`.
