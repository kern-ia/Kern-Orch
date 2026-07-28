---
id: 0016
feature: nested-runs
branch: feature/nested-runs
status: done
files:
  - internal/graph/subgraph.go
  - internal/topology/loader.go
  - internal/topology/loadfile.go
  - internal/report/http.go
  - internal/cmd/runtime.go
  - internal/cmd/commands.go
  - contracts/kern.step-event.v2.nested.json
tests:
  - internal/graph/subgraph_test.go
  - internal/report/v2_test.go
  - internal/cmd/publish_skills_test.go
decisions:
  - "2026-07-28 : l'enfant rapporte comme un RUN À PART avec `parent`, pas replié dans le flux du parent — le compteur de niveaux du parent est une séquence qu'un sink protège contre les écritures désordonnées"
  - "2026-07-28 : le nœud passe SA PROPRE `graphRef` au constructeur du hook — à la profondeur 2 le nœud n'est pas dans le graphe que l'appelant détient, et la recherche ne trouverait rien en silence"
  - "2026-07-28 : un id de run neuf à chaque exécution — un nœud rejoué est deux runs imbriqués, pas un run rapporté deux fois"
  - "2026-07-28 : rapport opt-in (`WithChildStep`) — un graphe assemblé en Go sans observabilité tourne exactement comme avant"
  - "2026-07-28 : reporter et runID créés AVANT `LoadFile` — un nœud sous-graphe reçoit son hook à la construction"
---

**Quoi** : un sous-graphe rapporte son propre run. Le parent continuait de le voir comme un
seul pas atomique, ce qui reste vrai pour son checkpoint — mais l'enfant existe maintenant
pour qui regarde.

**Piège** : `builtinRegistry` était consommé par `LoadFile` avant que le reporter n'existe.
Il a fallu remonter la création du run id et du reporter, sans quoi l'option arrivait trop
tard pour les nœuds déjà construits.
