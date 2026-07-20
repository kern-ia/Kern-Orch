# Harnais Agentique — CDC v2

## 1. Objectif

Construire un harnais agentique en Go : un orchestrateur qui gère un **graphe explicite d'agents et sous-agents** (état partagé, nodes, edges — type LangGraph), s'appuyant sur la bibliothèque de skills existante utilisée à la fois comme **tools** (fonctions invoquées par un agent) et comme **agents autonomes** (skills qui constituent eux-mêmes un nœud du graphe).

Ce n'est pas un simple runner séquentiel de scripts — c'est la brique d'orchestration d'un système agentique complet : gestion du cycle de vie de chaque instance d'agent/sous-agent, routage entre eux, état partagé mutable au fil du graphe.

## 2. Positionnement dans le système global

- **Brique d'exécution LLM (existante/séparée)** : un composant CLI multi-provider avec streaming, inspiré de Pi (pi-coding-agent), qui gère l'appel LLM effectif. **L'orchestrateur ne parle pas aux LLM directement** — il invoque cette brique en subprocess pour chaque nœud de type "agent".
- **Le harnais (ce projet)** : possède le graphe, le state, le routing, le registry de skills, et orchestre les appels vers la brique CLI externe + l'exécution directe des skills-tools.

```
Harnais (Go) ─┬─ Graph Engine (state, nodes, edges, routing)
              ├─ Skills Registry (tools vs agents)
              ├─ Agent Runner ── subprocess ──> Brique CLI multi-provider (streaming, inspirée Pi)
              └─ Checkpoint Store (SQLite)
```

## 3. Concepts clés (portage du modèle LangGraph en Go)

| Concept LangGraph | Équivalent ici |
|---|---|
| **State** | Objet partagé (struct Go ou map structuré) muté à chaque nœud, passé le long du graphe |
| **Node** | Soit un **skill-tool** (exécution directe, pas de LLM, fonction pure ou script) soit un **skill-agent** (invoque la brique CLI multi-provider en subprocess avec le state en contexte) |
| **Edge** | Transition entre nodes — statique ou **conditionnelle** (fonction de routage évaluée sur le state/output du nœud précédent) |
| **Sous-graphe / sous-agent** | Un node peut lui-même déclencher un graphe imbriqué (sous-agent = run enfant avec son propre state, remonte un résultat au state parent) |
| **Checkpoint** | Persistance du state à chaque étape (SQLite) — reprise après échec, debug, audit du run |

## 4. Stack technique

- **Langage** : Go (1.22+)
- **CLI framework** : Cobra
- **Persistance / checkpoints** : SQLite (`modernc.org/sqlite`)
- **Exécution subprocess** : gestion stream stdout/stderr en continu (io.Pipe / bufio.Scanner) pour capter le streaming de la brique CLI multi-provider
- **Concurrency** : goroutines pour nodes parallélisables (edges qui fan-out vers plusieurs nodes indépendants)
- **Définition de graphe** : YAML ou JSON déclaratif (nodes, edges, conditions) — à trancher (cf. section 6)

## 5. Architecture cible (repo)

```
harness/
├── main.go
├── internal/
│   ├── cmd/            # Cobra (run <graph>, resume <run-id>, status, list-skills)
│   ├── graph/          # Moteur d'exécution : Node, Edge, State, routing
│   │   ├── engine.go
│   │   ├── node.go
│   │   ├── edge.go
│   │   └── state.go
│   ├── skills/         # Registry — distinction tool vs agent (metadata SKILL.md)
│   │   ├── manager.go
│   │   └── loader.go
│   ├── agentrunner/    # Wrapper subprocess vers la brique CLI multi-provider
│   │   └── runner.go   # spawn, stream parsing, timeout, retry
│   ├── checkpoint/     # Persistance SQLite du state à chaque étape
│   └── config/         # Config hiérarchique (graphes, skills, providers)
└── schema.json
```

## 6. Points ouverts — à trancher avant de coder (pas d'hypothèse prise ici)

1. **Format de déclaration du graphe** : YAML statique (nodes/edges déclarés à l'avance) vs génération dynamique par du code Go (comme LangGraph en Python où le graphe est construit programmatiquement) ?
2. **Fonction de routage des edges conditionnelles** : évaluée en Go pur (le harnais décide) ou déléguée à un skill/LLM (un agent peut décider du prochain nœud) ?
3. **Granularité des checkpoints** : à chaque node, ou uniquement aux frontières de sous-graphe ?
4. **Contrat d'interface avec la brique CLI multi-provider** : quel est le protocole d'échange (stdin/stdout JSON streamé ? format des messages ? comment le state est-il sérialisé en entrée et le résultat parsé en sortie) ? C'est le point le plus structurant — sans ce contrat, `agentrunner/` ne peut pas être écrit.
5. **Différenciation skill-tool vs skill-agent** : marquée dans le frontmatter du SKILL.md (`type: tool` / `type: agent`) ou déduite d'autre chose ?

## 7. Inspiration (révisée)

- **LangGraph (Python)** : référence conceptuelle pour le modèle state/node/edge — pas de fork, juste porter le design en Go.
- **charmbracelet/crush (Go, FSL-1.1-MIT)** : toujours utile pour `internal/skills/manager.go` (pattern de registry) et l'event bus (observabilité des runs) — **pas pour le modèle d'agent** (Crush a un coordinator/session simple, pas de graphe).
- **Protocol-Lattice/go-agent** : framework Go avec mémoire graph-aware et orchestration multi-agent — plus proche du besoin réel que Crush sur la partie graphe/multi-agent, à inspecter en complément.

## 8. Instructions pour Claude Code

- Ne pas commencer l'implémentation avant d'avoir répondu au point 6.4 (contrat avec la brique CLI multi-provider) — c'est la dépendance bloquante pour `agentrunner/`.
- Démarrer par `internal/graph/state.go` et `internal/graph/node.go` (le cœur du moteur), en isolant complètement l'exécution des nodes du moteur de routage.
- `internal/skills/` : lire `internal/skills/manager.go` de Crush pour inspiration du registry, sans copier — adapter pour supporter le champ `type: tool|agent`.
