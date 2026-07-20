# Kern-Orch — Architecture & diagrammes de classe

> Harnais agentique (Go 1.26) — v0.2.0. Un orchestrateur qui exécute un graphe explicite
> d'agents et de sous-agents (state partagé, nœuds, edges, checkpoints). Il ne parle jamais
> au LLM directement : il l'invoque en subprocess pour chaque nœud `agent`.

**Stack** : Go 1.26 · Cobra · `modernc.org/sqlite` (no cgo) · `yaml.v3` · 7 packages `internal/`.

**Conventions des diagrammes de classe** : `..|>` réalise (implements) · `*--` possède ·
`..>` utilise/dépend.

**Direction des dépendances** : `graph` définit les *ports* (`AgentRunner`, `StepFunc`) ;
l'infra (`agentrunner`, `checkpoint`) dépend de `graph`, **jamais l'inverse**.

---

## 1. Architecture & flux d'exécution

```mermaid
flowchart TB
  subgraph CLI["internal/cmd — Cobra"]
    RUN["run / resume / status / list-skills"]
  end
  CFG["internal/config<br/>Config (env)"]
  SK["internal/skills<br/>Registry · SKILL.md"]
  TOP["internal/topology<br/>Registry + LoadFile<br/>YAML → Graph"]

  subgraph CORE["internal/graph — moteur (agnostique métier)"]
    ENG["Engine<br/>level-synchronous<br/>fan-out · cycle guard"]
    GR["Graph<br/>nodes · edges · entry"]
    ST["State<br/>map partagé · Clone/Merge"]
    PORT{{"AgentRunner (port)<br/>StepFunc (hook)"}}
  end

  AR["internal/agentrunner<br/>Stub · Subprocess"]
  CP["internal/checkpoint<br/>Store · SQLiteStore"]
  EXT["Brique LLM externe<br/>multi-provider CLI<br/>(repo du collègue)"]
  DB[("SQLite<br/>checkpoints")]

  RUN --> CFG
  RUN --> TOP
  RUN --> SK
  TOP --> GR
  RUN --> ENG
  ENG --> GR
  GR --> ST
  ENG -. hook .-> PORT
  ENG -. appelle .-> PORT
  AR -. réalise .-> PORT
  CP -. réalise .-> PORT
  ENG --> CP
  CP --> DB
  AR == "subprocess JSON-lines" ==> EXT

  classDef core fill:#eef2f9,stroke:#334155,color:#0f172a;
  classDef port fill:#e2e8f0,stroke:#334155,stroke-dasharray:4 3,color:#0f172a;
  classDef infra fill:#e0edff,stroke:#2563eb,color:#0f172a;
  classDef ext fill:#fde7d4,stroke:#b45309,color:#3a2a12;
  classDef cli fill:#ede9fe,stroke:#6d28d9,color:#241a3a;
  class ENG,GR,ST core;
  class PORT port;
  class AR,CP,DB infra;
  class RUN,CFG,SK,TOP cli;
  class EXT ext;
```

- **Le harnais gouverne, le LLM exécute** : le routage est Go-pur ; un nœud `agent` appelle
  la CLI externe et fusionne sa sortie, mais c'est le moteur qui décide la suite.
- **Agnostique** : rien de métier dans `internal/graph`. La valeur métier entre par les
  **skills**, les **tool funcs** (registry Go) et le **graphe YAML**.

---

## 2. Cœur : `State` & `Node` (internal/graph 1/2)

L'unité de travail. `Node` mute le state en place et *ne choisit jamais* le nœud suivant
(le routage est la responsabilité du moteur). Trois implémentations selon `Kind`.

