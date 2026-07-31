---
name: prospection
type: agent
description: Pilote une prospection CRM en langage naturel — fiche lead, plan d'action, validation humaine, exécution
graph: examples/prospection.yaml
---

# prospection

Dispatché depuis le chat (`/prospection <texte en langage naturel décrivant le lead>`).
Contrairement à un agent skill ordinaire (un seul nœud ad-hoc, tout le texte comme
prompt), ce skill déclare un `graph:` — Dispatch charge le fichier
`examples/prospection.yaml` au lieu de construire un run à un nœud, et le texte du chat
devient `state["message"]` avant que le premier nœud (`secretaire`) ne s'exécute.

Nécessite `KERN_AGENT_CLI` pointé sur `skills/prospection/agent_cli.py` — un seul
script, une branche par nœud (`secretaire`, `commercial`, `approved`), qui réutilise en
bibliothèque la configuration de modèle agnostique et les prompts déjà écrits et vérifiés
dans `mon-orchestrateur-agents/agents-locaux/crew-crm`.

**Quota Gemini (démo 2026-07-31)** : la clé actuelle a un plafond gratuit d'environ 20
requêtes/jour sur le modèle courant — vérifié en vrai, pas contournable par un autre
modèle (les autres générations gratuites ont un quota à zéro ou sont dépréciées pour
cette clé). `secretaire` et `commercial` sont vérifiés en vrai ; `approved` (l'écriture
CRM) reste à vérifier lors de la répétition, une fois le quota journalier réinitialisé.
**Secours en direct si le quota tombe à zéro pendant la démo** : relancer `kern-orch
serve` avec `MODEL_BACKEND=ollama` dans l'environnement (aucun changement de code) — plus
lent (qwen3.6:27b a montré 50-60s pour un mot lors des tests de cette session), mais
fonctionnel.
