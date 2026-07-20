---
id: okf-0003
feature: agentrunner
branch: feature/agentrunner
status: done
files:
  - internal/agentrunner/protocol.go
  - internal/agentrunner/stub.go
  - internal/agentrunner/subprocess.go
tests:
  - internal/agentrunner/stub_test.go
  - internal/agentrunner/subprocess_test.go
  - internal/agentrunner/integration_test.go
decisions:
  - "2026-07-20 : contrat PROVISOIRE JSON-lines (§6.4) — 1 Request sur stdin, Events {token|result|error} par ligne sur stdout ; dernier result gagne. À réconcilier avec la vraie CLI"
  - "2026-07-20 : Stub déterministe (per-node > Default > echo prompt) = mode sans LLM ; Subprocess via KERN_AGENT_CLI"
  - "2026-07-20 : les deux impl satisfont graph.AgentRunner (assert compile-time)"
---

**Quoi** : Implémentations du port `graph.AgentRunner`. `Stub` déterministe (tourne sans
aucune config LLM). `Subprocess` spawn la CLI externe, écrit la Request sur stdin, scanne les
Events JSON-lines de stdout (forward tokens vers TokenSink, capture le result), gère erreur/exit.
E2E prouvé avec un vrai process externe.

**Pièges** : typed-nil `*bytes.Buffer` → `io.Writer` non-nil (panic) — voir retro.md / skill pieges.md.
