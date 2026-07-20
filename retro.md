# Rétro — Kern-Orch

Journal des pièges spécifiques au projet, ajoutés au moment où ils mordent.
Les pièges génériques (réutilisables hors projet) remontent aussi dans le skill
`greenfield-tdd-okf/references/pieges.md`.

## 2026-07-20 — Bootstrap
- `agentrunner` dépend d'une brique CLI externe non accessible (GitHub d'un collègue).
  Décision : `AgentRunner` interface + `stubRunner` déterministe → l'app tourne sans
  aucune config LLM ; le vrai `subprocessRunner` se branche via `KERN_AGENT_CLI`.
  (applique le piège générique « mode simulé si secret/service externe requis »)