```mermaid
classDiagram
  direction LR

  class Node {
    <<interface>>
    +ID() string
    +Kind() Kind
    +Execute(ctx, State) error
  }
  class Kind {
    <<enumeration>>
    KindTool
    KindAgent
    KindSubgraph
  }
  class State {
    -data map[string]any
    +Step int
    +Get(key) (any, bool)
    +Set(key, value)
    +Clone() State
    +Merge(other)
    +MarshalJSON()
  }
  class ToolNode {
    -id string
    -fn ToolFunc
    +Execute(ctx, State) error
  }
  class AgentNode {
    -id string
    -prompt string
    -runner AgentRunner
    +Execute(ctx, State) error
  }
  class SubgraphNode {
    -id string
    -sub Graph
    -input func
    -output func
    +Execute(ctx, State) error
  }
  class ToolFunc {
    <<func>>
    func(ctx, State) error
  }

  ToolNode ..|> Node
  AgentNode ..|> Node
  SubgraphNode ..|> Node
  Node ..> Kind
  ToolNode *-- ToolFunc
  ToolNode ..> State
  AgentNode ..> State
  SubgraphNode ..> State
```

- `SubgraphNode` = le **sous-agent** (§3) : exécute un `Graph` enfant avec son propre state
  (seedé du parent via `input`, remonté via `output`).
- `State.Clone()` isole une branche de fan-out ; `Merge()` remonte le résultat.

---

## 3. Moteur : `Graph`, `Engine` & routage (internal/graph 2/2)

Le `Graph` possède les nœuds et une `RouteFunc` par nœud (routage Go-pur). L'`Engine`
exécute par niveaux, en parallèle, et expose le hook `StepFunc` — la couture de
persistance, sans que `graph` connaisse `checkpoint`.

```mermaid
classDiagram
  direction LR

  class Graph {
    -nodes map
    -routes map
    -entry string
    +AddNode(n) Graph
    +SetEntry(id) Graph
    +AddEdge(from, RouteFunc) Graph
    +Validate() error
  }
  class Engine {
    -g Graph
    -maxSteps int
    -onStep StepFunc
    +OnStep(f) Engine
    +Run(ctx, State) error
    +RunFrom(ctx, State, frontier) error
  }
  class RouteFunc {
    <<func>>
    func(State) []string
    Static() Conditional() Terminal()
  }
  class StepInfo {
    +Step int
    +Frontier []string
  }
  class StepFunc {
    <<func>>
    func(ctx, StepInfo, State) error
  }
  class AgentRunner {
    <<interface>>
    +Run(ctx, AgentRequest) (AgentResult, error)
  }

  Engine *-- Graph
  Graph *-- RouteFunc
  Engine ..> StepFunc
  StepFunc ..> StepInfo
  Engine ..> AgentRunner
```

- **Ordonnancement level-synchronous** : chaque frontière tourne en parallèle sur des clones
  du state, merge stable, frontière suivante dédupliquée — testé sous `-race`.
- `RunFrom` généralise `Run` (entry = frontière initiale) et sert la reprise depuis un checkpoint.

---

## 4. Runner d'agent : le port vers le LLM (internal/agentrunner)

Deux implémentations du port `graph.AgentRunner` : `Stub` déterministe (l'app tourne sans
aucune config LLM) et `Subprocess` qui parle un protocole *JSON-lines* à la CLI externe
(contrat §6.4 **provisoire**).

```mermaid
classDiagram
  direction LR

  class AgentRunner {
    <<interface>>
    +Run(ctx, AgentRequest) (AgentResult, error)
  }
  class Stub {
    +Responses map
    +Default map
    +Run(...) AgentResult
  }
  class Subprocess {
    +Path string
    +Args []string
    +Env []string
    +Stderr io.Writer
    +TokenSink io.Writer
    +Run(...) AgentResult
    -consume(stdout) AgentResult
  }
  class AgentRequest {
    +NodeID string
    +Prompt string
    +State State
  }
  class AgentResult {
    +Output map[string]any
  }
  class Request {
    +NodeID string
    +Prompt string
    +State json.RawMessage
  }
  class Event {
    +Type string
    +Text string
    +Output map
    +Message string
  }

  Stub ..|> AgentRunner
  Subprocess ..|> AgentRunner
  AgentRunner ..> AgentRequest
  AgentRunner ..> AgentResult
  Subprocess *-- Request
  Subprocess ..> Event
```

