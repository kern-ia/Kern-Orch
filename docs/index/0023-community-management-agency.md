---
id: 0023
feature: community-management-agency
branch: feature/community-management-agency
status: wip
files:
  - internal/cmd/comm_routers.go
  - internal/cmd/runtime.go
  - examples/community-management-agency.yaml
  - skills/community-management-agency/agent_cli.py
  - skills/community-management-agency/SKILL.md
  - skills/community-management-agency/run.sh
tests:
  - internal/cmd/comm_routers_test.go
  - internal/cmd/comm_graph_test.go
decisions:
  - "2026-08-05 : onConfirmDecision (codé en dur sur un nœud \"confirm\" et les cibles \"approved\"/\"refused\") n'est pas réutilisable pour ce graphe, qui a DEUX points d'approbation. decisionRouter(nodeID, approved, refused) généralise le motif, testé en isolation, câblé deux fois (confirm_strategie, confirm_publication) sans toucher au routeur existant."
  - "2026-08-05 : double mode côté stratège (avis / proposition) porté par un marqueur littéral \"MODE: avis|proposition\" que le prompt force en tête de réponse, parsé côté adaptateur (extract_mode) — pas de heuristique sur le message brut de l'utilisateur, le jugement reste au modèle. Défaut sûr si le marqueur manque : \"proposition\" (jamais sauter la validation humaine par erreur)."
  - "2026-08-05 : crew-comm réutilisé en bibliothèque (agents/audience.py, strategiste.py, redacteur.py) exactement comme prospection réutilise crew-crm — _extraire_plan importé tel quel (privé par convention, réutilisé quand même, même précédent que _marquer_inventions)."
  - "2026-08-05 : run_publieur reproduit le garde-fou G2 de crew-comm/agents/publieur.py à l'identique — sans connecteur de publication branché, aucun appel modèle, signalement explicite plutôt qu'un faux compte-rendu."
---

**Quoi** : nouveau skill `community-management-agency` — brief audience → avis ou
proposition de stratégie → (validation humaine si proposition) → rédaction par
plateforme → validation humaine → publication. Port de `crew-comm` (LangGraph/Ollama)
dans un graphe kern-orch, sur le patron de `prospection`.

**Vérifié** : `go test ./... -race` vert (build inclus). Logique pure des trois nouveaux
routeurs Go testée en isolation, y compris l'absence de fuite entre les deux portes
d'approbation. Le graphe YAML charge et valide (`g.Validate()`, tous les nœuds cibles des
routeurs existent). Import réel des prompts `crew-comm` sous le venv du projet, et logique
pure de l'adaptateur (`extract_mode`, `strip_mode_line`, `run_publieur`, `_extraire_plan`)
vérifiée en isolation. **Reste à faire** : passe E2E contre le vrai CLI `claude`, les
quatre nœuds enchaînés, les deux modes — non faite dans cette session.

**Pièges** : aucun nouveau — les deux déjà documentés pour `prospection` (pollution de
stdout, format de contenu variable par fournisseur) ne s'appliquent pas ici, `claude -p`
répond en texte simple.
