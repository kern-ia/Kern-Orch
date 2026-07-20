---
id: okf-0007
feature: subgraph
branch: feature/subgraph
status: done
files:
  - internal/graph/subgraph.go
  - internal/graph/node.go
  - internal/topology/loader.go
  - internal/topology/loadfile.go
  - internal/cmd/commands.go
  - internal/cmd/runtime.go
  - examples/parent.yaml
  - examples/child.yaml
tests:
  - internal/graph/subgraph_test.go
  - internal/topology/loadfile_test.go
decisions:
  - "2026-07-20 : SubgraphNode = sous-agent (§3) — child state seedé du parent (Clone par défaut), résultat mergé au retour ; options WithInput/WithOutput"
  - "2026-07-20 : granularité checkpoint = frontière de sous-graphe (§6.3) : le sous-run est UN step atomique côté parent"
  - "2026-07-20 : YAML `type: subgraph` + `graph: <fichier>` ; topology.LoadFile résout les fichiers (relatifs au parent) avec garde anti-récursion"
  - "2026-07-20 : Load([]byte) refuse les subgraphs (pas de résolution fichier) → run/resume utilisent LoadFile"
---

**Quoi** : Nœuds sous-graphe / sous-agent. `SubgraphNode` exécute un graphe imbriqué avec son
propre state (seedé depuis le parent, résultat remonté). Mappers `WithInput`/`WithOutput`
personnalisables. Côté YAML, `type: subgraph` référence un fichier ; `topology.LoadFile` le
charge récursivement avec détection de cycle. E2E prouvé au binaire (parent.yaml → child.yaml).

**Pièges** : récursion de fichiers subgraph → garde via un set "in-progress" (supprimé au retour,
donc un même fichier réutilisé par deux frères reste OK). Le sous-run n'a pas de hook checkpoint
propre (atomique côté parent) — choix §6.3 assumé.
