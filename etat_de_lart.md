# État de l'art & positionnement marché

> Question posée : *sommes-nous sur un projet valable ? A-t-on un produit qui répond au
> marché ?* Réponse franche ci-dessous, appuyée sur l'état de l'art 2026. — 2026-07-22.
>
> Ce document est une **analyse de positionnement**, pas un plan d'exécution. Il ne décide
> rien : il pose le constat pour trancher la stratégie ensuite.

## Verdict court

**Le positionnement est valable et bien timé — mais on n'a pas encore le produit qui répond
au marché.** On a construit la partie **commoditisée** (le moteur d'orchestration) et on a une
**bonne thèse** pour la partie à valeur (gouvernance / souveraineté), mais celle-ci est **quasi
entièrement non construite**.

- **Projet valable ?** Oui sur la *direction* (souverain, gouverné, self-host, timing EU). Comme
  socle d'architecture, c'est propre et solide.
- **Produit qui répond au marché ?** **Non, pas encore.** Le fini (moteur) est le morceau
  commoditisé ; ce qui répondrait au marché (le **plan de contrôle / gouvernance**) est à
  ~80 % devant nous.

## Constat 1 — le moteur (kern-orch) n'est pas un différenciateur

Graphe / state / checkpoints / resume : c'est devenu une **commodité**, et en **Go**
c'est désormais contesté par des acteurs financés :

| Concurrent | Nature | Ce qui recoupe kern-orch |
|---|---|---|
| **Google ADK Go** (1.0, nov. 2025) | Google | agents séquentiels / parallèles / loop, **OTel natif** |
| **Eino** (ByteDance / CloudWeGo) | Go natif, mature | multi-agent, **human-in-the-loop interrupt/resume** (= notre `resume`) |
| **Genkit Go** (Firebase) | Google | streaming, eval, tracing intégrés |
| **LangChainGo** | port communautaire | chains / agents / tools / memory |

→ La thèse « le Go est mal servi » **s'affaiblit**. Se battre frontalement sur le moteur = lutte
contre du **gratuit, mature et backé Google/ByteDance**. Pas de moat ici.

## Constat 2 — le marché réel est ailleurs, et il est chaud

La demande qui **paie** en 2026, c'est la **souveraineté + gouvernance** pour le régulé :

- **AI Act** (obligations « high-risk » à partir du **2 août 2026**), **DORA**, **NIS2**,
  exposition **CLOUD Act** → banques, santé, secteur public EU veulent du **on-prem, data
  residency, audit trail, garde-fous, défense prompt-injection, pseudonymisation PII**.
- Le modèle **cloud multi-tenant perd** les acheteurs régulés (ils veulent tourner dans leur
  périmètre : clés, runtime, logs d'audit chez eux).

C'est **exactement** là que vivent nos briques différenciantes :

| Besoin marché régulé | Brique kern-* correspondante | Statut |
|---|---|---|
| Pseudonymisation / PII | **kern-anon** | 🔌 externe · faite · à câbler |
| Règles / budgets / permissions | **kern-policy** | ⬜ vide |
| Garde-fous bloquants, anti-injection | **kern-guard** | ⬜ vide |
| Audit trail / traçabilité (OTel) | **kern-obs** | ⬜ en cours |
| Self-host / zéro lock-in | architecture briques `kern-*` | 🟡 principe posé |

→ **Le moat n'est pas dans le graphe : il est dans gouvernance + souveraineté + conformité +
PII + audit.** Notre cartographie le pressent déjà (moitié droite/basse) — mais tant que ces
briques sont vides, on a **une belle colonne vertébrale sans le produit vendable**.

## Concurrence déjà présente sur le créneau souverain

- **Mistral AI** — option entreprise la plus mature en EU régulé (track record à l'échelle).
- **AIgent** (Suisse) — plateforme d'agents souveraine, model-choice par agent, hosting Suisse
  ou on-prem, GDPR-ready, du SME à la banque régulée.
- Divers guides d'achat « sovereign agentic AI » structurent déjà des tiers de déploiement
  (VPC vendor-managed vs customer-operated).

→ Le créneau **n'est pas vierge** : la différenciation devra être nette (ouverture, self-host
réel, briques composables, conformité AI Act outillée).

## La question stratégique que ça pose

**Make vs Adopt sur le moteur :**
- **Make** (actuel) : on garde notre `kern-orch` maison. Risque : réinventer une commodité
  contestée par Google/ByteDance, dilue l'effort loin du moat.
- **Adopt** : on bâtit la couche gouvernance/souveraineté **par-dessus Eino ou ADK Go** (tous
  deux Go + OTel natifs). `kern-orch` devient **mince** (adaptateur), et **toute** la
  différenciation va sur `kern-anon / policy / guard / obs` + conformité.

**MVP orienté conformité** (à définir) : le plus petit livrable démontrable qui parle à un
acheteur régulé — typiquement une démo « agent gouverné » qui montre, bout à bout : PII
pseudonymisée avant l'LLM, policy qui bloque une action hors budget/permission, garde-fou
bloquant, et **audit trail complet** rejouable. C'est ça qui se vend, pas le graphe.

## Recommandation (analyse, non tranchée)

1. Ne plus investir sur le moteur comme différenciateur ; le figer « suffisant » ou envisager
   l'adoption d'Eino/ADK.
2. Concentrer l'effort sur les **joyaux** : kern-policy, kern-guard, kern-anon (câblage),
   kern-obs (audit) — la valeur régulée.
3. Cadrer un **MVP conformité** daté sur le tailwind **AI Act (août 2026)**.
4. Choisir un **segment** précis (ex. banque/DORA, santé, secteur public EU) plutôt que
   « framework d'agents généraliste ».

## Sources
- [Best Go AI agent frameworks 2026 — Fast.io](https://fast.io/resources/best-ai-agent-frameworks-for-golang-2026/)
- [Golang AI agent frameworks 2026 — Relia Software](https://reliasoftware.com/blog/golang-ai-agent-frameworks)
- [The best AI agent frameworks in 2026 — LangChain](https://www.langchain.com/resources/ai-agent-frameworks)
- [Sovereign agentic AI — EU procurement guide 2026 — Knowlee](https://www.knowlee.ai/blog/sovereign-agentic-ai-platforms-2026)
- [Self-hosted AI agent platforms 2026 (CISO/regulated) — Knowlee](https://www.knowlee.ai/blog/self-hosted-ai-agent-platforms-2026)
- [Sovereign AI for regulated industries — Lyzr](https://www.lyzr.ai/blog/sovereign-ai/)
- [Top sovereign AI platforms Europe 2026 — Vstorm](https://vstorm.co/agentic-ai/ai-platforms/top-5-sovereign-ai-platforms-in-europe-ranked-by-compliance-regional-fit-and-data-control/)
- [10 best AI agent platforms for enterprise 2026 — Rasa](https://rasa.com/blog/10-best-ai-agent-platforms-for-enterprise-in-2026)
