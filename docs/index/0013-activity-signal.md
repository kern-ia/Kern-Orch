---
id: 0013
feature: activity-signal
branch: feature/activity-signal
status: done
files:
  - internal/report/activity.go
  - internal/report/http.go
  - internal/agentrunner/subprocess.go
  - internal/cmd/runtime.go
  - internal/cmd/commands.go
  - internal/config/config.go
  - contracts/kern.activity.v1.json
tests:
  - internal/report/activity_test.go
  - internal/cmd/publish_skills_test.go
decisions:
  - "2026-07-27 : `ActivityReporter` poste HORS du fil de l'appelant — un step se rapporte entre deux niveaux où une pause coûte peu, l'activité se rapporte au moment exact où un agent va démarrer"
  - "2026-07-27 : `Flush()` obligatoire avant sortie de commande — le signal d'arrêt est le dernier d'un run, donc précisément celui qu'un processus qui sort laisserait tomber"
  - "2026-07-27 : contexte détaché (`context.WithoutCancel`) — un run est déjà annulé quand son dernier agent s'arrête, rapporter sur son contexte reviendrait à ne jamais rapporter l'arrêt"
  - "2026-07-27 : `OnActivity` s'ouvre au spawn, pas au premier token — un provider qui répond d'un bloc ne streame rien et n'aurait jamais été vu comme travaillant"
  - "2026-07-27 : fermeture en `defer` pour couvrir tous les chemins de sortie, y compris les erreurs"
  - "2026-07-27 : `activityRelay` comme couture — le runner est construit avant que le run ait un id, le hook ne peut pas être écrit à la construction"
  - "2026-07-27 : troisième variable d'URL (`KERN_ACTIVITY_REPORT_URL`), même raison que les deux autres — pas de route sœur devinée"
  - "2026-07-27 : `resume` câble l'activité aussi, sinon un run repris n'allumerait jamais le phare"
---

**Quoi** : kern-orch dit quand un modèle travaille. Un signal à l'ouverture et un à la
fermeture par nœud agent, envoyés sans bloquer le run.

**Frontière** : comme les deux autres contrats, une URL configurée et rien d'autre. kern-orch
ne sait pas ce qu'un sink fait de ces signaux.

**Pièges** :
- `newRunner` était appelé avant `newRunID()` : le hook ne pouvait pas connaître le run.
  D'où le relais, plutôt que de réordonner la commande.
- Un `Flush()` oublié rend le test vert en local (le sink répond vite) et faux en réel.
  Le test `TestFlushWaitsForSignalsInFlight` compte les requêtes pour l'interdire.
