# Rétro — Kern-Orch

Journal des pièges spécifiques au projet, ajoutés au moment où ils mordent.
Les pièges génériques (réutilisables hors projet) remontent aussi dans le skill
`greenfield-tdd-okf/references/pieges.md`.

## 2026-07-20 — Bootstrap
- `agentrunner` dépend d'une brique CLI externe non accessible (GitHub d'un collègue).
  Décision : `AgentRunner` interface + `stubRunner` déterministe → l'app tourne sans
  aucune config LLM ; le vrai `subprocessRunner` se branche via `KERN_AGENT_CLI`.
  (applique le piège générique « mode simulé si secret/service externe requis »)

## 2026-07-20 — Agentrunner
- Typed-nil Go : un `*bytes.Buffer` nil affecté à un champ `io.Writer` donne une interface
  NON nil → le garde `if w == nil` rate et `io.WriteString` panique (nil deref). Ne jamais
  stocker un pointeur typé nil dans un champ interface ; ne l'assigner que s'il est non-nil.
  (générique → remonté dans le skill pieges.md)
- E2E subprocess testé avec un vrai process externe (script sh dans t.TempDir()) en plus du
  pattern TestHelperProcess — prouve le chemin réel Engine→AgentNode→CLI sans la brique du collègue.
