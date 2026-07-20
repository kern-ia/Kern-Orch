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

## 2026-07-20 — Clôture v0.1.0
- Squelette complet livré : graph engine, agentrunner (stub+subprocess), checkpoint+resume,
  skills registry, topology loader YAML, config, CLI (run/resume/status/list-skills). Tout vert
  sous `go test -race`, E2E prouvé au binaire (stub ET vraie CLI subprocess).
- Inversion de dépendance tenue : `graph` définit les ports (AgentRunner, StepFunc) ; les
  packages d'infra (agentrunner, checkpoint) dépendent de `graph`, jamais l'inverse.
- Dettes assumées à réconcilier plus tard :
  1. Contrat JSON-lines agentrunner PROVISOIRE (§6.4) — à aligner sur la vraie CLI du collègue.
  2. resume redemande le chemin YAML (non persisté dans le checkpoint) — envisager de le stocker.
  3. builtinRegistry ne fournit que `noop` — les projets enregistrent leurs tools/routers en Go.
  4. modernc.org/sqlite : ouvrir avec PRAGMA busy_timeout/WAL si accès concurrent multi-run un jour.
