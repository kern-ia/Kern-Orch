---
id: okf-0011
feature: topology-and-failure
branch: feature/topology-contract
status: done
files:
  - internal/topology/describe.go
  - internal/report/http.go
  - internal/cmd/runtime.go
  - internal/cmd/commands.go
tests:
  - internal/topology/describe_test.go
  - internal/report/v2_test.go
decisions:
  - "2026-07-26 : la topologie vient du YAML (topology.Describe), pas du graphe runtime — les arêtes de graph.Graph sont des RouteFunc, une route conditionnelle est inénumérable"
  - "2026-07-26 : une arête pilotée par router part sans cible avec dynamic:true — un consommateur doit savoir que l'image est incomplète, pas lire le nœud comme terminal"
  - "2026-07-26 : la topologie n'est envoyée qu'au premier événement du run ; elle ne change pas"
  - "2026-07-26 : ReportFailure est un appel séparé — le hook ne voit que les niveaux réussis, le moteur signale l'échec en retournant de Run"
  - "2026-07-26 : l'échec transporte la frontière qui tournait au moment de la casse, sinon le sink sait que ça a cassé mais pas où"
  - "2026-07-26 : ReportFailure utilise context.WithoutCancel — le contexte du run est déjà annulé à ce moment, sinon aucun échec ne serait jamais rapporté"
---

**Quoi** : kern-orch publie la forme de son graphe et ses échecs. `topology.Describe` lit le
YAML déclaré ; le reporter l'émet une fois par run. `ReportFailure` clôt un run cassé en
nommant le message et la frontière active.

**Pièges** :
- `graph.Graph` ne peut pas rendre ses arêtes : ce sont des closures. Seule la déclaration
  YAML les connaît, et encore, pas les cibles d'un router.
- Le contexte du run est annulé quand `Run` retourne une erreur : sans
  `context.WithoutCancel`, le rapport d'échec partait déjà mort.