- **Contrat provisoire** : 1 `Request` sur stdin, des `Event` `{token · result · error}` par
  ligne sur stdout ; à réconcilier avec la vraie CLI.
- Sélection au runtime : `Subprocess` si `KERN_AGENT_CLI` est défini, sinon `Stub`.

---

## 5. Persistance & reprise (internal/checkpoint)

Le state est persisté par niveau. L'`Engine` appelle `Store.Save` via son hook `StepFunc` ;
`resume` recharge le dernier checkpoint et relance `Engine.RunFrom`.

```mermaid
classDiagram
  direction LR

  class Store {
    <<interface>>
    +Save(ctx, Record) error
    +Latest(ctx, runID) (Record, bool, error)
    +List(ctx) ([]Summary, error)
    +Close() error
  }
  class SQLiteStore {
    -db sql.DB
    +Save(...) error
    +Latest(...) (Record, bool, error)
    +List(...) ([]Summary, error)
  }
  class Record {
    +RunID string
    +Step int
    +Frontier []string
    +State State
    +Status string
    +CreatedAt time
  }
  class Summary {
    +RunID string
    +LastStep int
    +Status string
    +UpdatedAt time
  }
  class StepFunc {
    <<func graph>>
    func(ctx, StepInfo, State) error
  }

  SQLiteStore ..|> Store
  Store ..> Record
  Store ..> Summary
  StepFunc ..> Store
```

- Clé `(run_id, step)`, upsert idempotent ; un `SubgraphNode` est **un step atomique** côté
  parent (granularité §6.3).
- SQLite pur Go (`modernc.org/sqlite`, no cgo).

---

## 6. Câblage : catalogue, chargement, CLI (skills · topology · config · cmd)

Le YAML déclare la topologie ; les fonctions `tool`/`router` sont du Go enregistré par nom
dans `topology.Registry` (modèle hybride §6.1). `skills.Registry` lit le `type` des `SKILL.md`.

```mermaid
classDiagram
  direction LR

  class SkillsRegistry {
    -byName map
    +Get(name) (Skill, bool)
    +List() []Skill
    +Load(dir)
  }
  class Skill {
    +Name string
    +Type Type
    +Description string
    +Dir string
  }
  class TopologyRegistry {
    -tools map
    -routers map
    -runner AgentRunner
    +Tool(name, fn) Registry
    +Router(name, fn) Registry
  }
  class Loader {
    <<package>>
    Load(bytes, reg) Graph
    LoadFile(path, reg) Graph
  }
  class Config {
    +SkillsDir string
    +CheckpointDB string
    +AgentCLI string
    +FromEnv() Config
  }
  class cmd {
    <<package>>
    run resume status list-skills
    newRunner() builtinRegistry()
  }

  SkillsRegistry *-- Skill
  TopologyRegistry ..> AgentRunner
  Loader ..> TopologyRegistry
  Loader ..> Graph
  cmd ..> Config
  cmd ..> Loader
  cmd ..> SkillsRegistry
  cmd ..> Engine
```

- `LoadFile` résout `type: subgraph` (fichiers imbriqués) avec garde anti-récursion ;
  `Load([]byte)` refuse les subgraphs.
- Un `type: tool` dans un SKILL.md reste une **étiquette de catalogue** — l'exécution passe
  encore par une func Go du registry (lien non encore fusionné).

---

## Reste ouvert

Réconciliation du contrat JSON-lines **§6.4** avec la CLI multi-provider du collègue —
isolée derrière le port `AgentRunner`, donc l'impact se limitera à
`internal/agentrunner/{protocol,subprocess}.go`.
