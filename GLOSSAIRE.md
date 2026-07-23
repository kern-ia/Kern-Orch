# Kern — Glossaire technique

> Le lexique du système : moteur d'orchestration, briques `kern-*` et concepts
> transverses. Version artifact (mise en page) : lien à part. Dernière mise à jour : 2026-07-23.

**Principe directeur** : *le harnais gouverne, le LLM exécute*. Une seule brique a le LLM
dans son cœur (**kern-scorer**). Tout le reste est du **Go déterministe** ; le LLM n'est
branché qu'aux nœuds **agent**, via **kern-link**.

**Étiquettes** utilisées ci-dessous :
- 🟢 **déterministe · sans LLM**
- 🟣 **LLM** (possible / cœur)
- 🔵 **brique externe / tiers**

---

## 1. Moteur d'exécution (kern-orch)

Le cœur : graphe, state, routage, reprise.

| Terme | Étiq. | Définition |
|---|---|---|
| **State** | 🟢 | Objet **partagé et mutable** threadé le long du graphe, muté à chaque nœud. Map sérialisable (JSON) → persistable par checkpoint. Pas concurrent-safe : en fan-out chaque branche reçoit un `Clone` fusionné ensuite sur une seule goroutine. |
| **Zone de contexte** | 🟢 | Étiquette portée par une clé du state : `persistant` (défaut, survit) ou `éphémère` (scratch, jeté au gel). `Set` = persistant, `SetZoned` = zoné. Garde petit le contexte d'un agent long. |
| **Freeze / gel** | 🟢 | **Respawn d'un contexte frais** : ne garde que le *carry-over* (défaut : zone persistante), jette le reste, incrémente `Frozen`. Exposé comme tool builtin `freeze`. Contient la croissance du contexte sur les runs longs. |
| **Node / nœud** | 🟢 | Unité d'exécution. Trois *kinds* : `tool` (Go pur), `agent` (spawn LLM), `subgraph` (graphe imbriqué). Interface `Execute(ctx, *State)` — exécution isolée du routage. |
| **Tool** | 🟢 | **Fonction déterministe** qu'un agent déclenche pour *agir* — sans repasser par le LLM. Écrit un fichier, appelle une API, calcule. Builtins : `noop`, `double`, `seed`, `freeze`. Le cerveau réfléchit, le tool exécute. |
| **Agent (nœud)** | 🟣 | Nœud qui **spawne la CLI LLM en subprocess** (kern-link) avec le state comme contexte. Le *seul* endroit où un modèle est réellement appelé. Non déterministe, coûte des tokens. |
| **Tool vs Agent** | 🟢 | Pas la nature d'une capacité mais **comment on la branche** dans le graphe. Un même skill peut servir de tool (func invoquée) ou d'agent (nœud). Marqué en frontmatter du `SKILL.md` : `type: tool \| agent`. |
| **Edge / RouteFunc** | 🟢 | Le lien qui **décide du nœud suivant**. Fonction Go `func(*State) []string` : `Static` (cible fixe), `Conditional` (branche selon le state), `Terminal` (rien → fin). Routage **déterministe**, jamais délégué au LLM — testable, gratuit, auditable. Le LLM influence en écrivant dans le state ; le Go décide. |
| **Frontier / frontière** | 🟢 | L'ensemble des nœuds à exécuter au *niveau* courant. Le moteur la calcule via les edges ; kern-pilot peut la *réécrire* de l'extérieur (replan). Point d'accroche : `RunFrom(frontier)`. |
| **Engine / fan-out** | 🟢 | Ordonnancement **level-synchrone** : chaque niveau exécute sa frontière *en parallèle* (goroutines, un Clone par branche), fusionne dans un ordre stable, calcule le suivant. Frontière mono-nœud → *remplacement* (honore suppressions/gel) ; fan-out (>1) → *Merge* additif. |
| **Cycle guard** | 🟢 | Budget de pas (`maxSteps`, défaut 10 000). Le run échoue si épuisé → protège des boucles infinies. |
| **Checkpoint** | 🟢 | État persisté en **SQLite** à chaque pas (hook `StepFunc`) : runID, step, frontière, state, statut, *et le chemin du graphe*. Granularité = frontière de niveau (frontière de sous-graphe = un pas atomique). |
| **Resume** | 🟢 | Reprise après échec : recharge le dernier checkpoint, relance `RunFrom`. `resume <run-id>` seul suffit — le chemin YAML est dans le checkpoint. |
| **Subgraph / sous-agent** | 🟢 | Un nœud lance un **graphe imbriqué** avec son propre state (seedé via `WithInput`, remonté via `WithOutput`). Vu du parent : *un pas atomique*. En YAML : `type: subgraph`, chargé avec garde anti-cycle. |
| **AgentRunner / Stub** | 🟢 | **Port** (interface) défini par `graph` pour l'exécution LLM. `Subprocess` spawne la vraie CLI (`KERN_AGENT_CLI`) ; `Stub` déterministe fait tout tourner *sans config LLM*. Inversion de dépendance : l'infra dépend de `graph`, jamais l'inverse. |

---

## 2. Briques kern-*

Modules **autonomes et agnostiques** (repo/déployable propre), reliés par **contrats neutres**,
jamais du code enfoui dans un autre. Frontières : subprocess/JSON-lines (kern-link), OTLP/GenAI
(kern-obs), contrat `write/query` (kern-memory).

| Brique | Étiq. | Rôle (une question) |
|---|---|---|
| **kern-orch** | 🟢 | *orchestre* — graphe, state, routage, checkpoints. Cœur agnostique métier. *(ce repo — EPIC-01 clos, v0.3.0)* |
| **kern-skills** | 🟢 | *catalogue* — « qu'est-ce qui existe ? ». Registre des `SKILL.md`, type `tool\|agent`. La carte du menu. |
| **kern-tools** | 🟢 | *implémente* — « comment on exécute ? ». Tools invocables (schéma I/O, validation), exposables API/MCP. La cuisine. |
| **kern-exec** | 🟢 | *confine* — bac à sable pour commandes/code arbitraire (cwd/env, timeout, quotas). Requis pour l'exécution risquée seulement. |
| **kern-policy** | 🟢 | *autorise* — évalue règles & budgets au runtime (allow/deny/**escalate**), point d'application *avant* orch. Règles YAML, décision Go (policy-as-code). |
| **kern-guard** | 🟢 | *valide* — garde-fou **structurel, inline, bloquant** (schémas, invariants). Embryon : `Graph.Validate`. |
| **kern-pilot** | 🟢 | *corrige* — canal de pilotage (control plane) : steer · queue · replan · nudge sur un run vivant. Le volant. Plomberie Go pur, source enfichable (humain/obs/agent). |
| **kern-obs** | 🟢 | *observe* — observabilité **structurelle** : ingestion OTLP/GenAI, watcher temps réel, « déclaré vs observé ». Déterministe, gratuit, toujours allumé. |
| **kern-scorer** | 🟣 | *juge* — scoring **sémantique, asynchrone**. Backend enfichable : heuristiques, embeddings, ou LLM-as-judge. Hors chemin critique. Seule brique où le LLM peut être central. |
| **kern-memory** | 🟢* | *se souvient* — mémoire agnostique, contrat `write`/`query`, couches `.okf` · RAG (chromem-go) · DAG. 100% Go. *(*embeddings via backend)* |
| **kern-anon** | 🔵 | Anonymisation / PII (Presidio) : pseudonymise avant l'appel LLM, ré-hydrate au retour. Faite, intégration à câbler. |
| **kern-link** | 🔵 | **Point de passage unique** vers les providers LLM (stream, multi-provider). Invoqué en subprocess (JSON-lines, §6.4 provisoire). |
| **kern-vault** | 🔵 | Coffre de credentials, secrets hors du corps des messages. Alimente kern-link. |
| **kern-ui** | 🔵 | Interface : pilotage & visualisation. En amont de kern-pilot. |

---

## 3. Concepts transverses

| Terme | Définition |
|---|---|
| **Data plane / control plane** | **Data plane** = le graphe qui s'exécute (kern-orch, les nœuds). **Control plane** = ce qui agit *sur* le run de l'extérieur (kern-pilot). Un nœud ne peut pas réécrire la boucle qui l'exécute → le pilotage out-of-band vit à côté. |
| **In-band vs out-of-band** | Auto-correction *dans* le run (agent écrit une décision → edge conditionnel) = tool + edge, pas besoin de pilot. Intervention *hors* du run, à tout moment, par un acteur externe = kern-pilot. |
| **Escalade** | Troisième verdict de policy (à côté de allow/deny) : l'action **dépasse l'autorité** de la brique → *suspendue et remontée d'un cran* (souvent un humain) pour approbation. Déclenche le human-in-the-loop, porté par kern-pilot. |
| **Human / agent-in-the-loop** | Un **humain** (ou un agent superviseur) qui garde la main pendant un run : approuve une escalade, réoriente via steer/replan. Le canal (kern-pilot) est agnostique à la source. |
| **LLM-as-judge** 🟣 | Utiliser un LLM pour **noter** la qualité/pertinence d'une sortie. Un des backends de kern-scorer (avec heuristiques et embeddings). Un juge doit être *indépendant* de ce qu'il évalue → lit la télémétrie de l'extérieur. |
| **Structurel vs sémantique** | La ligne de partage obs/scorer. **Structurel** : « le run a-t-il suivi le plan ? » → diff déterministe, sans LLM (kern-obs). **Sémantique** : « la réponse est-elle bonne ? » → compréhension du sens, LLM possible (kern-scorer). |
| **Déclaré vs observé** | Valeur ajoutée de kern-obs : comparer le **plan déclaré** (nœuds/edges attendus, émis en attributs) à la **trace réelle**. Détecte boucles anormales, divergences — sans lire l'intérieur de kern-orch. |
| **OTLP / GenAI semconv** | La frontière neutre d'observabilité : OpenTelemetry + conventions `gen_ai.*`. kern-orch *émet* (span par run/nœud/appel) ; kern-obs *ingère*, de n'importe quelle brique. **LangSmith écarté** (propriétaire). |
| **Policy-as-code** | Règles **déclaratives** (YAML) + **moteur d'évaluation** découplé de l'appli (cf. OPA). Le livre de lois est de la donnée ; le tribunal (kern-policy) est la logique + budgets stateful + escalade. |
| **.okf** | Couche mémoire **déclarative, versionnée, auditable** — le différenciateur de kern-memory. Fiche structurée (proche des fiches OKF du repo). Rappel traçable, pas une boîte noire vectorielle. |
| **RAG / DAG / chromem-go** | **RAG** : rappel par similarité vectorielle (défaut `chromem-go`, embarqué Go ; pgvector/Qdrant en scale). **DAG** : couche graphe multi-hop/provenance/temporel (phase 2). Deux couches de kern-memory sous un contrat `write/query` unique. |
| **Agnostique / brique** | Un module `kern-*` **autonome** (repo/déployable propre), composable via contrats standards, sans dépendre des internes d'un autre. La valeur métier vient des *skills*, pas du harnais — qui reste neutre. |

---

## La phrase qui résume tout

**Le harnais gouverne, le LLM exécute.** Orchestration, routage, observation structurelle,
pilotage, policy : tout est du **Go déterministe**. Le LLM n'apparaît qu'à deux endroits — dans
les nœuds *agent* (produire du langage, via kern-link) et dans kern-scorer (juger du sens).
Partout ailleurs : reproductible, gratuit, auditable.

Chaque brique répond à **une** question : skills *catalogue*, tools *implémente*, exec *confine*,
policy *autorise*, guard *valide*, pilot *corrige*, obs *observe*, scorer *juge*, memory
*se souvient*, orch *orchestre*.
